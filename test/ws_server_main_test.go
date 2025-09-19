package mytest

import (
	"testing"
	"time"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
)

func TestWSServer(tt *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)

	wsServer := &network.WSServer{
		Decoder:        &network.DefaultDecoder{},
		Router:         &network.MessageRouter{},
		IdleTimeout:    1 * time.Minute,
		SessionHandler: &TestSessionHandler{},
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler[string, int](1, t, false)

	wsServer.Start(config.GlobalConfig.Server.Addr)

	system.WaitElegantExit()
}
