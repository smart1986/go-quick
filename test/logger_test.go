package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"testing"
)

type TimeOffsetHandler struct {
	timeOffset int64
}

func (t *TimeOffsetHandler) GetTimeOffset() int64 {
	return t.timeOffset
}
func TestLogger(t *testing.T) {
	config.InitConfig()
	timeHandler := &TimeOffsetHandler{timeOffset: 60 * 60 * 10}
	logger.NewLoggerOfTimeOffset(config.GlobalConfig, timeHandler)
	logger.Info("Starting server")
}
