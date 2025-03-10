package mytest

import (
	"fmt"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/db"
	"github.com/smart1986/go-quick/logger"
	"testing"
)

type MyConfig struct {
	*config.Config `yaml:",inline"`
	Test           string `yaml:"test"`
}

func TestConfig(t *testing.T) {

	c := &MyConfig{
		Config: &config.Config{},
	}
	config.InitConfigCustomize("./config.yml", c)
	config.GlobalConfig = c.Config
	fmt.Println(c)
}

func TestMysql(t *testing.T) {
	c := &MyConfig{
		Config: &config.Config{},
	}
	config.InitConfigCustomize("./config.yml", c)
	config.GlobalConfig = c.Config
	logger.NewLogger(config.GlobalConfig)
	db.InitMysql(config.GlobalConfig)

}
