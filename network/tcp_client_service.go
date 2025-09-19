package network

import (
	"sync"

	"github.com/smart1986/go-quick/logger"
)

var (
	clientInstance *TcpClientService
	once           sync.Once
)

type (
	TcpClientService struct {
		connector        *Connector
		responseHandlers map[int32]*ResponseHandler // Map to store response handlers
		pushHandlers     map[int32]*ResponseHandler // Map to store response handlers
	}

	ResponseHandler struct {
		callback func(*DataMessage)
		done     chan *DataMessage
	}
)

func GetClientService() *TcpClientService {
	once.Do(func() {
		clientInstance = &TcpClientService{
			responseHandlers: make(map[int32]*ResponseHandler),
			pushHandlers:     make(map[int32]*ResponseHandler),
		}
	})
	return clientInstance
}

func (cs *TcpClientService) Init(addr string, decoder IDecode, framer IFramer, size int) {
	cs.connector = NewConnector(addr, decoder, framer, cs.rec)
	err := cs.connector.Connect()
	if err != nil {
		panic(err)
	}
}
func (cs *TcpClientService) rec(message *DataMessage) {
	if handler, ok := cs.responseHandlers[message.Header.GetMsgId()]; ok {
		handler.done <- message
		delete(cs.responseHandlers, message.Header.GetMsgId())
		return
	}
	if handler, ok := cs.pushHandlers[message.Header.GetMsgId()]; ok {
		if handler.callback != nil {
			go handler.callback(message)
		} else {
			logger.Warn("No callback for message ID:", message.Header.GetMsgId())
		}
	} else {
		logger.Warn("No handler for message ID:", message.Header.GetMsgId())
	}

}
func (cs *TcpClientService) AddMessageHandler(msgId int32, handler func(*DataMessage)) {
	if handler == nil {
		return
	}
	cs.pushHandlers[msgId] = &ResponseHandler{
		callback: handler,
	}
}

func (cs *TcpClientService) RemoveMessageHandler(msgId int32) {
	delete(cs.pushHandlers, msgId)
}

func (cs *TcpClientService) RequestMessage(msg *DataMessage) (*DataMessage, error) {
	handler := &ResponseHandler{
		done: make(chan *DataMessage, 1),
	}
	cs.responseHandlers[msg.Header.GetMsgId()] = handler
	err := cs.connector.SendMessage(msg)
	if err != nil {
		return nil, err
	}
	return <-handler.done, nil
}
func (cs *TcpClientService) SendMessage(msg *DataMessage) error {
	return cs.connector.SendMessage(msg)
}
