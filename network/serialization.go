package network

import (
	"bytes"
	"encoding/binary"
	"github.com/smart1986/go-quick/logger"
	"net"
	"sync"
)

type (
	DefaultHandlerPacket struct {
	}
	DefaultEncoder       struct{}
	DefaultDecoder       struct{}
	DefaultPacketHandler struct{}
	IHandlerPacket       interface {
		HandlePacket(conn net.Conn) ([]byte, bool)
		ToPacket(data []byte) ([]byte, error)
	}
	IEncode interface {
		Encode(d *DataMessage) []byte
	}
	IDecode interface {
		Decode(array []byte) *DataMessage
	}
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 4096)
	},
}

func (dm *DefaultHandlerPacket) HandlePacket(conn net.Conn) ([]byte, bool) {
	header := make([]byte, 4)
	totalRead := 0
	for totalRead < 4 {
		n, err := conn.Read(header[totalRead:])
		if err != nil {
			logger.Debug("ConnectContext disconnected:", conn.RemoteAddr(), err)
			return nil, false
		}
		totalRead += n
	}

	var length uint32
	err := binary.Read(bytes.NewReader(header), binary.BigEndian, &length)
	if err != nil || length < 4 {
		logger.Error("Error parsing length or invalid length:", err, length)
		return nil, false
	}

	bodyLength := length - 4
	body := bufPool.Get().([]byte)[:bodyLength]
	totalRead = 0
	for totalRead < int(bodyLength) {
		n, err := conn.Read(body[totalRead:])
		if err != nil {
			logger.Debug("ConnectContext disconnected:", conn.RemoteAddr(), err)
			bufPool.Put(body)
			return nil, false
		}
		totalRead += n
	}
	// 拷贝一份，避免外部修改
	result := make([]byte, bodyLength)
	copy(result, body)
	bufPool.Put(body)
	return result, true
}

func (dm *DefaultHandlerPacket) ToPacket(data []byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	totalLength := int32(4 + len(data))

	if err := binary.Write(buf, binary.BigEndian, totalLength); err != nil {
		logger.Error("Error encoding message")
		return nil, err
	}
	buf.Write(data)
	return buf.Bytes(), nil
}

func (dm *DefaultEncoder) Encode(d *DataMessage) []byte {
	buf := new(bytes.Buffer)
	data := make([]byte, 4+2+len(d.Msg))
	header := d.Header.(*DataHeader)

	binary.BigEndian.PutUint32(data[0:4], uint32(header.MsgId))
	binary.BigEndian.PutUint16(data[4:6], uint16(header.Code))
	copy(data[6:], d.Msg)
	buf.Write(data)
	return buf.Bytes()
}

func (dm *DefaultDecoder) Decode(array []byte) *DataMessage {
	buf := bytes.NewBuffer(array)

	var msgId int32
	if err := binary.Read(buf, binary.BigEndian, &msgId); err != nil {
		return nil
	}

	var code int16
	if err := binary.Read(buf, binary.BigEndian, &code); err != nil {
		return nil
	}

	msgLength := int32(len(array) - 6)
	msg := make([]byte, msgLength)
	if err := binary.Read(buf, binary.BigEndian, msg); err != nil {
		return nil
	}
	header := &DataHeader{
		MsgId: msgId,
		Code:  code,
	}
	return &DataMessage{
		Header: header,
		Msg:    msg,
	}
}
