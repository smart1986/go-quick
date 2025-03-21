package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/db"
	"github.com/smart1986/go-quick/logger"
	"testing"
)

func TestMongo(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)

	mongoDB := db.MongoDB{}
	mongoDB.InitDb(config.GlobalConfig)
}
