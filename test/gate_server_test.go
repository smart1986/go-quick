package mytest

import (
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/util"
	"strings"
	"testing"
)

func TestGate(t *testing.T) {
	config.InitConfig("gate.yml")
	logger.NewLogger(config.GlobalConfig)
	addr := config.GlobalConfig.Server.Addr
	s := strings.Split(addr, ":")
	localIp, err := util.GetLocalIP()
	if err != nil {
		panic(err)
	}
	localAddr := localIp + ":" + s[1]
	third.InitEtcd("gate", localAddr, "", OnNodeChange)
	CreateClientPoolFromEtcd("game", 1)

	tcpNet := network.TcpServer{
		UseHeartBeat:        true,
		SocketHandlerPacket: &network.DefaultHandlerPacket{},
		Encoder:             &Gate2ClientEncoder{},
		Decoder:             &network.DefaultDecoder{},
		Router:              &GateRouter{},
	}

	tcpNet.Start(config.GlobalConfig)
}
