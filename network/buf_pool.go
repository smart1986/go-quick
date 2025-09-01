package network

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type poolWithStats struct {
	pool     sync.Pool
	capacity int
	gets     int64
	puts     int64
}

type BufPool struct {
	pools []*poolWithStats
	quit  chan struct{}
}

func NewBufPool() *BufPool {
	bp := &BufPool{
		quit: make(chan struct{}),
	}

	// 分级：512B / 2KB / 4KB
	sizes := []int{512, 2 * 1024, 4 * 1024}
	for _, size := range sizes {
		p := &poolWithStats{capacity: size}
		p.pool.New = func(sz int) func() interface{} {
			return func() interface{} {
				return make([]byte, sz)
			}
		}(size)
		bp.pools = append(bp.pools, p)
	}

	return bp
}

// Get 根据请求大小返回合适的 buffer
func (bp *BufPool) Get(size int) []byte {
	for _, p := range bp.pools {
		if size <= p.capacity {
			atomic.AddInt64(&p.gets, 1)
			return p.pool.Get().([]byte)[:size]
		}
	}
	// 超过最大，直接分配
	return make([]byte, size)
}

// Put 归还 buffer
func (bp *BufPool) Put(buf []byte) {
	for _, p := range bp.pools {
		if cap(buf) == p.capacity {
			atomic.AddInt64(&p.puts, 1)
			p.pool.Put(buf[:p.capacity])
			return
		}
	}
	// 不是池里的，丢弃
}

// Stats 返回统计信息
func (bp *BufPool) Stats() map[int]map[string]int64 {
	stats := make(map[int]map[string]int64)
	for _, p := range bp.pools {
		gets := atomic.LoadInt64(&p.gets)
		puts := atomic.LoadInt64(&p.puts)
		stats[p.capacity] = map[string]int64{
			"gets": gets,
			"puts": puts,
			"live": gets - puts,
		}
	}
	return stats
}

// StartMonitor 每隔 interval 打印一次统计
func (bp *BufPool) StartMonitor(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Printf("[BufPool Stats] %+v\n", bp.Stats())
			case <-bp.quit:
				return
			}
		}
	}()
}

// StopMonitor 停止监控
func (bp *BufPool) StopMonitor() {
	close(bp.quit)
}
