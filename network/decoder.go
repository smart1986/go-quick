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
