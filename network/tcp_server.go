package network

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"net"
	"sync/atomic"
	"time"
)

var ClientId int64

type TcpServer struct {
	Running             bool
	UseHeartBeat        bool
	SocketHandlerPacket IHandlerPacket
	Encoder             IEncode
	Decoder             IDecode
	Router              Router
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
	PrintMessageHandler()
	listener, err := net.Listen("tcp", c.Server.Addr)
	if err != nil {
		logger.Error("Error starting TCP server:", err)
		return
	}
	defer listener.Close()
	logger.Info("Server started on :", c.Server.Addr)
	t.Running = true

	// Initialize ClientId
	atomic.StoreInt64(&ClientId, 0)

	for t.Running {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("Error accepting connection:", err)
			continue
		}
		go handleConnection(conn, t)
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
	clientId := atomic.AddInt64(&ClientId, 1)
	client := NewClient(&conn, clientId, t.Encoder, t.SocketHandlerPacket, t.Router)
	client.Running = true

	defer client.Clear()

	var timeout *time.Timer
	if t.UseHeartBeat {
		timeout = time.NewTimer(1 * time.Minute)
		defer timeout.Stop()

		go func() {
			<-timeout.C
			logger.Debug("Client timed out:", conn.RemoteAddr())
			client.Running = false
		}()
	}

	for client.Running {
		array, done := t.SocketHandlerPacket.HandlePacket(conn)
		if !done {
			client.Running = false
			return
		}

		if timeout != nil && !timeout.Stop() {
			select {
			case <-timeout.C:
			default:
			}
		}
		if timeout != nil {
			timeout.Reset(1 * time.Minute)
		}

		dataMessage := t.Decoder.Decode(array)
		logger.Debug("Received data message:", dataMessage)
		client.Execute(dataMessage)
	}
}
