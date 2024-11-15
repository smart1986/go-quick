package network

import (
	"github.com/smart1986/go-quick/logger"
	"net"
	"reflect"
	"runtime"
)

var Clients = make(map[int64]*Client)

func init() {
}

type (
	Client struct {
		Conn          *net.Conn
		ConnectId     int64
		Running       bool
		Encoder       IEncode
		PacketHandler IHandlerPacket
		MessageRouter Router
	}
)

func PrintMessageHandler() {
	for k, v := range MessageHandler {
		logger.Info("register msgId:", k, ",handler:", reflect.ValueOf(v).Type())
	}
}

func NewClient(conn *net.Conn, connectId int64, encoder IEncode, paketHandler IHandlerPacket, router Router) *Client {
	var client = &Client{
		Conn:          conn,
		ConnectId:     connectId,
		Encoder:       encoder,
		PacketHandler: paketHandler,
		MessageRouter: router,
	}
	Clients[connectId] = client
	return client
}

func (c *Client) Execute(message *DataMessage) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1024)
			n := runtime.Stack(buf, false)
			logger.Error("Client error recovered:", r)
			logger.Error("Stack trace:", string(buf[:n]))
		}
	}()
	c.MessageRouter.Route(c, message)
}

func (c *Client) SendMessage(msg *DataMessage) {
	var id = c.ConnectId
	encode := c.Encoder.Encode(msg)
	packet, err := c.PacketHandler.ToPacket(encode)
	if err != nil {
		logger.Error("Error marshalling message:", err)
		return
	}
	_, err1 := (*c.Conn).Write(packet)
	logger.Debug("Send data to client:", id, ",len:", len(packet))
	if err1 != nil {
		logger.Error("Error sending data:", err1)
	}

}

func (c *Client) Clear() {
	delete(Clients, c.ConnectId)
}
