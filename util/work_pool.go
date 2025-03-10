package util

import "sync"

type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

func NewWorkerPool(workerCount int) *WorkerPool {
	pool := &WorkerPool{
		tasks: make(chan func(), workerCount),
	}
	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		task()
	}
}

func (p *WorkerPool) AddTask(task func()) {
	if task == nil {
		return
	}
	p.tasks <- task
}

func (p *WorkerPool) Close() {
	close(p.tasks)
	p.wg.Wait()
}
