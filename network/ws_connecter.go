package network

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/smart1986/go-quick/logger"
)

// =========== WebSocket Connector ===========

type WSConnector struct {
	ServerURL string // ws://host:port/path 或 wss://...
	Conn      *websocket.Conn

	running      int32
	reconnecting int32

	mu      sync.RWMutex // 保护 Conn
	writeMu sync.Mutex   // 串行化写

	MessageHandler func(*DataMessage)
	Decoder        IDecode
	bufPool        *BufPool

	// 可选配置
	DialTimeout  time.Duration // 拨号超时
	IdleTimeout  time.Duration // 读空闲超时（无消息时的等待时长）
	WriteTimeout time.Duration // 写超时

	// 重连参数
	MaxAttempts   int           // -1 = 无限
	BaseDelay     time.Duration // 指数退避基准
	MaxDelay      time.Duration // 退避上限
	JitterPercent float64       // 抖动

	// HTTP 头（鉴权等）
	Header http.Header

	// 生命周期
	startOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewWSConnector(serverURL string, decoder IDecode, messageHandler func(*DataMessage)) *WSConnector {
	if serverURL == "" || decoder == nil || messageHandler == nil {
		logger.Error("NewWSConnector: invalid args")
		return nil
	}
	cctx, cancel := context.WithCancel(context.Background())
	c := &WSConnector{
		ServerURL:      serverURL,
		Decoder:        decoder,
		MessageHandler: messageHandler,
		bufPool:        NewBufPool(),
		DialTimeout:    5 * time.Second,
		IdleTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxAttempts:    -1, // -1 = 无限重连
		BaseDelay:      200 * time.Millisecond,
		MaxDelay:       5 * time.Second,
		JitterPercent:  0.2,
		Header:         http.Header{},
		ctx:            cctx,
		cancel:         cancel,
	}
	atomic.StoreInt32(&c.running, 1)
	return c
}

// 仅负责建立连接并更新 c.Conn，不启动读循环
func (c *WSConnector) dialOnce(ctx context.Context) error {
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: c.Header})
	if err != nil {
		return fmt.Errorf("ws dial %s failed: %w", c.ServerURL, err)
	}
	c.mu.Lock()
	c.Conn = conn
	c.mu.Unlock()
	logger.Info("WS connected:", c.ServerURL)
	return nil
}

func (c *WSConnector) Connect() error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("connector not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.DialTimeout)
	defer cancel()
	if err := c.dialOnce(ctx); err != nil {
		return err
	}
	c.startOnce.Do(func() { go c.readLoop() })
	return nil
}

func (c *WSConnector) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("websocket.readLoop panic:", r)
		}
		logger.Debug("websocket.readLoop exit")
		c.Close()
	}()

	var buf []byte // 粘包缓存（协议：4B总长 + 6B头 + 载荷）

	for atomic.LoadInt32(&c.running) == 1 {
		c.mu.RLock()
		conn := c.Conn
		c.mu.RUnlock()
		if conn == nil {
			if err := c.reconnectWithBackoff(c.ctx); err != nil {
				if atomic.LoadInt32(&c.running) == 0 {
					return
				}
				logger.Error("WS reconnect failed:", err)
				return
			}
			buf = nil // 重连成功后清半包
			continue
		}

		// 读：用 context 控制超时（无消息时希望“继续等”，不应重连）
		readCtx, cancel := context.WithTimeout(context.Background(), chooseTimeout(c.IdleTimeout, 70*time.Second))
		mt, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			// 空闲超时：只是没有消息，继续读即可
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			// 如果是对端关闭/正常关闭，也会在 CloseStatus 有代码
			code := websocket.CloseStatus(err)
			if code != -1 {
				logger.Debug("WS closed by peer, code:", code, "err:", err)
			} else {
				logger.Debug("WS read error, will reconnect:", err)
			}
			_ = conn.Close(websocket.StatusAbnormalClosure, "read error")
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()
			// 下一轮触发重连
			buf = nil
			continue
		}
		if mt != websocket.MessageBinary {
			// 如需支持文本/其他类型，可在此处理
			logger.Debug("WS non-binary message ignored")
			continue
		}

		// 按业务帧协议拼接/切分
		buf = join(buf, data)
		frames, remain, defErr := wsDeframe(buf)
		if defErr != nil {
			logger.Error("WS deframe error:", defErr)
			_ = conn.Close(websocket.StatusProtocolError, "bad frame")
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()
			buf = nil
			continue
		}
		buf = remain

		for _, frame := range frames {
			// frame 是新分配的切片（append(nil, body...)），不是池内存
			msg := c.Decoder.Decode(c.bufPool, frame)
			if msg == nil {
				logger.Error("Decoder returned nil; closing ws")
				_ = conn.Close(websocket.StatusProtocolError, "decode nil")
				c.mu.Lock()
				if c.Conn == conn {
					c.Conn = nil
				}
				c.mu.Unlock()
				break
			}
			if c.MessageHandler != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("MessageHandler panic:", r)
						}
						// 统一回收 Decode 产生的 payload
						if msg.Msg != nil {
							msg.Close()
						}
					}()
					c.MessageHandler(msg)
				}()
			} else {
				if msg.Msg != nil {
					msg.Close()
				}
			}
		}
	}
}

func (c *WSConnector) reconnectWithBackoff(ctx context.Context) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("not running")
	}
	if !atomic.CompareAndSwapInt32(&c.reconnecting, 0, 1) {
		// 已在重连中，等待其完成
		for atomic.LoadInt32(&c.reconnecting) == 1 && atomic.LoadInt32(&c.running) == 1 {
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
	defer atomic.StoreInt32(&c.reconnecting, 0)

	attempt := 0
	for atomic.LoadInt32(&c.running) == 1 && (c.MaxAttempts < 0 || attempt < c.MaxAttempts) {
		attempt++
		dctx, cancel := context.WithTimeout(ctx, chooseTimeout(c.DialTimeout, 5*time.Second))
		err := c.dialOnce(dctx)
		cancel()
		if err == nil {
			return nil
		}
		backoff := expBackoff(c.BaseDelay, c.MaxDelay, attempt)
		backoff = addJitter(backoff, c.JitterPercent)
		logger.Debug("WS reconnect attempt", attempt, "in", backoff, "err:", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
	return fmt.Errorf("ws reconnect exhausted after %d attempts", attempt)
}

func (c *WSConnector) SendMessage(msg *DataMessage) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return errors.New("connector stopped")
	}
	c.mu.RLock()
	conn := c.Conn
	c.mu.RUnlock()
	if conn == nil {
		return errors.New("no active connection")
	}

	// 业务帧编码（包含 4B totalLen）
	frame, err := wsBuildFrame(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), chooseTimeout(c.WriteTimeout, 5*time.Second))
	defer cancel()

	// 串行化写，避免帧交叠
	c.writeMu.Lock()
	err = conn.Write(ctx, websocket.MessageBinary, frame)
	c.writeMu.Unlock()

	if err != nil {
		// 写失败：立刻关闭并让读循环触发重连
		logger.Error("WS send error:", err)
		c.mu.Lock()
		if c.Conn == conn {
			_ = c.Conn.Close(websocket.StatusAbnormalClosure, "write error")
			c.Conn = nil
		}
		c.mu.Unlock()
		return err
	}
	return nil
}

func (c *WSConnector) Close() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return
	}
	if c.cancel != nil {
		c.cancel() // 取消任何等待中的退避
	}
	atomic.StoreInt32(&c.reconnecting, 0)

	c.mu.Lock()
	if c.Conn != nil {
		_ = c.Conn.Close(websocket.StatusNormalClosure, "client close")
		c.Conn = nil
	}
	c.mu.Unlock()
}

// =========== 工具/帧编解码（保持协议不变） ===========

// 帧格式: [0:4)=totalLen(含自身) + [4:8)=MsgId + [8:10)=Code + [10:)=payload
func wsBuildFrame(m *DataMessage) ([]byte, error) {
	h, ok := m.Header.(*DataHeader)
	if !ok {
		return nil, fmt.Errorf("invalid header type")
	}
	total := 4 + 6 + len(m.Msg)

	out := make([]byte, total)
	// 写 4B 长度
	binary.BigEndian.PutUint32(out[0:4], uint32(total))
	// 写 6B 业务头
	binary.BigEndian.PutUint32(out[4:8], uint32(h.MsgId))
	binary.BigEndian.PutUint16(out[8:10], uint16(h.Code))
	// 写 payload
	copy(out[10:], m.Msg)
	return out, nil
}

// wsDeframe: 可反复喂入，解析出 0..N 帧（仅返回 6B头+payload，不含前导4B长度）
func wsDeframe(buf []byte) (frames [][]byte, remain []byte, err error) {
	i := 0
	for {
		if len(buf[i:]) < 4 {
			return frames, buf[i:], nil
		}
		total := int(binary.BigEndian.Uint32(buf[i : i+4]))
		if total < 10 { // 4B len + 6B 头
			return nil, nil, fmt.Errorf("bad total: %d", total)
		}
		if len(buf[i:]) < total {
			return frames, buf[i:], nil
		}
		body := buf[i+4 : i+total] // 仅 body (6B头+payload)
		// 复制一份独立切片，避免引用到不断增长的 buf
		frames = append(frames, append([]byte(nil), body...))
		i += total
	}
}

// 更高效的 join（避免额外分配逻辑的复杂度）
func join(a, b []byte) []byte {
	return append(a, b...)
}

func expBackoff(base, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	if max <= 0 {
		max = 5 * time.Second
	}
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt-1)))
	if d > max {
		d = max
	}
	return d
}

func addJitter(d time.Duration, rate float64) time.Duration {
	if d <= 0 || rate <= 0 {
		return d
	}
	span := int64(float64(d) * rate)
	if span == 0 {
		return d
	}
	off := rand.Int63n(span*2+1) - span
	return time.Duration(int64(d) + off)
}

func chooseTimeout(pref, fallback time.Duration) time.Duration {
	if pref > 0 {
		return pref
	}
	return fallback
}
