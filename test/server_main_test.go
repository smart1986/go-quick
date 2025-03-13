package mytest

import (
	"context"
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
	clientv3 "go.etcd.io/etcd/client/v3"
	"testing"
	"time"
)

func TestServer(tt *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	third.InitEtcd(config.GlobalConfig)
	third.InstanceEtcd.RegisterAndWatch("/test/", "192.168.0.106", "", nil)
	//}()
	value, err := third.InstanceEtcd.Get(context.Background(), "/106", clientv3.WithPrefix())
	if err != nil {
		panic(err)
	}
	for _, kv := range value.Kvs {
		logger.Debug("Key:", string(kv.Key), " Value:", string(kv.Value))
	}

	tcpNet := network.TcpServer{
		SocketHandlerPacket: &network.DefaultHandlerPacket{},
		Encoder:             &network.DefaultEncoder{},
		Decoder:             &network.DefaultDecoder{},
		Router:              &network.MessageRouter{},
		IdleTimeout:         1 * time.Minute,
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler(t)

	tcpNet.Start(config.GlobalConfig)

	system.WaitElegantExit()
}

type (
	TestServerHandler struct{}
)

func (receiver *TestServerHandler) Execute(c *network.ConnectContext, dataMessage *network.DataMessage) *network.DataMessage {
	logger.Info("Received message:", dataMessage)
	c.SendMessage(dataMessage)
	return nil
}
func (receiver *TestServerHandler) MsgId() int32 {
	return 1
}
