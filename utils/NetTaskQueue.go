package utils

type INetTask interface {
}

type NetTaskQueue[T INetTask] struct {
	queue *RingQueue[T]
}
