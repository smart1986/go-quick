package network

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type (
	WSServer struct {
		OriginPatterns  []string
		Schema          string // ws 或 wss
		CertFile        string // 证书文件，wss 必须
		KeyFile         string // 私钥文件，wss 必须
		CompressionMode websocket.CompressionMode

		Decoder               IDecode
		Framer                IFramer
		Router                Router
		IdleTimeout           time.Duration
		SessionHandler        ISessionHandler
		ConnectIdentifyParser IConnectIdentifyParser

		BufPool *BufPool

		clients sync.Map
	}

	WSConnectContext struct {
		ConnectId             uuid.UUID
		lastActive            time.Time
		Conn                  *websocket.Conn
		Running               bool
		MessageRouter         Router
		ConnectIdentifyParser IConnectIdentifyParser
		Session               map[string]interface{}
		Framer                IFramer
		BufPool               *BufPool
		writeMu               sync.Mutex
		sessionMu             sync.RWMutex
	}
)

func (wsc *WSConnectContext) Execute(msg *DataMessage) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			logger.Error("ConnectContext error recovered:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()

	var identify interface{}
	if wsc.ConnectIdentifyParser != nil {
		id, err := wsc.ConnectIdentifyParser.ParseConnectIdentify(wsc)
		if err != nil {
			logger.Error("Error parsing connect identify:", err)
			return
		}
		identify = id
	}
	wsc.MessageRouter.Route(identify, wsc, msg)
}
func (wsc *WSConnectContext) SendMessage(msg *DataMessage) {
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	frame := wsc.Framer.CreateFrame(msg)
	wsc.writeMu.Lock()
	err := wsc.Conn.Write(wctx, websocket.MessageBinary, frame)
	wsc.writeMu.Unlock()
	if err != nil {
		logger.Error("WSConnectContext: write message error:", err)
	}

}

func (wsc *WSConnectContext) WriteSession(key string, value interface{}) {
	wsc.sessionMu.Lock()
	wsc.Session[key] = value
	wsc.sessionMu.Unlock()
}
func (wsc *WSConnectContext) GetSession(key string) interface{} {
	wsc.sessionMu.RLock()
	v := wsc.Session[key]
	wsc.sessionMu.RUnlock()
	return v
}
func (wsc *WSConnectContext) DeleteSession(key string) {
	wsc.sessionMu.Lock()
	delete(wsc.Session, key)
	wsc.sessionMu.Unlock()
}

func (wsc *WSConnectContext) GetConnectId() string {
	return wsc.ConnectId.String()
}
func (wss *WSServer) OnSystemExit() {
	wss.clients.Range(func(key, value interface{}) bool {
		wss.CloseContext(value.(*WSConnectContext))
		return true
	})
	logger.Info("WSServer released")
}

func (wss *WSServer) Start(addr string) {
	// 路径前缀（用于 http.HandleFunc）
	if wss.Schema != "/ws" && wss.Schema != "/wss" {
		logger.Warn("WSServer: Schema not set, default to '/ws'")
		wss.Schema = "/ws"
	}
	if wss.Decoder == nil {
		wss.Decoder = &DefaultDecoder{}
	}
	if wss.Router == nil {
		wss.Router = &MessageRouter{}
	}
	if wss.Framer == nil {
		wss.Framer = &DefaultFramer{}
	}
	if wss.BufPool == nil {
		wss.BufPool = NewBufPool()
	}
	http.HandleFunc(wss.Schema, wss.wsHandler)

	useTLS := wss.Schema == "/wss"
	if useTLS {
		if wss.CertFile == "" || wss.KeyFile == "" {
			panic("WSServer: CertFile and KeyFile must be provided for wss")
		}
		go func() {
			logger.Info("WSServer (TLS) started at ", addr, " path=", wss.Schema)
			if err := http.ListenAndServeTLS(addr, wss.CertFile, wss.KeyFile, nil); err != nil {
				panic(err)
			}
		}()
	} else {
		go func() {
			logger.Info("WSServer started at ", addr, " path=", wss.Schema)
			if err := http.ListenAndServe(addr, nil); err != nil {
				panic(err)
			}
		}()
	}
	system.RegisterExitHandler(wss)
}

func (wss *WSServer) wsHandler(wr http.ResponseWriter, req *http.Request) {
	insecureSkipVerify := wss.OriginPatterns == nil || len(wss.OriginPatterns) == 0
	conn, err := websocket.Accept(wr, req, &websocket.AcceptOptions{
		OriginPatterns:     wss.OriginPatterns,
		InsecureSkipVerify: insecureSkipVerify,
		CompressionMode:    wss.CompressionMode,
	})
	if err != nil {
		logger.Error("WSServer: websocket accept error:", err)
		return
	}
	client := &WSConnectContext{
		Conn:                  conn,
		ConnectId:             uuid.New(),
		Running:               true,
		lastActive:            time.Now(),
		MessageRouter:         wss.Router,
		Session:               make(map[string]interface{}),
		ConnectIdentifyParser: wss.ConnectIdentifyParser,
		BufPool:               wss.BufPool,
		Framer:                wss.Framer,
	}

	wss.clients.Store(client.ConnectId.String(), client)
	logger.Debug("New client connected:", req.RemoteAddr, ", ConnectId:", client.ConnectId)

	if wss.SessionHandler != nil {
		wss.SessionHandler.OnAccept(client)
	}

	defer wss.CloseContext(client)

	for {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)
		if wss.IdleTimeout > 0 {
			ctx, cancel = context.WithTimeout(req.Context(), wss.IdleTimeout)
		} else {
			ctx, cancel = context.WithCancel(req.Context()) // 不设超时
		}
		msgType, data, err := conn.Read(ctx)
		cancel()
		if err != nil {
			if wss.SessionHandler != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					wss.SessionHandler.OnIdleTimeout(client)
				}
			}
			logger.Debug("Connection lost, stopping client:", client.ConnectId)
			return
		}
		if msgType == websocket.MessageText {
			// 忽略文本消息
			logger.Warn("ignore text message")
			continue
		}
		if msgType != websocket.MessageBinary {
			continue
		}

		dataMessage := wss.Decoder.Decode(wss.BufPool, data)

		// 健壮性：解码失败直接断开
		if dataMessage == nil || dataMessage.Header == nil {
			logger.Error("Decode failed; closing client:", client.ConnectId)
			return
		}

		logger.Debug("Received data message, header:", dataMessage.Header, ", length:", len(dataMessage.Msg))
		client.Execute(dataMessage)
		wss.BufPool.Put(dataMessage.Msg)

	}
}

func (wss *WSServer) CloseContext(connectCtx IConnectContext) {
	if connectCtx == nil {
		return
	}
	// 幂等：只有第一次删除成功才继续收尾
	if _, loaded := wss.clients.LoadAndDelete(connectCtx.GetConnectId()); !loaded {
		return
	}
	client, ok := connectCtx.(*WSConnectContext)
	if !ok {
		return
	}
	client.Running = false
	_ = client.Conn.Close(websocket.StatusNormalClosure, "closing connection")

	if wss.SessionHandler != nil {
		wss.SessionHandler.OnClose(client)
	}
	logger.Debug("ConnectContext closed:", client.ConnectId)
}

func (wss *WSServer) GetConnectContext(connectId string) IConnectContext {
	if connectId == "" {
		return nil
	}
	if v, ok := wss.clients.Load(connectId); ok {
		if cc, ok2 := v.(*WSConnectContext); ok2 {
			return cc
		}
	}
	return nil
}
