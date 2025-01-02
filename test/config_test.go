package mytest

import (
	"fmt"
	"github.com/smart1986/go-quick/config"
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
