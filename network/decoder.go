package network

import (
	"encoding/binary"
)

type (
	DefaultDecoder struct{}
	IDecode        interface {
		Decode(pool *BufPool, array []byte) *DataMessage
	}
)

const MaxPacketSize = 4 * 1024

func (dm *DefaultDecoder) Decode(pool *BufPool, array []byte) *DataMessage {
	// 6B 头：4B msgId + 2B code
	if len(array) < 6 {
		return nil
	}

	// 解析头
	msgId := int32(binary.BigEndian.Uint32(array[:4]))
	code := int16(binary.BigEndian.Uint16(array[4:6]))

	// 载荷长度
	n := len(array) - 6
	var payload []byte

	if n > 0 {
		// 从池子取并确保 len=n
		payload = pool.Get(n)
		if cap(payload) < n {
			// 如果你的 Get 已保证 cap>=n，这段不会触发；保守兜底
			payload = make([]byte, n)
		} else {
			payload = payload[:n]
		}
		// 拷贝，避免与 array 共享底层内存（array 很快会被回收/复用）
		copied := copy(payload, array[6:])
		if copied != n {
			// 理论不该发生；出于健壮性考虑可记录一下
			// logger.Warn("payload copy short:", copied, "expect:", n)
			return nil
		}
	} else {
		// 空载荷：避免占用池资源
		payload = nil // 或者 []byte{}
	}

	msg := &DataMessage{
		Header: &DataHeader{MsgId: msgId, Code: code},
		Msg:    payload,
	}

	// 统一释放策略：由使用者调用 msg.Close() 归还到池中
	if n > 0 {
		msg.Release = func() {
			pool.Put(payload)
			msg.Msg = nil // 避免重复归还
		}
	}
	return msg
}
