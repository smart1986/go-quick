package network

import (
	"context"
	"encoding/binary"
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
		Schema          string // 路由路径：/ws 或 /wss
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

		clients sync.Map // key: connectId string, val: *WSConnectContext
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
			logger.Error("WSConnectContext.Execute recovered:", r)
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

// SendMessage 发送一条业务帧（帧格式：4B总长 + 6B头 + payload）
// 失败时立即关闭连接，促使读循环尽快退出
func (wsc *WSConnectContext) SendMessage(msg *DataMessage) {
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()

	frame := wsc.Framer.CreateFrame(msg) // CreateFrame 应该返回完整业务帧（含 4B totalLen）
	wsc.writeMu.Lock()
	err := wsc.Conn.Write(wctx, websocket.MessageBinary, frame)
	wsc.writeMu.Unlock()
	if err != nil {
		logger.Error("WSConnectContext: write message error:", err)
		_ = wsc.Conn.Close(websocket.StatusAbnormalClosure, "write error")
		wsc.Running = false
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
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			logger.Error("wsHandler panic:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()

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
		// 读：保留你的策略——超时即断开
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
			// 保持原策略：超时也断开
			if wss.SessionHandler != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					wss.SessionHandler.OnIdleTimeout(client)
				}
			}
			logger.Debug("WS connection lost, stopping client:", client.ConnectId, ", err:", err)
			return
		}
		if msgType == websocket.MessageText {
			// 忽略文本消息
			logger.Warn("WS ignore text message")
			continue
		}
		if msgType != websocket.MessageBinary {
			continue
		}

		// === 关键修正：从 data 中按你的业务帧协议解出 0..N 个“6B头+payload”的 body ===
		bodies, defErr := wsExtractBodies(data)
		if defErr != nil {
			logger.Error("WS deframe error:", defErr)
			_ = conn.Close(websocket.StatusProtocolError, "bad frame")
			return
		}

		for _, body := range bodies {
			dataMessage := wss.Decoder.Decode(wss.BufPool, body) // Decode 期望输入为 6B头+payload（不含4B长度）
			if dataMessage == nil || dataMessage.Header == nil {
				logger.Error("Decode failed; closing client:", client.ConnectId)
				return
			}

			client.lastActive = time.Now()
			logger.Debug("Received data message, header:", dataMessage.Header, ", length:", len(dataMessage.Msg))
			client.Execute(dataMessage)

			// 统一通过 Close() 回收 payload（避免重复 Put）
			dataMessage.Close()
		}
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
	logger.Debug("WS ConnectContext closed:", client.ConnectId)
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

// ================== 业务帧解包（支持一条 WS 消息中携带多帧） ==================
//
// data 布局为  [4B totalLen][6B 业务头][payload] [4B totalLen][6B 业务头][payload] ...
// 返回的 bodies 每个元素是「6B头 + payload」（不含 4B totalLen）
func wsExtractBodies(data []byte) ([][]byte, error) {
	var bodies [][]byte
	i := 0
	for i < len(data) {
		if len(data[i:]) < 10 { // 至少 4B 长度 + 6B 业务头
			return nil, errors.New("short frame")
		}
		total := int(binary.BigEndian.Uint32(data[i : i+4]))
		if total < 10 {
			return nil, errors.New("bad total length")
		}
		if len(data[i:]) < total {
			return nil, errors.New("incomplete frame")
		}
		// body = 6B头 + payload
		body := data[i+4 : i+total]
		// 拷贝出独立切片，避免上层持有原 data 的大块内存
		bodies = append(bodies, append([]byte(nil), body...))
		i += total
	}
	return bodies, nil
}
