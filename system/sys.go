package system

import (
	"os"
	"os/signal"
	"reflect"
	"syscall"
)

type (
	IExit interface {
		OnSystemExit()
	}
)

var exitHandlers []IExit

func RegisterExitHandler(handler IExit) {
	exitHandlers = append(exitHandlers, handler)
}
func WaitElegantExit(exitFunc ...func()) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for i := range c {
		switch i {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			// 这里做一些清理操作或者输出相关说明，比如 断开数据库连接
			for _, call := range exitFunc {
				call()
			}
			for _, handler := range exitHandlers {
				if reflect.TypeOf(handler).Implements(reflect.TypeOf((*IExit)(nil)).Elem()) {
					handler.OnSystemExit()
				}
			}
			os.Exit(0)
		}
	}
}
