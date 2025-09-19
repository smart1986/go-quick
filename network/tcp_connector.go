package network

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smart1986/go-quick/logger"
)

// Connector 管理 TCP 连接、读写循环与自动重连
type Connector struct {
	ServerAddr string

	mu        sync.RWMutex // 保护 Conn
	Conn      net.Conn
	writeMu   sync.Mutex // 串行化写，避免帧交叠
	running   int32      // 1=运行中, 0=已停止
	reconning int32      // 1=正在重连

	// 依赖
	MessageHandler func(*DataMessage)
	Decoder        IDecode
	Framer         IFramer
	bufPool        *BufPool

	// 参数
	DialTimeout  time.Duration // 拨号超时
	IdleTimeout  time.Duration // 读空闲超时（ReadDeadline）
	WriteTimeout time.Duration // 写超时（WriteDeadline）

	// 生命周期控制
	startOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewConnector 要求：serverAddr、decoder、framer、messageHandler 必须提供
func NewConnector(serverAddr string, decoder IDecode, framer IFramer, messageHandler func(message *DataMessage)) *Connector {
	switch {
	case serverAddr == "":
		logger.Error("NewConnector: serverAddr required")
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
	cctx, cancel := context.WithCancel(context.Background())
	c := &Connector{
		ServerAddr:     serverAddr,
		Decoder:        decoder,
		Framer:         framer,
		MessageHandler: messageHandler,
		bufPool:        NewBufPool(),
		DialTimeout:    5 * time.Second,
		IdleTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
		ctx:            cctx,
		cancel:         cancel,
	}
	atomic.StoreInt32(&c.running, 1)
	return c
}

// 仅负责拨号建连并设置 TCP 选项，不启动 readLoop
func (c *Connector) dialOnce(ctx context.Context) (net.Conn, error) {
	if atomic.LoadInt32(&c.running) == 0 {
		return nil, errors.New("connector is not running")
	}
	dialer := &net.Dialer{Timeout: c.DialTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("connect %s failed: %w", c.ServerAddr, err)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}
	return conn, nil
}

// Connect 拨号+启动读循环（幂等：readLoop 只会启动一次）
func (c *Connector) Connect() error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("connector is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.DialTimeout)
	defer cancel()

	conn, err := c.dialOnce(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.Conn = conn
	c.mu.Unlock()
	logger.Info("Connected to server:", c.ServerAddr)

	c.startOnce.Do(func() { go c.readLoop() })
	return nil
}

// readLoop：持续读取帧 -> 解码 -> 回调 MessageHandler
func (c *Connector) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("readLoop panic:", r)
		}
		logger.Debug("readLoop exit")
		c.Close()
	}()

	for atomic.LoadInt32(&c.running) == 1 {
		// 获取当前连接
		c.mu.RLock()
		conn := c.Conn
		c.mu.RUnlock()

		if conn == nil {
			// 尚未建立或已断开，尝试重连（只拨号，不再起新的 readLoop）
			if err := c.reconnectWithBackoff(c.ctx); err != nil {
				if atomic.LoadInt32(&c.running) == 0 {
					return
				}
				logger.Error("Reconnect failed:", err)
				// 发生不可恢复错误，退出
				return
			}
			continue
		}

		// 读超时作为 idle 控制，避免永久阻塞
		if c.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(c.IdleTimeout))
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}

		// 读取完整帧（DeFrame 语义：返回 body（来自池）、ok、err；调用方负责 Put(body)）
		array, ok, herr := c.Framer.DeFrame(conn, c.bufPool)

		// 错误分类
		if herr != nil {
			// 超时：继续循环（空闲）
			var ne net.Error
			if errors.As(herr, &ne) && ne.Timeout() {
				continue
			}
			// 其他错误（EOF/closed/网络错/协议错）：按断开处理
		}

		if !ok || herr != nil {
			// 连接不可用：关闭当前连接并置空，然后重连
			_ = conn.Close()
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()

			if err := c.reconnectWithBackoff(c.ctx); err != nil {
				if atomic.LoadInt32(&c.running) == 0 {
					return
				}
				logger.Error("Reconnect failed:", err)
				return
			}
			continue
		}

		// 针对本帧建立作用域，确保 defer 在每帧结束就执行（不拖到循环结束）
		func() {
			// 总是归还 DeFrame 拿到的 array
			defer c.bufPool.Put(array)

			// 正常收到数据，解码（Decode 会拷贝 payload 到新缓冲（来自池））
			msg := c.Decoder.Decode(c.bufPool, array)
			if msg == nil {
				logger.Error("Decoder returned nil; closing current connection")
				_ = conn.Close()
				c.mu.Lock()
				if c.Conn == conn {
					c.Conn = nil
				}
				c.mu.Unlock()
				// 触发重连由下一轮循环完成
				return
			}

			// 调用回调；确保用完后归还 msg.Msg（Decode 文档要求上层 Put）
			if c.MessageHandler != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("MessageHandler panic:", r)
						}
						// 统一回收 payload
						if msg.Msg != nil {
							msg.Close()
						}
					}()
					c.MessageHandler(msg)
				}()
			} else {
				// 没有回调也要回收 payload
				if msg.Msg != nil {
					msg.Close()
				}
			}
		}()
	}
}

// 指数退避+抖动重连；成功后仅替换连接，由现有 readLoop 继续读取
func (c *Connector) reconnectWithBackoff(ctx context.Context) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("not running")
	}
	// 防止并发重连
	if !atomic.CompareAndSwapInt32(&c.reconning, 0, 1) {
		// 已经有人在重连了：等待其结束
		for atomic.LoadInt32(&c.reconning) == 1 && atomic.LoadInt32(&c.running) == 1 {
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
		}
		if atomic.LoadInt32(&c.running) == 0 {
			return errors.New("stopped")
		}
		return nil
	}
	defer atomic.StoreInt32(&c.reconning, 0)

	const (
		baseDelay     = 200 * time.Millisecond
		maxDelay      = 5 * time.Second
		jitterPercent = 0.2 // ±20%
	)

	for attempt := 0; atomic.LoadInt32(&c.running) == 1; attempt++ {
		dctx, cancel := context.WithTimeout(ctx, c.DialTimeout)
		newConn, err := c.dialOnce(dctx)
		cancel()

		if err == nil {
			// 成功：替换连接；旧连接若仍存活，readLoop 上一轮会关闭它
			c.mu.Lock()
			old := c.Conn
			c.Conn = newConn
			c.mu.Unlock()

			if old != nil && old != newConn {
				_ = old.Close()
			}
			logger.Info("Reconnected to server:", c.ServerAddr)
			return nil
		}

		// 退避 + 抖动（可被 ctx/c.ctx 取消）
		backoff := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if backoff > maxDelay {
			backoff = maxDelay
		}
		j := jitter(backoff, jitterPercent)
		logger.Debug("Reconnect in", j, "err:", err)
		select {
		case <-time.After(j):
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
	return errors.New("stopped")
}

// SendMessage writev 发送；失败返回错误，并主动关闭触发重连（由读循环处理）
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

	// 避免多 goroutine 并发写导致帧交叠
	c.writeMu.Lock()
	n, err := c.Framer.WriteFrame(conn, msg)
	c.writeMu.Unlock()

	if err != nil {
		logger.Error("Send error:", err, ", bytes:", n)
		// 主动触发重连：关闭并置空
		c.mu.Lock()
		if c.Conn == conn {
			_ = c.Conn.Close()
			c.Conn = nil
		}
		c.mu.Unlock()
		return err
	}
	return nil
}

// Close 关闭连接器与当前连接，并取消重连等待
func (c *Connector) Close() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return
	}
	if c.cancel != nil {
		c.cancel() // 取消任何等待中的重连退避
	}
	atomic.StoreInt32(&c.reconning, 0)

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
	// 这里使用 crypto/rand 或 math/rand 都可；如需更快可换 math/rand
	n := time.Now().UnixNano() % ((max * 2) + 1) // 简易、足够的抖动
	off := n - max
	return time.Duration(int64(d) + off)
}

// （可选）简单心跳帧构造示例：4B总长 + 6B业务头 + 空载荷
// 可根据你的 IFramer 实现直接 WriteFrame，而不必手工拼
func buildHeartbeatFrame() []byte {
	const total = 10 // 4(len)+6(header)
	buf := make([]byte, total)
	// binary.BigEndian.PutUint32(buf[0:4], total)
	// header 自行约定（例如 MsgId=0/Code=0）
	return buf
}
