package network

import (
	"encoding/binary"
	"github.com/smart1986/go-quick/logger"
	"io"
	"net"
)

type (
	DefaultHandlerPacket struct {
	}
	DefaultDecoder       struct{}
	DefaultPacketHandler struct{}
	IHandlerPacket       interface {
		HandlePacket(conn net.Conn, bufPool *BufPool) ([]byte, bool)
		ToPacket(data []byte, bufPool *BufPool) ([]byte, error)
	}
	IDecode interface {
		Decode(pool *BufPool, array []byte) *DataMessage
	}
)

const MaxPacketSize = 4 * 1024

// HandlePacket 读取并返回数据包，直接返回 bufPool 的内存
// 调用方必须在处理完成后调用 bufPool.Put(body)
func (dm *DefaultHandlerPacket) HandlePacket(conn net.Conn, bufPool *BufPool) ([]byte, bool) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		logger.Debug("ConnectContext disconnected:", conn.RemoteAddr(), err)
		return nil, false
	}

	length := binary.BigEndian.Uint32(header)
	if length < 4 || length > MaxPacketSize {
		logger.Error("Invalid length:", length)
		return nil, false
	}

	bodyLength := int(length - 4)
	body := bufPool.Get(bodyLength)
	if cap(body) < bodyLength {
		bufPool.Put(body) // 原 buffer 不够大，归还
		body = make([]byte, bodyLength)
	} else {
		body = body[:bodyLength]
	}

	if _, err := io.ReadFull(conn, body); err != nil {
		logger.Debug("ConnectContext disconnected:", conn.RemoteAddr(), err)
		bufPool.Put(body)
		return nil, false
	}

	return body, true // ⚠️ 调用方负责 bufPool.Put(body)
}

func (dm *DefaultHandlerPacket) ToPacket(data []byte, bufPool *BufPool) ([]byte, error) {
	total := 4 + len(data)

	var pkt []byte
	if total <= MaxPacketSize {
		pkt = bufPool.Get(total)
	} else {
		pkt = make([]byte, total) // 超出限制，单独分配
	}

	binary.BigEndian.PutUint32(pkt[:4], uint32(total))
	copy(pkt[4:], data)
	return pkt, nil
}

func (dm *DefaultDecoder) Decode(pool *BufPool, array []byte) *DataMessage {
	if len(array) < 6 {
		return nil
	}
	msgId := int32(binary.BigEndian.Uint32(array[0:4]))
	code := int16(binary.BigEndian.Uint16(array[4:6]))

	n := len(array) - 6
	payload := pool.Get(n)   // 小包来自池；大包内部会 fallback 到 make
	copy(payload, array[6:]) // 拷贝，避免与 array 共享内存

	return &DataMessage{
		Header: &DataHeader{MsgId: msgId, Code: code},
		Msg:    payload, // ✅ 上层用完记得 pool.Put(payload)
	}
}
