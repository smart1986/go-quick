package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/quickhttp"
	"testing"
)

func TestHttp(t *testing.T) {
	config.InitConfig()
	logger.NewLogger(config.GlobalConfig)

	server := &quickhttp.HttpServer{}
	server.InitNoAuth(config.GlobalConfig.Server.Addr, true)
}
