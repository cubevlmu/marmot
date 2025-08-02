package modules

import (
	"container/heap"
	"fmt"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"strconv"
	"strings"
	"sync"
	"time"
)

type STaskType int

const (
	STUnknown STaskType = iota
	STBanGroup
	STUnbanGroup
	STBroadcast
)

func parseTime(ts string) (int64, error) {
	loc := time.Local
	t, err := time.ParseInLocation("2006-01-02 15:04:05", ts, loc)
	if err != nil {
		return 0, err
	}

	unixTime := t.UnixNano()
	return unixTime, nil
}

type ScheduleTask struct {
	ActionTime  string    `koanf:"action_time" yaml:"action_time"`
	ActionTimes int       `koanf:"action_times" yaml:"action_times"`
	Interval    string    `koanf:"interval" yaml:"interval"`
	TaskType    STaskType `koanf:"task_type" yaml:"task_type"`
	TaskData    string    `koanf:"task_data" yaml:"task_data"`
	Group       []int64   `koanf:"group" yaml:"group"`
}

type ScheduleCfg struct {
	Tasks []ScheduleTask `koanf:"tasks" yaml:"tasks"`
}

func (s ScheduleCfg) CreateDefaultConfig() interface{} {
	return &ScheduleCfg{
		Tasks: []ScheduleTask{
			{
				ActionTime:  "2025-09-26 22:00:00",
				ActionTimes: 3,
				Interval:    "30s",
				TaskType:    STBroadcast,
				TaskData:    "测试",
				Group:       []int64{},
			},
		},
	}
}

type taskItem struct {
	ActionTime  int64
	ActionTimes int
	Interval    int64
	index       int
	id          int64
}

type taskHeap []*taskItem

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].ActionTime < h[j].ActionTime }
func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(x any) {
	item := x.(*taskItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type ScheduleMgr struct {
	cfg     *ScheduleCfg
	lock    sync.Mutex
	cond    *sync.Cond
	tasks   taskHeap
	ctx     *zero.Ctx
	running bool
}

func (s *ScheduleMgr) execute(action *taskItem) {
	if action.index >= len(s.cfg.Tasks) {
		return
	}
	r := s.cfg.Tasks[action.id]

	if s.ctx == nil {
		s.ctx = zero.GetBot(core.Common.BotQQ)
	}

	for _, id := range r.Group {
		switch r.TaskType {
		case STBanGroup:
			if r.TaskData != "" {
				s.ctx.SendGroupMessage(id, r.TaskData)
			}
			s.ctx.SetGroupWholeBan(id, true)
			return
		case STUnbanGroup:
			if r.TaskData != "" {
				s.ctx.SendGroupMessage(id, r.TaskData)
			}
			s.ctx.SetGroupWholeBan(id, false)
			return
		case STBroadcast:
			s.ctx.SendGroupMessage(id, r.TaskData)
			return
		case STUnknown:
			return
		}
	}
}

func (s *ScheduleMgr) run() {
	for {
		s.lock.Lock()

		for len(s.tasks) == 0 {
			s.cond.Wait() // wait next task
		}

		now := time.Now().UnixNano()
		item := s.tasks[0]
		delay := item.ActionTime - now

		if delay <= 0 {
			heap.Pop(&s.tasks)
			s.lock.Unlock()
			go s.execute(item)

			if item.ActionTimes == -1 || item.ActionTimes > 1 {
				// For infinite tasks (ActionTimes = -1), don't decrease ActionTimes
				if item.ActionTimes > 0 {
					item.ActionTimes-- // Decrease remaining times for repeated task
				}
				item.ActionTime += item.Interval // Update action time based on interval

				// Update config with new action time and remaining times
				s.cfg.Tasks[item.id].ActionTime = time.Unix(0, item.ActionTime).Format("2006-01-02 15:04:05")
				s.cfg.Tasks[item.id].ActionTimes = item.ActionTimes

				// Re-add the task back to the queue
				s.lock.Lock()
				heap.Push(&s.tasks, item)
				s.lock.Unlock()

			} else {
				// Remove the task if it's a one-time task
				s.lock.Lock()
				s.cfg.Tasks = append(s.cfg.Tasks[:item.id], s.cfg.Tasks[item.id+1:]...)
				heap.Init(&s.tasks)
				s.lock.Unlock()
			}

			r := core.SaveCustomConfigToFile(core.GetSubDirFilePath("scheduler.yml"), s.cfg)
			if r != nil {
				core.LogError("[Schedule] failed to update scheduler.yml err: %v", r)
			}

			continue
		}

		timer := time.NewTimer(time.Duration(delay))
		s.lock.Unlock()

		select {
		case <-timer.C:
			// Prepare to run next turn
		}
	}
}

func (s *ScheduleMgr) Init(mgr *core.ModuleMgr) bool {
	s.cfg = &ScheduleCfg{}
	path := core.GetSubDirFilePath("scheduler.yml")
	r := core.InitCustomConfig(s.cfg, path)
	if r != nil {
		core.LogWarn("[ScheduleMgr] failed to init scheduler config %v", r)
		s.cfg = s.cfg.CreateDefaultConfig().(*ScheduleCfg)
	}

	// validate task time and handle infinite tasks (ActionTimes == -1)
	validTasks := make([]ScheduleTask, 0)
	currnetTm := time.Now().UnixNano()
	for _, task := range s.cfg.Tasks {
		t, err := parseTime(task.ActionTime)
		if err != nil {
			core.LogError("[ScheduleMgr] failed to parse action time %v", err)
			continue
		}

		if t-currnetTm < 0 {
			core.LogDebug("[ScheduleMgr] task with negative action time will be removed: %v", task)
			continue
		}

		// Special handling for infinite tasks (ActionTimes == -1)
		if task.ActionTimes == -1 {
			core.LogDebug("[ScheduleMgr] infinite task detected, will repeat indefinitely: %v", task)
		}

		validTasks = append(validTasks, task)
	}
	s.cfg.Tasks = validTasks

	// save to config
	err := core.SaveCustomConfigToFile(path, s.cfg)
	if err != nil {
		core.LogError("[ScheduleMgr] failed to save updated scheduler config %v", err)
	} else {
		core.LogInfo("[ScheduleMgr] updated scheduler config saved successfully")
	}

	// init scheduler heap
	s.tasks = make(taskHeap, len(s.cfg.Tasks))
	for i, task := range s.cfg.Tasks {
		t, e := parseTime(task.ActionTime)
		if e != nil {
			core.LogError("[ScheduleMgr] [Task index: %v] failed to parse action time %v", i, e)
			continue
		}
		d, e := time.ParseDuration(task.Interval)
		if e != nil {
			core.LogError("[ScheduleMgr] [Task index: %v] failed to parse interval time %v", i, e)
			continue
		}
		s.tasks[i] = &taskItem{
			ActionTime:  t,
			ActionTimes: task.ActionTimes,
			Interval:    d.Nanoseconds(),
			index:       i,
			id:          int64(i),
		}
	}

	s.cond = sync.NewCond(&s.lock)
	heap.Init(&s.tasks)
	s.running = true
	go s.run()

	mgr.RegisterCmd().
		RegisterGroupAdmin("RegTask", s.onRegTask)

	return true
}

func (s *ScheduleMgr) Stop(_ *core.ModuleMgr) {
	r := core.SaveCustomConfigToFile(core.GetSubDirFilePath("scheduler.yml"), s.cfg)
	if r != nil {
		core.LogError("[ScheduleMgr] failed to save scheduler.yml err: %v", r)
	}

	s.cfg = nil
}

func (s *ScheduleMgr) Reload(mgr *core.ModuleMgr) {
	s.Init(mgr)
	s.Stop(mgr)
}

func (s *ScheduleMgr) onRegTask(args []string, ctx *zero.Ctx) {
	if len(args) != 5 {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("使用方法 RegTask [2025:08:02 15:00] [1] [10s] [1] [\"群聊禁言\"]"))
		return
	}

	t, e := parseTime(strings.ReplaceAll(args[0], "\"", ""))
	if e != nil {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("错误的时间格式! xxxx:xx:xx xx:xx:xx"))
		return
	}

	times, e := strconv.Atoi(args[1])
	if e != nil {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("转换次数失败!"))
		return
	}

	dur, e := time.ParseDuration(args[2])
	if e != nil {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("错误的间隔时间"))
		return
	}

	types, e := strconv.Atoi(args[3])
	if e != nil {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("转换类型失败!"))
		return
	}

	s.cfg.Tasks = append(s.cfg.Tasks, ScheduleTask{
		ActionTime:  args[0],
		ActionTimes: times,
		TaskType:    STaskType(types),
		Interval:    args[2],
		TaskData:    strings.ReplaceAll(args[3], "\"", ""),
		Group:       []int64{ctx.Event.GroupID},
	})

	e = core.SaveCustomConfigToFile(core.GetSubDirFilePath("scheduler.yml"), s.cfg)
	if e != nil {
		core.LogError("[ScheduleMgr] failed to save scheduler.yml err: %v", e)
	}

	s.lock.Lock()
	heap.Push(&s.tasks, &taskItem{
		ActionTime:  t,
		ActionTimes: times,
		Interval:    int64(dur),
		id:          int64(len(s.tasks)),
		index:       len(s.cfg.Tasks),
	})
	s.lock.Unlock()

	ctx.SendGroupMessage(ctx.Event.GroupID, fmt.Sprintf("成功添加! %v", s.cfg.Tasks[len(s.cfg.Tasks)-1]))
}

func init() {
	core.RegisterNamed("schedule", func() core.IModule {
		return &ScheduleMgr{}
	})
}
