package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
	"testing"
	"time"
)

func TestServer(tt *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	//third.InitEtcd(config.GlobalConfig)
	//third.InstanceEtcd.RegisterAndWatch("/test/", "192.168.0.106", "", nil)
	//}()
	//value, err := third.InstanceEtcd.Get(context.Background(), "/106", clientv3.WithPrefix())
	//if err != nil {
	//	panic(err)
	//}
	//for _, kv := range value.Kvs {
	//	logger.Debug("Key:", string(kv.Key), " Value:", string(kv.Value))
	//}

	tcpNet := network.TcpServer{
		SocketHandlerPacket: &network.DefaultHandlerPacket{},
		Decoder:             &network.DefaultDecoder{},
		Router:              &network.MessageRouter{},
		IdleTimeout:         1 * time.Minute,
		SessionHandler:      &TestSessionHandler{},
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler(t, false)

	tcpNet.Start(config.GlobalConfig)

	system.WaitElegantExit()
}

type (
	TestServerHandler  struct{}
	TestSessionHandler struct{}
)

func (receiver *TestServerHandler) Execute(connectIdentify interface{}, c *network.ConnectContext, dataMessage *network.DataMessage) *network.DataMessage {
	logger.Info("Received message:", dataMessage)
	c.SendMessage(dataMessage)
	return nil
}
func (receiver *TestServerHandler) MsgId() int32 {
	return 1
}

func (receiver *TestSessionHandler) OnAccept(context *network.ConnectContext) {
	logger.Info("New connection accepted:", context.ConnectId)
}
func (receiver *TestSessionHandler) OnClose(context *network.ConnectContext) {
	logger.Info("Connection closed:", context.ConnectId)
}
func (receiver *TestSessionHandler) OnIdleTimeout(context *network.ConnectContext) {
	logger.Info("Connection idle timeout:", context.ConnectId)
}
