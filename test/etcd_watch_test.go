package mytest

import (
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"testing"
)

func TestEtcdWatch(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	third.InitEtcd("/test/", "127.0.0.1", "", nil)
	//}()

	system.WaitElegantExit()
}
