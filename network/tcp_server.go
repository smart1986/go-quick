package network

import (
	"context"
	"github.com/google/uuid"
	"github.com/smart1986/go-quick/config"
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

		SocketHandlerPacket   IHandlerPacket
		Decoder               IDecode
		Framer                IFramer
		Router                Router
		IdleTimeout           time.Duration
		SessionHandler        ISessionHandler
		ConnectIdentifyParser IConnectIdentifyParser

		BufPool *BufPool
		clients sync.Map
	}
	ConnectContext struct {
		ConnectId             uuid.UUID
		lastActive            time.Time
		Conn                  net.Conn
		Running               bool
		PacketHandler         IHandlerPacket
		MessageRouter         Router
		ConnectIdentifyParser IConnectIdentifyParser
		Session               map[string]interface{}
		BufPool               *BufPool
		ServerFramer          IFramer
	}

	ISessionHandler interface {
		OnAccept(context *ConnectContext)
		OnClose(context *ConnectContext)
		OnIdleTimeout(context *ConnectContext)
	}
)

func (t *TcpServer) Start(c *config.Config) {
	if t.SocketHandlerPacket == nil {
		panic("SocketHandlerPacket must be provided")
	}
	if t.Decoder == nil {
		panic("Decoder must be provided")
	}
	if t.Router == nil {
		panic("Router must be provided")
	}
	if t.Framer == nil {
		t.Framer = &DefaultFramer{}
	}
	if t.BufPool == nil {
		t.BufPool = NewBufPool()
	}

	printMessageHandler()

	ln, err := net.Listen("tcp", c.Server.Addr)
	if err != nil {
		logger.Error("Error starting TCP server:", err)
		return
	}
	t.ln = ln
	t.ctx, t.cancel = context.WithCancel(context.Background())
	atomic.StoreInt32(&t.running, 1)
	t.clients = sync.Map{}

	logger.Info("Server started on:", c.Server.Addr)
	system.RegisterExitHandler(t)

	go func() {
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
	t.clients.Range(func(key, value interface{}) bool {
		t.CloseContext(value.(*ConnectContext))
		return true
	})
	logger.Info("TcpServer released")
}

func (t *TcpServer) GetConnectContext(connectId string) *ConnectContext {
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

func handleConnection(ctx context.Context, conn net.Conn, t *TcpServer) {
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
		PacketHandler:         t.SocketHandlerPacket,
		MessageRouter:         t.Router,
		Session:               make(map[string]interface{}),
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
		// 用 ReadDeadline 控 idle，避免 ticker + 数据竞争
		if t.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(t.IdleTimeout))
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}

		// 读取一帧（body = 业务头6B + payload）
		array, ok := t.SocketHandlerPacket.HandlePacket(conn, t.BufPool)
		if !ok {
			logger.Debug("Connection lost, stopping client:", client.ConnectId)
			return
		}
		client.lastActive = time.Now()

		dataMessage := t.Decoder.Decode(t.BufPool, array)

		// 健壮性：解码失败直接断开
		if dataMessage == nil || dataMessage.Header == nil {
			logger.Error("Decode failed; closing client:", client.ConnectId)
			return
		}

		logger.Debug("Received data message, header:", dataMessage.Header, ", length:", len(dataMessage.Msg))
		client.Execute(dataMessage)
		t.BufPool.Put(array)
	}
}

func (t *TcpServer) CloseContext(connectContext *ConnectContext) {
	if connectContext == nil {
		return
	}
	// 幂等：只有第一次删除成功才继续收尾
	if _, loaded := t.clients.LoadAndDelete(connectContext.ConnectId.String()); !loaded {
		return
	}

	connectContext.Running = false
	_ = connectContext.Conn.Close()

	if t.SessionHandler != nil {
		t.SessionHandler.OnClose(connectContext)
	}
	logger.Debug("ConnectContext closed:", connectContext.ConnectId)
}

func printMessageHandler() {
	for k, v := range MessageHandler {
		logger.Info("register msgId:", k, ",handler:", reflect.ValueOf(v.Handler).Type())
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

	var identify interface{}
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
	// 建议给写入设置一个合理超时（可做成配置）
	_ = c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer c.Conn.SetWriteDeadline(time.Time{})

	n, err := c.ServerFramer.WriteFrame(c.Conn, msg) // 统一走 writev
	if err != nil {
		logger.Error("Send error:", err, ", bytes:", n)
		return
	}
	logger.Debug("Send to:", c.ConnectId, ", bytes:", n, ", header:", msg.Header.ToString())
}
