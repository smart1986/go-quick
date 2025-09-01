package network

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smart1986/go-quick/logger"
)

type Connector struct {
	ServerAddr     string
	Conn           net.Conn
	running        int32 // 原子控制
	reconnecting   int32 // 原子标记：是否处于重连中
	mu             sync.RWMutex
	MessageHandler func(*DataMessage)
	Marshal        IHandlerPacket
	Decoder        IDecode
	bufPool        *BufPool
	Framer         IFramer

	// 可选配置
	DialTimeout  time.Duration // 连接超时
	IdleTimeout  time.Duration // 读超时(空闲)
	WriteTimeout time.Duration // 写超时
}

// NewConnector 要求：serverAddr、marshal、decoder、framer、messageHandler 必须提供
func NewConnector(serverAddr string, marshal IHandlerPacket, decoder IDecode, framer IFramer, messageHandler func(message *DataMessage)) *Connector {
	switch {
	case serverAddr == "":
		logger.Error("NewConnector: serverAddr required")
		return nil
	case marshal == nil:
		logger.Error("NewConnector: marshal(IHandlerPacket) required")
		return nil
	case decoder == nil:
		logger.Error("NewConnector: decoder(IDecode) required")
		return nil
	case framer == nil:
		logger.Error("NewConnector: framer(IFramer) required")
		return nil
	case messageHandler == nil:
		logger.Error("NewConnector: messageHandler required")
		return nil
	}
	c := &Connector{
		ServerAddr:     serverAddr,
		Marshal:        marshal,
		Decoder:        decoder,
		Framer:         framer,
		MessageHandler: messageHandler,
		bufPool:        NewBufPool(),
		DialTimeout:    5 * time.Second,
		IdleTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
	}
	atomic.StoreInt32(&c.running, 1)
	return c
}

func (c *Connector) Connect() error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("connector is not running")
	}
	dialer := &net.Dialer{Timeout: c.DialTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.Dial("tcp", c.ServerAddr)
	if err != nil {
		return fmt.Errorf("connect %s failed: %w", c.ServerAddr, err)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}
	c.mu.Lock()
	c.Conn = conn
	c.mu.Unlock()
	logger.Info("Connected to server:", c.ServerAddr)

	go c.readLoop() // 独立读循环
	return nil
}

// readLoop：持续读取帧 -> 解码 -> 回调 MessageHandler
func (c *Connector) readLoop() {
	defer func() {
		logger.Debug("readLoop exit")
		c.Close()
	}()

	for atomic.LoadInt32(&c.running) == 1 {
		// 读超时作为 idle 控制，避免永久阻塞
		c.mu.RLock()
		conn := c.Conn
		c.mu.RUnlock()
		if conn == nil {
			// 尚未建立或已断开，尝试重连
			if err := c.reconnectWithBackoff(context.Background()); err != nil {
				logger.Error("Reconnect failed:", err)
				return
			}
			continue
		}

		if c.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(c.IdleTimeout))
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}

		// 读取完整帧（不含4字节长度；由 IHandlerPacket 定义）
		array, ok := c.Marshal.HandlePacket(conn, c.bufPool)
		if !ok {
			logger.Debug("Connection lost; will reconnect…")
			_ = conn.Close()
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()
			// 进入重连
			if err := c.reconnectWithBackoff(context.Background()); err != nil {
				logger.Error("Reconnect failed:", err)
				return
			}
			continue
		}

		// 正常收到数据，解码
		msg := c.Decoder.Decode(c.bufPool, array)
		if msg == nil {
			logger.Error("Decoder returned nil; closing")
			_ = conn.Close()
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()
			continue
		}
		if c.MessageHandler != nil {
			c.MessageHandler(msg)
		}
		c.bufPool.Put(array) // 半限制策略：小包回收，大包丢弃
	}
}

// 指数退避+抖动重连；成功后启动新的 readLoop（由 Connect 内部完成）
func (c *Connector) reconnectWithBackoff(ctx context.Context) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("not running")
	}
	// 防止并发重连
	if !atomic.CompareAndSwapInt32(&c.reconnecting, 0, 1) {
		// 已经有人在重连了：等待其结束
		for atomic.LoadInt32(&c.reconnecting) == 1 && atomic.LoadInt32(&c.running) == 1 {
			time.Sleep(50 * time.Millisecond)
		}
		if atomic.LoadInt32(&c.running) == 0 {
			return errors.New("stopped")
		}
		return nil
	}
	defer atomic.StoreInt32(&c.reconnecting, 0)

	const (
		maxAttempts   = 10
		baseDelay     = 200 * time.Millisecond
		maxDelay      = 5 * time.Second
		jitterPercent = 0.2 // ±20%
	)
	var lastErr error
	for attempt := 0; attempt < maxAttempts && atomic.LoadInt32(&c.running) == 1; attempt++ {
		if err := c.Connect(); err == nil {
			return nil
		} else {
			lastErr = err
			// 退避 + 抖动
			backoff := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
			if backoff > maxDelay {
				backoff = maxDelay
			}
			j := jitter(backoff, jitterPercent)
			logger.Debug("Reconnect in", j)
			select {
			case <-time.After(j):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("unknown reconnect error")
	}
	return fmt.Errorf("reconnect exhausted: %w", lastErr)
}

// SendMessage：writev 发送；失败返回错误，不做自动重连（由读循环驱动重连更干净）
func (c *Connector) SendMessage(msg *DataMessage) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("connector stopped")
	}
	c.mu.RLock()
	conn := c.Conn
	c.mu.RUnlock()
	if conn == nil {
		return errors.New("no active connection")
	}

	if c.WriteTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
	} else {
		_ = conn.SetWriteDeadline(time.Time{})
	}
	defer conn.SetWriteDeadline(time.Time{})

	n, err := c.Framer.WriteFrame(conn, msg)
	if err != nil {
		logger.Error("Send error:", err, ", bytes:", n)
		return err
	}
	return nil
}

func (c *Connector) Close() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return
	}
	atomic.StoreInt32(&c.reconnecting, 0)
	c.mu.Lock()
	if c.Conn != nil {
		_ = c.Conn.Close()
		c.Conn = nil
	}
	c.mu.Unlock()
}

// ———— 工具函数 ————

func jitter(d time.Duration, rate float64) time.Duration {
	if d <= 0 || rate <= 0 {
		return d
	}
	// 生成 [-rate, +rate] 范围内的抖动
	max := int64(float64(d) * rate)
	if max == 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max*2+1))
	if err != nil {
		return d
	}
	off := n.Int64() - max
	return time.Duration(int64(d) + off)
}

// （可选）简单心跳帧构造示例：4B总长 + 6B业务头 + 空载荷
// 可根据你的 IFramer 实现直接 WriteFrame，而不必手工拼
func buildHeartbeatFrame() []byte {
	const total = 10 // 4(len)+6(header)
	buf := make([]byte, total)
	binary.BigEndian.PutUint32(buf[0:4], total)
	// header 自行约定（例如 MsgId=0/Code=0）
	return buf
}
