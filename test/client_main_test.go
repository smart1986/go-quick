package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"testing"
)

func TestClient(t *testing.T) {
	config.InitConfig()
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
	// 使用信号通道来阻塞主线程
	done := make(chan struct{})
	<-done
}

func test(message *network.DataMessage) {
	logger.Info("Received message:", message)
	logger.Info("Received message content:", string(message.Msg))

}
