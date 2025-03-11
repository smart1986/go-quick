package util

import (
	"sync"
	"time"
)

type WorkerPool struct {
	tasks           chan func()
	wg              sync.WaitGroup
	workerCount     int
	idleCount       int
	coreWorkerCount int
	maxWorkerCount  int
	maxIdleTime     time.Duration
	mu              sync.Mutex
}

func NewFixWorkerPool(workerCount int) *WorkerPool {
	return NewWorkerPool(workerCount, workerCount, 0)
}

func NewWorkerPool(coreWorkerCount, maxWorkerCount int, maxIdleTime time.Duration) *WorkerPool {
	pool := &WorkerPool{
		tasks:           make(chan func(), maxWorkerCount),
		workerCount:     coreWorkerCount,
		coreWorkerCount: coreWorkerCount,
		maxWorkerCount:  maxWorkerCount,
		maxIdleTime:     maxIdleTime,
	}
	for i := 0; i < coreWorkerCount; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	if maxIdleTime > 0 {
		go pool.monitorIdleWorkers()
	}
	return pool
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	idleTimer := time.NewTimer(p.maxIdleTime)
	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			if task != nil {
				task()
				idleTimer.Reset(p.maxIdleTime)
			}
		case <-idleTimer.C:
			p.mu.Lock()
			if p.workerCount > p.coreWorkerCount {
				p.workerCount--
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
			idleTimer.Reset(p.maxIdleTime)
		}
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

func (p *WorkerPool) Resize(newWorkerCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if newWorkerCount > p.maxWorkerCount {
		newWorkerCount = p.maxWorkerCount
	}

	if newWorkerCount > p.workerCount {
		for i := p.workerCount; i < newWorkerCount; i++ {
			p.wg.Add(1)
			go p.worker()
		}
	} else if newWorkerCount < p.workerCount {
		for i := newWorkerCount; i < p.workerCount; i++ {
			p.tasks <- nil // Send nil task to signal worker to exit
		}
	}
	p.workerCount = newWorkerCount
}

func (p *WorkerPool) monitorIdleWorkers() {
	for {
		time.Sleep(100 * time.Millisecond) // Adjust the interval as needed
		p.mu.Lock()
		idleCount := p.idleCount
		p.mu.Unlock()

		if idleCount > p.workerCount/2 {
			p.Resize(p.workerCount / 2)
		} else if idleCount == 0 {
			p.Resize(p.workerCount * 2)
		}
	}
}
