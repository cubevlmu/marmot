package core

import (
	"gorm.io/gorm"
	"marmot/utils"
)

type TaskType int

const (
	TTypeUnknown TaskType = iota
	TTypeInsert
	TTypeUpdate
	TTypeDelete // hard delete
	TTypeRemove // mark delete
)

type QueueTask struct {
	taskType TaskType
	taskData interface{}
	resultCh chan error
}

type DbCtx struct {
	Db         *gorm.DB
	writeQueue *utils.RingQueue[QueueTask]
}

func newDbCtx(name string) *DbCtx {
	path := GetSubDirFilePath(name)
	ctx := &DbCtx{
		writeQueue: utils.NewRingQueue[QueueTask](100),
	}
	var err error
	ctx.Db, err = utils.OpenSqlite(path)
	if err != nil {
		LogError("[Db] failed to open database: %v", err)
		return nil
	}

	go actionWorker(ctx)

	return ctx
}

func actionWorker(db *DbCtx) {
	for {
		d := db.writeQueue.WaitDequeue()

		var r *gorm.DB
		switch d.taskType {
		case TTypeInsert:
			r = db.Db.Create(d.taskData)
			break
		case TTypeUpdate:
			r = db.Db.Model(d.taskData).Updates(d.taskData)
			break
		case TTypeRemove:
		case TTypeDelete:
			r = db.Db.Delete(d.taskData)
			break
		case TTypeUnknown:
			r = nil
			break
		}

		if r == nil || r.Error != nil {
			LogError("[Db] run queue task failed [type : %v error : %v]", d.taskType, r)
		}

		if d.resultCh != nil {
			select {
			case d.resultCh <- r.Error:
			default:
				// skip blocking
			}
		}
	}
}

func (db *DbCtx) Insert(data interface{}) error {
	ch := make(chan error, 1)
	tsk := QueueTask{
		taskType: TTypeInsert,
		taskData: data,
		resultCh: ch,
	}
	r := db.writeQueue.Enqueue(tsk)
	if r != nil {
		return r
	}
	return <-ch
}

func (db *DbCtx) Update(data interface{}) error {
	ch := make(chan error, 1)
	tsk := QueueTask{
		taskType: TTypeUpdate,
		taskData: data,
		resultCh: ch,
	}
	r := db.writeQueue.Enqueue(tsk)
	if r != nil {
		return r
	}
	return <-ch
}

func (db *DbCtx) Remove(data interface{}) error {
	ch := make(chan error, 1)
	tsk := QueueTask{
		taskType: TTypeRemove,
		taskData: data,
		resultCh: ch,
	}
	r := db.writeQueue.Enqueue(tsk)
	if r != nil {
		return r
	}
	return <-ch
}

func (db *DbCtx) Delete(data interface{}) error {
	ch := make(chan error, 1)
	tsk := QueueTask{
		taskType: TTypeDelete,
		taskData: data,
		resultCh: ch,
	}
	r := db.writeQueue.Enqueue(tsk)
	if r != nil {
		return r
	}
	return <-ch
}
