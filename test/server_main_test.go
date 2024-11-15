package mytest

import (
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"testing"
)

func TestServer(tt *testing.T) {
	config.InitConfig()
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	third.InitEtcd("/test/", "192.168.0.106", "", nil)
	//}()
	tcpNet := network.TcpServer{
		UseHeartBeat:        true,
		SocketHandlerPacket: &network.DefaultHandlerPacket{},
		Encoder:             &network.DefaultEncoder{},
		Decoder:             &network.DefaultDecoder{},
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler(t)

	tcpNet.Start(config.GlobalConfig)
}

type (
	TestServerHandler struct{}
)

func (receiver *TestServerHandler) Execute(c *network.Client, dataMessage *network.DataMessage) *network.DataMessage {
	logger.Info("Received message:", dataMessage)
	c.SendMessage(dataMessage)
	return nil
}
func (receiver *TestServerHandler) MsgId() int32 {
	return 1
}
