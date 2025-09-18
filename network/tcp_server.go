package network

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"net"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type (
	TcpServer struct {
		running int32
		ctx     context.Context
		cancel  context.CancelFunc
		ln      net.Listener

		Decoder               IDecode
		Framer                IFramer
		Router                Router
		IdleTimeout           time.Duration
		SessionHandler        ISessionHandler
		ConnectIdentifyParser IConnectIdentifyParser

		BufPool *BufPool
		clients sync.Map // key: connectId string, val: *ConnectContext
	}

	ConnectContext struct {
		ConnectId             uuid.UUID
		lastActive            time.Time
		Conn                  net.Conn
		Running               bool
		MessageRouter         Router
		ConnectIdentifyParser IConnectIdentifyParser
		Session               map[string]any
		BufPool               *BufPool
		ServerFramer          IFramer
		writeMu               sync.Mutex
		sessionMu             sync.RWMutex
	}
)

func (t *TcpServer) Start(addr string) {
	if t.Decoder == nil {
		t.Decoder = &DefaultDecoder{}
	}
	if t.Router == nil {
		t.Router = &MessageRouter{}
	}
	if t.Framer == nil {
		t.Framer = &DefaultFramer{}
	}
	if t.BufPool == nil {
		t.BufPool = NewBufPool()
	}

	printMessageHandler()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("Error starting TCP server:", err)
		return
	}
	t.ln = ln
	t.ctx, t.cancel = context.WithCancel(context.Background())
	atomic.StoreInt32(&t.running, 1)
	t.clients = sync.Map{}

	logger.Info("Server started on:", addr)
	system.RegisterExitHandler(t)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				logger.Error("Accept loop panic:", r)
				logger.Error("Stack trace:", string(buf[:n]))
			}
		}()

		for atomic.LoadInt32(&t.running) == 1 {
			conn, err := ln.Accept()
			if err != nil {
				// 关闭 listener 后 Accept 会返回错误，判断是否在关停
				if atomic.LoadInt32(&t.running) == 0 {
					return
				}
				logger.Error("Error accepting connection:", err)
				continue
			}
			go handleConnection(t.ctx, conn, t)
		}
	}()
}

func (t *TcpServer) OnSystemExit() {
	// 幂等停止
	if !atomic.CompareAndSwapInt32(&t.running, 1, 0) {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	if t.ln != nil {
		_ = t.ln.Close() // 解除 Accept 阻塞
	}

	// 关闭所有连接
	t.clients.Range(func(key, value any) bool {
		t.CloseContext(value.(*ConnectContext))
		return true
	})
	logger.Info("TcpServer released")
}

func (t *TcpServer) GetConnectContext(connectId string) IConnectContext {
	if connectId == "" {
		return nil
	}
	if v, ok := t.clients.Load(connectId); ok {
		if cc, ok2 := v.(*ConnectContext); ok2 {
			return cc
		}
	}
	return nil
}

func handleConnection(_ context.Context, conn net.Conn, t *TcpServer) {
	// 顶层保护，避免单连接异常影响整体
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			logger.Error("handleConnection panic:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()

	// TCP 优化
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	client := &ConnectContext{
		Conn:                  conn,
		ConnectId:             uuid.New(),
		Running:               true,
		lastActive:            time.Now(),
		MessageRouter:         t.Router,
		Session:               make(map[string]any),
		ConnectIdentifyParser: t.ConnectIdentifyParser,
		BufPool:               t.BufPool,
		ServerFramer:          t.Framer,
	}
	t.clients.Store(client.ConnectId.String(), client)
	logger.Debug("New client connected:", conn.RemoteAddr(), ", ConnectId:", client.ConnectId)

	if t.SessionHandler != nil {
		t.SessionHandler.OnAccept(client)
	}

	defer t.CloseContext(client)

	for client.Running && atomic.LoadInt32(&t.running) == 1 {
		// 用 ReadDeadline 控 idle（你的策略：超时=断开）
		if t.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(t.IdleTimeout))
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}

		// 读取一帧（body = 业务头6B + payload）
		array, ok, err := t.Framer.DeFrame(conn, t.BufPool)
		if !ok {
			// 你的既定策略：读超时也断开
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if t.SessionHandler != nil {
					t.SessionHandler.OnIdleTimeout(client)
				}
			}
			logger.Debug("Connection lost, stopping client:", client.ConnectId, ", err:", err)
			return
		}
		client.lastActive = time.Now()

		// 每帧作用域，确保 array 必定归还
		func() {
			defer t.BufPool.Put(array)

			dataMessage := t.Decoder.Decode(t.BufPool, array)

			// 健壮性：解码失败直接断开
			if dataMessage == nil || dataMessage.Header == nil {
				logger.Error("Decode failed; closing client:", client.ConnectId)
				client.Running = false
				return
			}

			logger.Debug("Received data message, header:", dataMessage.Header, ", length:", len(dataMessage.Msg), ", from:", client.ConnectId)
			client.Execute(dataMessage)

			// ✅ 统一通过 Close() 回收 payload（避免重复 Put）
			dataMessage.Close()
		}()
	}
}

func (t *TcpServer) CloseContext(connectContext IConnectContext) {
	if connectContext == nil {
		return
	}
	// 幂等：只有第一次删除成功才继续收尾
	if _, loaded := t.clients.LoadAndDelete(connectContext.GetConnectId()); !loaded {
		return
	}
	client, ok := connectContext.(*ConnectContext)
	if !ok {
		return
	}

	client.Running = false
	_ = client.Conn.Close()

	if t.SessionHandler != nil {
		t.SessionHandler.OnClose(connectContext)
	}
	logger.Debug("ConnectContext closed:", connectContext.GetConnectId())
}

func printMessageHandler() {
	for k, v := range MessageHandler {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		fv := rv.FieldByName("handlerName")

		if fv.IsValid() && fv.Kind() == reflect.String {
			logger.Info("Registered message handler: msgId:", k, ", type:", fv.String())
		} else {
			logger.Info("Registered message handler: msgId:", k, ", type:", reflect.TypeOf(v).String())
		}
	}
}

func (c *ConnectContext) Execute(message *DataMessage) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			logger.Error("ConnectContext error recovered:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()

	var identify any
	if c.ConnectIdentifyParser != nil {
		id, err := c.ConnectIdentifyParser.ParseConnectIdentify(c)
		if err != nil {
			logger.Error("Error parsing connect identify:", err)
			return
		}
		identify = id
	}
	c.MessageRouter.Route(identify, c, message)
}

func (c *ConnectContext) SendMessage(msg *DataMessage) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer c.Conn.SetWriteDeadline(time.Time{})

	c.writeMu.Lock()
	n, err := c.ServerFramer.WriteFrame(c.Conn, msg)
	c.writeMu.Unlock()

	if err != nil {
		logger.Error("Send error:", err, ", bytes:", n)
		// 写失败视为连接异常：立刻关闭，促使读循环退出并由 defer 清理会话
		_ = c.Conn.Close()
		c.Running = false
		return
	}
	logger.Debug("Send to:", c.ConnectId, ", bytes:", n, ", header:", msg.Header.ToString())
}

func (c *ConnectContext) WriteSession(key string, value any) {
	c.sessionMu.Lock()
	c.Session[key] = value
	c.sessionMu.Unlock()
}
func (c *ConnectContext) GetSession(key string) any {
	c.sessionMu.RLock()
	v := c.Session[key]
	c.sessionMu.RUnlock()
	return v
}
func (c *ConnectContext) DeleteSession(key string) {
	c.sessionMu.Lock()
	delete(c.Session, key)
	c.sessionMu.Unlock()
}

func (c *ConnectContext) GetConnectId() string {
	return c.ConnectId.String()
}
