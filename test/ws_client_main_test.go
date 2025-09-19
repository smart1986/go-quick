package mytest

import (
	"testing"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
)

func TestWSClient(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	serverAddr := "ws://127.0.0.1:8888/ws"
	connector := network.NewWSConnector(serverAddr, &network.DefaultDecoder{}, test)
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
