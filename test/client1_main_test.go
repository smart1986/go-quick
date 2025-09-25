package mytest

import (
	"testing"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
)

func Test1Client(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	serverAddr := "127.0.0.1:8888"
	connector := network.NewConnector(serverAddr, &TestDecoder{}, &TestFramer{}, test1)
	err := connector.Connect()
	if err != nil {
		panic(err)
	}
	header := &TestHeader{
		DataHeader: &network.DataHeader{
			MsgId: 1,
			Code:  0,
		},
		RequestId: 123,
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

func test1(message *network.DataMessage) {
	logger.Info("Received message:", message)
	logger.Info("Received message content:", string(message.Msg))

}
