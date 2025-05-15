package network

import (
	"fmt"
	"github.com/smart1986/go-quick/logger"
	"net"
	"sync"
	"time"
)

type Connector struct {
	ServerAddr     string
	Conn           net.Conn
	Running        bool
	reconnecting   bool
	mu             sync.Mutex
	MessageHandler func(*DataMessage)
	Marshal        IHandlerPacket
	Decoder        IDecode
	Encoder        IEncode
}

func NewConnector(serverAddr string, marshal IHandlerPacket, encoder IEncode, decoder IDecode, messageHandler func(message *DataMessage)) *Connector {
	if (serverAddr == "") || (marshal == nil) {
		logger.Error("Error creating connector: serverAddr and marshal must be provided")
		return nil
	}
	if messageHandler == nil {
		logger.Error("Error creating connector: messageHandler must be provided")
		return nil
	}
	connector := &Connector{ServerAddr: serverAddr}
	connector.Running = true
	connector.Marshal = marshal
	connector.MessageHandler = messageHandler
	connector.Encoder = encoder
	connector.Decoder = decoder
	return connector
}

func (c *Connector) Connect() error {
	conn, err := net.Dial("tcp", c.ServerAddr)
	if err != nil {
		return fmt.Errorf("error connecting to server: %w", err)
	}
	c.Conn = conn
	logger.Info("Connected to server:", c.ServerAddr)
	go c.HandleConnection()
	return nil
}

func (c *Connector) Reconnect() error {
	if !c.Running {
		return fmt.Errorf("connector is not running")
	}
	if c.reconnecting {
		return fmt.Errorf("already reconnecting")
	}
	c.reconnecting = true
	defer func() { c.reconnecting = false }()

	maxAttempts := 5
	for attempts := 0; attempts < maxAttempts; attempts++ {
		if !c.Running {
			return fmt.Errorf("connector is not running")
		}
		err := c.Connect()
		if err != nil {
			logger.Error(err)
			time.Sleep(1 * time.Second) // Wait for 1 second before retrying
			continue
		}
		return nil // Successfully reconnected
	}
	c.Running = false
	return fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}

func (c *Connector) HandleConnection() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic:", r)
		}
		logger.Debug("Connection closed")
		c.Close()
	}()

	for c.Running {
		array, done := c.Marshal.HandlePacket(c.Conn)
		if !done {
			logger.Debug("Connection lost, attempting to reconnect...")
			err := c.Reconnect()
			if err != nil {
				logger.Error("Error reconnecting:", err)
				c.Close()
				return
			}
		}

		dataMessage := c.Decoder.Decode(array)
		logger.Debug("Received data message:", dataMessage)
		if c.MessageHandler != nil {
			c.MessageHandler(dataMessage) // 调用外部传入的处理函数
		}
	}
}

func (c *Connector) SendMessage(msg *DataMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	encode := c.Encoder.Encode(msg)
	array, err := c.Marshal.ToPacket(encode)
	if err != nil {
		logger.Error("Error marshalling message:", err)
		return err
	}

	_, err = c.Conn.Write(array)
	if err != nil {
		logger.Error("Error writing to connection:", err)
		// 如果写入失败，尝试重连
		err := c.Reconnect()
		if err != nil {
			logger.Error("Error reconnecting:", err)
			c.Close()
			return err
		}
		// 再次尝试发送消息
		_, err = c.Conn.Write(array)
		if err != nil {
			logger.Error("Error writing to connection after reconnect:", err)
			return err
		}
	}
	return nil
}

func (c *Connector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconnecting = false
	c.Running = false
	if c.Conn != nil {
		c.Conn.Close()
	}
}
