package util

import (
	"github.com/panjf2000/ants/v2"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
)

type WorkerPool struct {
	*ants.Pool
}

func NewPool(size int, options ...ants.Option) *WorkerPool {
	newPool, err := ants.NewPool(size, options...)
	if err != nil {
		panic(err)
	}
	p := &WorkerPool{
		Pool: newPool,
	}
	system.RegisterExitHandler(p)
	return p
}

func (w *WorkerPool) OnSystemExit() {
	w.Release()
	logger.Info("WorkerPool released")
}
