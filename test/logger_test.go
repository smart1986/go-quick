package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"testing"
	"time"
)

type TimeOffsetHandler struct {
	timeOffset int64
}

func (t *TimeOffsetHandler) GetTimeOffset() int64 {
	return t.timeOffset
}
func (t *TimeOffsetHandler) GetNowSecond() int64 {
	return t.timeOffset
}
func TestLogger(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	timeHandler := &TimeOffsetHandler{timeOffset: int64(time.Now().Unix() + 10000)}
	logger.NewLoggerOfTimeOffset(config.GlobalConfig, timeHandler)
	logger.Info("Starting server")
}
