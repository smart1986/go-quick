package mytest

import (
	"testing"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
)

func TestClient(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	service := network.GetClientService()
	serverAddr := "127.0.0.1:8888"
	connector := network.NewConnector(serverAddr, &network.DefaultDecoder{}, &network.DefaultFramer{}, test)
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
