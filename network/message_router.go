package network

import (
	"github.com/smart1986/go-quick/logger"
)

var MessageHandler = make(map[int32]MessageExecutor)

type (
	Router interface {
		Route(c *ConnectContext, dataMessage interface{})
	}

	MessageExecutor interface {
		Execute(c *ConnectContext, dataMessage interface{}) *DataMessage
		MsgId() int32
	}

	MessageRouter struct {
	}
)

func RegisterMessageHandler(handler MessageExecutor) {
	msgId := handler.MsgId()
	if _, exists := MessageHandler[msgId]; exists {
		logger.Error("Message handler already exists, msgId:", msgId)
		return
	}
	MessageHandler[msgId] = handler
}

func (r *MessageRouter) Route(c *ConnectContext, dataMessage *DataMessage) {
	header := dataMessage.Header.(IDataHeader)
	if handler, existsHandler := MessageHandler[header.GetMsgId()]; existsHandler {
		response := handler.Execute(c, dataMessage)
		if response != nil {
			c.SendMessage(response)
		}
	} else {
		logger.Error("message handler not found, msgId:", header.GetMsgId())
		header.SetCode(1)
		c.SendMessage(NewDataMessage(header, nil))
	}
}
