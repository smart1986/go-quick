package util

import (
	"github.com/panjf2000/ants/v2"
	"github.com/smart1986/go-quick/logger"
)

type WorkerPool struct {
	*ants.Pool
}

func (w *WorkerPool) OnSystemExit() {
	w.Release()
	logger.Info("WorkerPool released")
}
