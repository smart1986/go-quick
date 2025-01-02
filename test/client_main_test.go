package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
	"testing"
)

func TestClient(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	serverAddr := "127.0.0.1:20000"
	marshal := &network.DefaultHandlerPacket{}
	connector := network.NewConnector(serverAddr, marshal, &network.DefaultEncoder{}, &network.DefaultDecoder{}, test)
	err := connector.Connect()
	if err != nil {
		panic(err)
	}
	header := &network.DataHeader{
		MsgId: 1,
		Code:  0,
	}
	msg := network.NewDataMessage(header, []byte("Hello"))
	err1 := connector.SendMessage(msg)

	if err1 != nil {
		logger.Info("Error sending message:", err1)
	}
	system.WaitElegantExit(func() {
		connector.Close()
	})
}

func test(message *network.DataMessage) {
	logger.Info("Received message:", message)
	logger.Info("Received message content:", string(message.Msg))

}
