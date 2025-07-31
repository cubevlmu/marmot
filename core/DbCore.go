package core

import (
	"errors"
	"gorm.io/gorm"
	"marmot/utils"
	"sync"
)

type TaskType int

const (
	TTypeUnknown TaskType = iota
	TTypeInsert
	TTypeUpdate
	TTypeDelete // hard delete
	TTypeRemove // soft delete
)

type QueueTask struct {
	taskType TaskType
	taskData interface{}
	resultCh chan error
}

type DbCtx struct {
	Db         *gorm.DB
	writeQueue *utils.RingQueue[QueueTask]
	closeCh    chan struct{}
	wg         sync.WaitGroup
}

func newDbCtx(name string) *DbCtx {
	path := GetSubDirFilePath(name)
	db, err := utils.OpenSqlite(path)
	if err != nil {
		LogError("[Db] failed to open database: %v", err)
		return nil
	}

	ctx := &DbCtx{
		Db:         db,
		writeQueue: utils.NewRingQueue[QueueTask](AppConfig.DbQueueSize),
		closeCh:    make(chan struct{}),
	}
	ctx.wg.Add(1)
	go ctx.actionWorker()
	return ctx
}

func (db *DbCtx) Close() {
	close(db.closeCh)
	db.writeQueue.Close()
	db.wg.Wait()
	//_ = db.Db.Close()
	db.Db = nil
}

func (db *DbCtx) actionWorker() {
	defer db.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			LogError("[Db] panic in worker: %v", r)
		}
	}()

	for {
		select {
		case <-db.closeCh:
			return
		default:
		}

		d, ok := db.writeQueue.WaitDequeue()
		if !ok {
			return
		}

		var r *gorm.DB
		switch d.taskType {
		case TTypeInsert:
			r = db.Db.Create(d.taskData)
		case TTypeUpdate:
			r = db.Db.Model(d.taskData).Updates(d.taskData)
		case TTypeRemove:
		case TTypeDelete:
			r = db.Db.Delete(d.taskData)
		case TTypeUnknown:
			r = nil
		}

		if r == nil || r.Error != nil {
			LogError("[Db] run queue task failed [type: %v, error: %v]", d.taskType, r)
		}

		if d.resultCh != nil {
			var err error
			if r != nil {
				err = r.Error
			} else {
				err = errors.New("nil db result")
			}

			select {
			case d.resultCh <- err:
			default:
				// Skip blocked
			}
		}
	}
}

func (db *DbCtx) submit(taskType TaskType, data interface{}) error {
	ch := make(chan error, 1)
	tsk := QueueTask{
		taskType: taskType,
		taskData: data,
		resultCh: ch,
	}
	err := db.writeQueue.Enqueue(tsk)
	if err != nil {
		return err
	}
	return <-ch
}

func (db *DbCtx) Insert(data interface{}) error {
	return db.submit(TTypeInsert, data)
}

func (db *DbCtx) Update(data interface{}) error {
	return db.submit(TTypeUpdate, data)
}

func (db *DbCtx) Remove(data interface{}) error {
	return db.submit(TTypeRemove, data)
}

func (db *DbCtx) Delete(data interface{}) error {
	return db.submit(TTypeDelete, data)
}
