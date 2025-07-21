package modules

import (
	"container/heap"
	zero "github.com/cubevlmu/CZeroBot"
	"gorm.io/gorm"
	"marmot/core"
	"sync"
	"time"
)

type STaskType int

const (
	STUnknown STaskType = iota
	STBanGroup
	STBroadcast
)

type ScheduleAction struct {
	ActionTime  int64
	ActionTimes int
}

type ScheduleTask struct {
	TaskId   int64 `gorm:"primaryKey"`
	Action   ScheduleAction
	TaskType STaskType
	TaskData interface{}
	Removed  bool
}

type taskItem struct {
	action ScheduleAction
	index  int
	id     int64
}

type taskHeap []*taskItem

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].action.ActionTime < h[j].action.ActionTime }
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
	db      *gorm.DB
	lock    sync.Mutex
	cond    *sync.Cond
	tasks   taskHeap
	running bool
}

func (s *ScheduleMgr) execute(action *taskItem) {
	body := &ScheduleTask{}
	if err := s.db.Model(&ScheduleTask{}).
		Where("TaskId == ?", action.id).
		First(&body); err != nil {
		core.LogError("[ScheduleMgr] failed to load task: %v", err)
	}

	switch body.TaskType {
	case STBanGroup:
	case STBroadcast:
	case STUnknown:
		return
	}
}

func (s *ScheduleMgr) run() {
	for {
		s.lock.Lock()

		for len(s.tasks) == 0 {
			s.cond.Wait() // wait next task
		}

		now := time.Now().Unix()
		item := s.tasks[0]
		delay := item.action.ActionTime - now

		if delay <= 0 {
			heap.Pop(&s.tasks)
			s.lock.Unlock()
			s.execute(item)

			if item.action.ActionTime-1 > 0 || item.action.ActionTimes <= 0 {
				item.action.ActionTime--
				heap.Push(&s.tasks, item)
			}
			continue
		}

		// wait a while to handle new task adding
		timer := time.NewTimer(time.Duration(delay) * time.Second)
		s.lock.Unlock()

		select {
		case <-timer.C:
			// prepare to run next turn
		}
	}
}

func (s *ScheduleMgr) Init(mgr *core.ModuleMgr) {
	action := make([]ScheduleAction, 0)
	ids := make([]int64, 0)
	if err := s.db.Model(&ScheduleTask{}).
		Where("removed = ?", false).
		Pluck("TaskId", &ids).
		Pluck("action", &action).Error; err != nil {
		core.LogError("[ScheduleMgr] failed to load actions: %v", err)
		return
	}

	s.tasks = make(taskHeap, len(action))
	for i, t := range action {
		s.tasks[i] = &taskItem{
			action: t,
			index:  i,
			id:     ids[i],
		}
	}

	s.cond = sync.NewCond(&s.lock)
	heap.Init(&s.tasks)
	s.running = true
	go s.run()
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

func newScheduleMgr() *ScheduleMgr {
	mgr := &ScheduleMgr{
		db: core.Common.Database.Db,
	}
	if mgr.db == nil {
		core.LogError("[ScheduleMgr] failed to connect database")
		panic("[ScheduleMgr] failed to connect database")
	}
	err := mgr.db.AutoMigrate(&ScheduleTask{})
	if err != nil {
		core.LogError("[ScheduleMgr] failed to set auto-migrate: %v", err)
		return nil
	}

	return mgr
}

func init() {
	core.RegisterNamed("schedule", func() core.IModule {
		return newScheduleMgr()
	})
}
