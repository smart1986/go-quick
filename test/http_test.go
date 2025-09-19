package mytest

import (
	"testing"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/quickhttp"
)

func TestHttp(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)

	server := &quickhttp.HttpServer{}
	server.InitNoAuth(config.GlobalConfig.Server.Addr, true)
}
