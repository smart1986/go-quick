package schedule

import (
	"time"

	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
)

type (
	Task struct {
		// TaskName 任务名称
		TaskName string
		// Task 任务
		TaskFunc func()
		// Interval 任务执行间隔(秒) 一次性任务为0
		Interval int64
		// StartTime 任务开始时间(秒) 立即执行为0
		StartTime int64
	}
)

var ticker *time.Ticker
var tasks = make(map[string]*Task)

func InitSchedule(timeComponent system.ITime) {
	ticker = time.NewTicker(1 * time.Second)

	go func() {
		defer func() {
			ticker.Stop()
		}()
		for {
			select {
			case <-ticker.C:
				for _, task := range tasks {
					now := timeComponent.GetNowSecond()
					if task.Interval == 0 && task.StartTime == 0 {
						doTask(task, now)
						delete(tasks, task.TaskName)
						logger.Debug("task:", task.TaskName, " remove")
						continue
					}
					if task.StartTime == 0 {
						doTask(task, now)
						continue
					}
					if task.Interval == 0 {
						if now == task.StartTime {
							doTask(task, now)
							delete(tasks, task.TaskName)
							logger.Debug("task:", task.TaskName, " remove")
							continue
						}
					}

					if now-task.StartTime >= task.Interval {
						doTask(task, now)
					}
				}
			}
		}

	}()
}

func doTask(task *Task, now int64) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.ErrorfWithStack("%s task error:", task.TaskName, err)
			}
		}()
		task.StartTime = now
		task.TaskFunc()
	}()
}

func AddJob(task *Task) {
	if task == nil {
		return
	}
	if task.TaskName == "" {
		logger.Error("task name is empty")
		return
	}
	if task.TaskFunc == nil {
		logger.Error("task func is nil")
		return
	}
	tasks[task.TaskName] = task
}
func AddJobs(taskSlice []*Task) {
	if taskSlice == nil {
		return
	}
	for _, task := range taskSlice {
		AddJob(task)
	}
}
