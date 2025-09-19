package mytest

import (
	"testing"
	"time"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
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
		IdleTimeout:    1 * time.Minute,
		SessionHandler: &TestSessionHandler{},
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler[string, int](1, t, false)

	tcpNet.Start(config.GlobalConfig.Server.Addr)

	system.WaitElegantExit()
}

type (
	TestServerHandler  struct{}
	TestSessionHandler struct{}
)

//var _ network.MessageExecutor[[]byte, int] = (*TestServerHandler)(nil)

func (receiver *TestServerHandler) Handle(uid int, context network.IConnectContext, dataMessage *network.DataMessage, data string) *network.DataMessage {
	logger.Info("Received message:", dataMessage)
	context.SendMessage(dataMessage)
	return nil
}

func (receiver *TestServerHandler) Unmarshal(data []byte) (string, error) {
	return string(data), nil
}

func (receiver *TestSessionHandler) OnAccept(context network.IConnectContext) {
	logger.Info("New connection accepted:", context.GetConnectId())
}
func (receiver *TestSessionHandler) OnClose(context network.IConnectContext) {
	logger.Info("Connection closed:", context.GetConnectId())
}
func (receiver *TestSessionHandler) OnIdleTimeout(context network.IConnectContext) {
	logger.Info("Connection idle timeout:", context.GetConnectId())
}
