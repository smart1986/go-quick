package network

import (
	"github.com/smart1986/go-quick/logger"
)

var MessageHandler = make(map[int32]*ExecutorWrapper)

type (
	Router interface {
		Route(connectIdentify interface{}, c *ConnectContext, dataMessage *DataMessage)
	}

	MessageExecutor interface {
		Execute(connectIdentify interface{}, c *ConnectContext, dataMessage *DataMessage) *DataMessage
		MsgId() int32
	}
	IConnectIdentifyParser interface {
		ParseConnectIdentify(c *ConnectContext) (interface{}, error)
	}

	MessageRouter struct {
	}

	ExecutorWrapper struct {
		Handler    MessageExecutor
		CheckLogin bool
	}
)

func RegisterMessageHandler(handler MessageExecutor, checkLogin bool) {
	if handler == nil {
		logger.Error("Message handler is nil")
		return
	}
	msgId := handler.MsgId()
	if _, exists := MessageHandler[msgId]; exists {
		logger.Error("Message handler already exists, msgId:", msgId)
		return
	}

	executorWrapper := &ExecutorWrapper{
		Handler:    handler,
		CheckLogin: checkLogin,
	}
	MessageHandler[msgId] = executorWrapper
}

func (r *MessageRouter) Route(connectIdentify interface{}, c *ConnectContext, dataMessage *DataMessage) {
	header := dataMessage.Header.(IDataHeader)
	if handler, existsHandler := MessageHandler[header.GetMsgId()]; existsHandler {
		if handler.CheckLogin {
			if connectIdentify == nil {
				logger.Error("Connect identify is nil, msgId:", header.GetMsgId())
				return
			}
		}
		response := handler.Handler.Execute(connectIdentify, c, dataMessage)
		if response != nil {
			c.SendMessage(response)
		}
	} else {
		logger.Error("message handler not found, msgId:", header.GetMsgId())
		header.SetCode(1)
		c.SendMessage(NewDataMessage(header, nil))
	}
}
