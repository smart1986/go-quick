package network

import (
	"encoding/binary"
	"fmt"
	"net"
)

type (
	IFramer interface {
		WriteFrame(conn net.Conn, m *DataMessage) (int64, error)
	}
	DefaultFramer struct{}
)

func (f *DefaultFramer) WriteFrame(conn net.Conn, m *DataMessage) (int64, error) {
	h, ok := m.Header.(*DataHeader)
	if !ok {
		return 0, fmt.Errorf("invalid header type")
	}
	// 业务头
	var bizHdr [6]byte
	binary.BigEndian.PutUint32(bizHdr[0:4], uint32(h.MsgId))
	binary.BigEndian.PutUint16(bizHdr[4:6], uint16(h.Code))
	total := 4 + 6 + len(m.Msg)
	var lenHdr [4]byte
	binary.BigEndian.PutUint32(lenHdr[:], uint32(total))

	buffs := net.Buffers{lenHdr[:], bizHdr[:], m.Msg}
	return buffs.WriteTo(conn) // writev，零中间大包
}
