package network

import (
	"reflect"
	"strings"

	"github.com/smart1986/go-quick/logger"
)

var (
	MessageHandler = make(map[int32]IMessageExecutor)
)

type (
	Router interface {
		Route(connectIdentify any, c IConnectContext, dataMessage *DataMessage)
	}

	IMessageExecutor interface {
		Execute(connectIdentify any, c IConnectContext, dataMessage *DataMessage) *DataMessage
		CheckLogin() bool
	}

	MessageExecutor[T any, V any] interface {
		Handle(V, IConnectContext, *DataMessage, T) *DataMessage
		Unmarshal([]byte) (T, error)
	}

	MessageRouter struct {
	}

	wrapped[T any, V any] struct {
		h           MessageExecutor[T, V]
		handlerName string
		checkLogin  bool
	}
)

func wrap[T any, V any](h MessageExecutor[T, V], checkLogin bool) *wrapped[T, V] {
	ht := reflect.TypeOf(h)
	var fullName string
	if ht.Kind() == reflect.Ptr {
		pkg := ht.Elem().PkgPath()
		name := ht.Elem().Name()
		idx := strings.LastIndex(pkg, "/") + 1
		if idx != -1 {
			pkg = pkg[idx:]
		}
		fullName = pkg + "." + name
	} else {
		pkg := ht.PkgPath()
		name := ht.Name()
		idx := strings.LastIndex(pkg, "/") + 1
		if idx != -1 {
			pkg = pkg[idx:]
		}
		fullName = pkg + "." + name
	}
	return &wrapped[T, V]{h: h, handlerName: fullName, checkLogin: checkLogin}
}

func (w *wrapped[T, V]) CheckLogin() bool { return w.checkLogin }

func (w *wrapped[T, V]) Execute(connectIdentify any, c IConnectContext, dataMessage *DataMessage) *DataMessage {
	v, err := w.h.Unmarshal(dataMessage.Msg)
	if err != nil {
		logger.Error("Failed to unmarshal message:", err)
		return nil
	}
	if connectIdentify == nil {
		return w.h.Handle(*new(V), c, dataMessage, v)
	}
	return w.h.Handle(connectIdentify.(V), c, dataMessage, v)
}

func RegisterMessageHandler[T any, V any](msgId int32, handler MessageExecutor[T, V], checkLogin bool) {
	if handler == nil {
		logger.Error("Message handler is nil")
		return
	}
	if _, exists := MessageHandler[msgId]; exists {
		logger.Error("Message handler already exists, msgId:", msgId)
		return
	}
	wrap := wrap[T, V](handler, checkLogin)
	MessageHandler[msgId] = wrap
}
func RegisterMessageHandlerLogin[T any, V any](msgId int32, handler MessageExecutor[T, V]) {
	RegisterMessageHandler[T, V](msgId, handler, true)
}

func (r *MessageRouter) Route(connectIdentify any, c IConnectContext, dataMessage *DataMessage) {
	if handler, existsHandler := MessageHandler[dataMessage.Header.GetMsgId()]; existsHandler {

		if handler.CheckLogin() {
			if connectIdentify == nil {
				logger.Error("Connect identify is nil, msgId:", dataMessage.Header.GetMsgId())
				return
			}
		}
		response := handler.Execute(connectIdentify, c, dataMessage)
		if response != nil {
			c.SendMessage(response)
		}
	} else {
		logger.Error("message handler not found, msgId:", dataMessage.Header.GetMsgId())
	}
}
