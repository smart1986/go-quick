package mytest

import (
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/schedule"
	"sync"
	"testing"
	"time"
)

type (
	MyTimeOffsetHandler struct{}
)

func (mt *MyTimeOffsetHandler) GetNowSecond() int64 {
	return time.Now().Unix()
}
func (mt *MyTimeOffsetHandler) GetNowMillisecond() int64 {
	return time.Now().UnixMilli()
}

func TestSchedule(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	timeHandler := &MyTimeOffsetHandler{}
	schedule.InitSchedule(timeHandler)

	task1 := &schedule.Task{
		Interval: 0,
		TaskFunc: func() {
			logger.Info("Task1")
		},
		StartTime: timeHandler.GetNowSecond() + 5,
		TaskName:  "Task1",
	}
	schedule.AddJob(task1)
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}
