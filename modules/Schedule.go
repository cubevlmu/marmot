package modules

import (
	"container/heap"
	"marmot/core"
	zero "marmot/onebot"
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
				ActionTime:  "2025-07-26 22:00:00",
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
	if action.index > len(s.cfg.Tasks) {
		return
	}
	r := s.cfg.Tasks[action.index]

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
			s.execute(item)

			if item.ActionTimes > 1 || item.ActionTimes <= 0 {
				if item.ActionTimes > 0 {
					item.ActionTimes--
				}
				item.ActionTime += item.Interval // Interval 是 time.Duration.Nanoseconds()
				s.lock.Lock()
				heap.Push(&s.tasks, item)
				s.lock.Unlock()
			}
			continue
		}

		// wait a while to handle new task adding
		timer := time.NewTimer(time.Duration(delay))
		s.lock.Unlock()

		select {
		case <-timer.C:
			// prepare to run next turn
		}
	}
}

func (s *ScheduleMgr) Init(_ *core.ModuleMgr) bool {
	s.cfg = &ScheduleCfg{}
	path := core.GetSubDirFilePath("scheduler.yml")
	r := core.InitCustomConfig(s.cfg, path)
	if r != nil {
		core.LogWarn("[ScheduleMgr] failed to init scheduler config %v", r)
		s.cfg = s.cfg.CreateDefaultConfig().(*ScheduleCfg)
	}

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

	return true
}

func (s *ScheduleMgr) Stop(_ *core.ModuleMgr) {

}

func (s *ScheduleMgr) Reload(mgr *core.ModuleMgr) {
	s.Init(mgr)
	s.Stop(mgr)
}

func (s *ScheduleMgr) OnMsg(_ *zero.Ctx) {
	return
}

func init() {
	core.RegisterNamed("schedule", func() core.IModule {
		return &ScheduleMgr{}
	})
}
