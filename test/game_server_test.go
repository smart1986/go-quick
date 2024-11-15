package mytest

import (
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"testing"
)

func TestGameServer(tt *testing.T) {
	config.InitConfig()
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	third.InitEtcd("game", "192.168.0.106:8080", "", nil)
	//}()
	tcpNet := network.TcpServer{
		UseHeartBeat:        false,
		SocketHandlerPacket: &network.DefaultHandlerPacket{},
		Encoder:             &Gate2GameEncoder{},
		Decoder:             &Game2GateDecoder{},
		Router:              &network.MessageRouter{},
	}

	t := &GameTestServerHandler{}
	network.RegisterMessageHandler(t)

	tcpNet.Start(config.GlobalConfig)
}

type (
	GameTestServerHandler struct{}
	//GameMessageRouter     struct{}
)

func (receiver *GameTestServerHandler) Execute(c *network.Client, dataMessage *network.DataMessage) *network.DataMessage {
	logger.Debug("Received message:", dataMessage)
	c.SendMessage(dataMessage)
	return nil
}
func (receiver *GameTestServerHandler) MsgId() int32 {
	return 1
}

//func (r *GameMessageRouter) Route(c *network.Client, dataMessage *network.DataMessage) {
//	header := dataMessage.Header.(*GateDataHeader)
//	if handler, existsHandler := network.MessageHandler[header.MsgId]; existsHandler {
//		response := handler.Execute(c, dataMessage)
//		if response != nil {
//			c.SendMessage(response)
//		}
//	} else {
//		logger.Error("message handler not found, msgId:", header.MsgId)
//		header.Code = 1
//		c.SendMessage(network.NewDataMessage(header, nil))
//	}
//}
