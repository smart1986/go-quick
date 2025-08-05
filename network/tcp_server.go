package network

import (
	"github.com/google/uuid"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"net"
	"reflect"
	"runtime"
	"sync"
	"time"
)

type (
	TcpServer struct {
		running             bool
		SocketHandlerPacket IHandlerPacket
		Encoder             IEncode
		Decoder             IDecode
		Router              Router
		IdleTimeout         time.Duration
		clients             sync.Map
		SessionHandler      ISessionHandler
	}
	ConnectContext struct {
		ConnectId     uuid.UUID
		lastActive    time.Time
		Conn          net.Conn
		Running       bool
		Encoder       IEncode
		PacketHandler IHandlerPacket
		MessageRouter Router
		Session       map[string]interface{}
	}

	ISessionHandler interface {
		OnAccept(context *ConnectContext)
		OnClose(context *ConnectContext)
		OnIdleTimeout(context *ConnectContext)
	}
)

func (t *TcpServer) OnSystemExit() {
	t.running = false
	t.clients.Range(func(key, value interface{}) bool {
		client := value.(*ConnectContext)
		client.Running = false
		if client.Conn != nil {
			client.Conn.Close()
			if t.SessionHandler != nil {
				t.SessionHandler.OnClose(client)
			}
		}
		return true
	})
	logger.Info("TcpServer released")
}

func (t *TcpServer) Start(c *config.Config) {
	if t.SocketHandlerPacket == nil {
		panic("SocketHandlerPacket must be provided")
	}
	if t.Encoder == nil {
		panic("Encoder must be provided")
	}
	if t.Decoder == nil {
		panic("Decoder must be provided")
	}
	if t.Router == nil {
		panic("Router must be provided")
	}
	printMessageHandler()
	listener, err := net.Listen("tcp", c.Server.Addr)
	if err != nil {
		logger.Error("Error starting TCP server:", err)
		return
	}
	logger.Info("Server started on :", c.Server.Addr)
	t.running = true
	t.clients = sync.Map{}

	// Start a global ticker to check for timeouts
	if t.IdleTimeout > 0 {
		go t.checkTimeouts()
	}
	system.RegisterExitHandler(t)
	go func() {
		for t.running {
			conn, err := listener.Accept()
			if err != nil {
				logger.Error("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, t)
		}
	}()

}
func (t *TcpServer) GetConnectContext(connectId string) *ConnectContext {
	if connectId == "" {
		return nil
	}
	context, ok := t.clients.Load(connectId)
	if !ok {
		logger.Error("ConnectContext not found for ConnectId:", connectId)
		return nil
	}
	client, ok := context.(*ConnectContext)
	if !ok {
		logger.Error("Invalid ConnectContext type for ConnectId:", connectId)
		return nil
	}
	return client

}

func (t *TcpServer) checkTimeouts() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		t.clients.Range(func(key, value interface{}) bool {
			client := value.(*ConnectContext)
			if time.Since(client.lastActive) > t.IdleTimeout {
				logger.Debug("ConnectContext timed out:", client.Conn.RemoteAddr())
				client.Running = false
				client.Conn.Close()
				t.clients.Delete(key)
				if t.SessionHandler != nil {
					t.SessionHandler.OnIdleTimeout(client)
				}
			}
			return true
		})
	}
}

func (t *TcpServer) CloseContext(connectContext *ConnectContext) {
	connectContext.Running = false
	connectContext.Conn.Close()
	t.clients.Delete(connectContext.ConnectId.String())
	if t.SessionHandler != nil {
		t.SessionHandler.OnClose(connectContext)
	}
}

func handleConnection(conn net.Conn, t *TcpServer) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic:", r)
		}
		err := conn.Close()
		if err != nil {
			logger.Error("Error closing connection:", err)
		}
	}()

	client := &ConnectContext{
		Conn:          conn,
		ConnectId:     uuid.New(),
		Running:       true,
		lastActive:    time.Now(),
		Encoder:       t.Encoder,
		PacketHandler: t.SocketHandlerPacket,
		MessageRouter: t.Router,
		Session:       make(map[string]interface{}),
	}
	t.clients.Store(client.ConnectId.String(), client)
	logger.Debug("New client connected:", conn.RemoteAddr(), ", ConnectId:", client.ConnectId)
	if t.SessionHandler != nil {
		t.SessionHandler.OnAccept(client)
	}

	defer func() {
		t.CloseContext(client)

	}()

	for client.Running {
		array, done := t.SocketHandlerPacket.HandlePacket(conn)
		if !done {
			logger.Error("Connection lost, stopping client:", client.ConnectId)
			client.Running = false
			return
		}

		client.lastActive = time.Now()

		dataMessage := t.Decoder.Decode(array)
		logger.Debug("Received data message:", dataMessage)
		client.Execute(dataMessage)

	}
}

func printMessageHandler() {
	for k, v := range MessageHandler {
		logger.Info("register msgId:", k, ",handler:", reflect.ValueOf(v).Type())
	}
}

func (c *ConnectContext) Execute(message *DataMessage) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1024)
			n := runtime.Stack(buf, false)
			logger.Error("ConnectContext error recovered:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()
	c.MessageRouter.Route(c, message)
}

func (c *ConnectContext) SendMessage(msg *DataMessage) {
	var id = c.ConnectId
	encode := c.Encoder.Encode(msg)
	packet, err := c.PacketHandler.ToPacket(encode)
	if err != nil {
		logger.Error("Error marshalling message:", err)
		return
	}
	_, err1 := c.Conn.Write(packet)
	logger.Debug("Send data to client:", id, ",len:", len(packet))
	if err1 != nil {
		logger.Error("Error sending data:", err1)
	}

}
