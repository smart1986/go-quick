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
	ServerURL      string // 如：ws://127.0.0.1:8080/ws  或 wss://host/ws
	Conn           *websocket.Conn
	running        int32
	reconnecting   int32
	mu             sync.RWMutex
	writeMu        sync.Mutex
	MessageHandler func(*DataMessage)
	Decoder        IDecode
	bufPool        *BufPool

	// 可选配置
	DialTimeout  time.Duration // 拨号超时
	IdleTimeout  time.Duration // 读超时(空闲)
	WriteTimeout time.Duration // 写超时

	// 重连参数
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	JitterPercent float64

	// HTTP 头（鉴权等）
	Header http.Header
}

func init() {
	// 给 backoff 抖动做个随机种子
	rand.Seed(time.Now().UnixNano())
}

func NewWSConnector(serverURL string, decoder IDecode, messageHandler func(*DataMessage)) *WSConnector {
	if serverURL == "" || decoder == nil || messageHandler == nil {
		logger.Error("NewWSConnector: invalid args")
		return nil
	}
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
	go c.readLoop()
	return nil
}

func (c *WSConnector) readLoop() {
	defer func() {
		logger.Debug("websocket.readLoop exit")
		c.Close()
	}()

	var buf []byte // 粘包缓存（协议：4B总长 + 6B头 + 载荷）

	for atomic.LoadInt32(&c.running) == 1 {
		c.mu.RLock()
		conn := c.Conn
		c.mu.RUnlock()
		if conn == nil {
			if err := c.reconnectWithBackoff(context.Background()); err != nil {
				logger.Error("WS reconnect failed:", err)
				return
			}
			// 重连成功后，避免老半包污染
			buf = nil
			continue
		}

		// 读超时：用 Context 控制
		readCtx, cancel := context.WithTimeout(context.Background(), chooseTimeout(c.IdleTimeout, 70*time.Second))
		mt, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			logger.Debug("WS read error, will reconnect:", err)
			_ = conn.Close(websocket.StatusAbnormalClosure, "read error")
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()

			if err := c.reconnectWithBackoff(context.Background()); err != nil {
				logger.Error("WS reconnect failed:", err)
				return
			}
			// 重连成功后清理半包
			buf = nil
			continue
		}
		if mt != websocket.MessageBinary {
			// 如需支持文本/其他类型，可在此处理或记录
			logger.Debug("WS non-binary message ignored")
			continue
		}

		buf = join(buf, data)
		frames, remain, err := wsDeframe(buf)
		if err != nil {
			logger.Error("WS deframe error:", err)
			_ = conn.Close(websocket.StatusProtocolError, "bad frame")
			c.mu.Lock()
			if c.Conn == conn {
				c.Conn = nil
			}
			c.mu.Unlock()
			// 立即在下一轮触发重连；同时清空缓冲
			buf = nil
			continue
		}
		buf = remain

		for _, frame := range frames {
			// 注意：frame 是新分配的切片，不应放回 bufPool（避免污染池）
			msg := c.Decoder.Decode(c.bufPool, frame)
			if msg == nil {
				logger.Error("Decoder returned nil; closing")
				_ = conn.Close(websocket.StatusProtocolError, "decode nil")
				c.mu.Lock()
				if c.Conn == conn {
					c.Conn = nil
				}
				c.mu.Unlock()
				break
			}
			if c.MessageHandler != nil {
				c.MessageHandler(msg)
			}
			// 移除：c.bufPool.Put(frame)  —— frame 非池内存，不可 Put
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
			time.Sleep(50 * time.Millisecond)
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
		logger.Debug("WS reconnect attempt", attempt, "in", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
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

	frame, err := wsBuildFrame(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), chooseTimeout(c.WriteTimeout, 5*time.Second))
	defer cancel()

	// 避免多协程并发写
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageBinary, frame)
}

func (c *WSConnector) Close() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return
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
	var bizHdr [6]byte
	binary.BigEndian.PutUint32(bizHdr[0:4], uint32(h.MsgId))
	binary.BigEndian.PutUint16(bizHdr[4:6], uint16(h.Code))
	total := 4 + 6 + len(m.Msg)
	var lenHdr [4]byte
	binary.BigEndian.PutUint32(lenHdr[:], uint32(total))

	out := make([]byte, 0, total)
	out = append(out, bizHdr[:]...)
	out = append(out, m.Msg...)
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
		frames = append(frames, append([]byte(nil), body...))
		i += total
	}
}

// 更高效的 join（避免 bytes.Buffer 额外分配）
func join(a, b []byte) []byte {
	// 若目标是简化，直接用 append；如需保留原语义也可保持 bytes.Buffer
	return append(a, b...)
	// 旧实现保留在此以供对比：
	// var bb bytes.Buffer
	// bb.Grow(len(a) + len(b))
	// bb.Write(a)
	// bb.Write(b)
	// return bb.Bytes()
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
