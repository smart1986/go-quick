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

var (
	Clients sync.Map
)

type TcpServer struct {
	Running             bool
	SocketHandlerPacket IHandlerPacket
	Encoder             IEncode
	Decoder             IDecode
	Router              Router
	IdleTimeout         time.Duration
}

type ConnectContext struct {
	ConnectId     uuid.UUID
	lastActive    time.Time
	Conn          net.Conn
	Running       bool
	Encoder       IEncode
	PacketHandler IHandlerPacket
	MessageRouter Router
	Session       map[string]interface{}
}

func (t *TcpServer) OnSystemExit() {
	t.Running = false
	Clients.Range(func(key, value interface{}) bool {
		client := value.(*ConnectContext)
		client.Running = false
		if client.Conn != nil {
			client.Conn.Close()
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
	t.Running = true

	// Start a global ticker to check for timeouts
	if t.IdleTimeout > 0 {
		go t.checkTimeouts()
	}
	system.RegisterExitHandler(t)
	go func() {
		for t.Running {
			conn, err := listener.Accept()
			if err != nil {
				logger.Error("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, t)
		}
	}()

}

func (t *TcpServer) checkTimeouts() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		Clients.Range(func(key, value interface{}) bool {
			client := value.(*ConnectContext)
			if time.Since(client.lastActive) > t.IdleTimeout {
				logger.Debug("ConnectContext timed out:", client.Conn.RemoteAddr())
				client.Running = false
				client.Conn.Close()
				Clients.Delete(key)
			}
			return true
		})
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

	logger.Debug("New client connected:", conn.RemoteAddr())
	client := &ConnectContext{
		Conn:          conn,
		ConnectId:     uuid.New(),
		Running:       true,
		lastActive:    time.Now(),
		Encoder:       t.Encoder,
		PacketHandler: t.SocketHandlerPacket,
		MessageRouter: t.Router,
	}
	Clients.Store(client.ConnectId.String(), client)

	defer func() {
		client.Running = false
		Clients.Delete(client.ConnectId)
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
