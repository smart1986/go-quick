package network

import (
	"encoding/binary"
	"fmt"
	"github.com/smart1986/go-quick/logger"
	"io"
	"net"
)

type (
	IFramer interface {
		WriteFrame(conn net.Conn, m *DataMessage) (int64, error)
		CreateFrame(m *DataMessage) []byte
		DeFrame(conn net.Conn, bufPool *BufPool) ([]byte, bool, error)
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

func (f *DefaultFramer) CreateFrame(m *DataMessage) []byte {
	h, ok := m.Header.(*DataHeader)
	if !ok {
		logger.Error("DefaultFramer: invalid header type")
		return nil
	}
	var bizHdr [6]byte
	binary.BigEndian.PutUint32(bizHdr[0:4], uint32(h.MsgId))
	binary.BigEndian.PutUint16(bizHdr[4:6], uint16(h.Code))
	total := 4 + 6 + len(m.Msg)
	var lenHdr [4]byte
	binary.BigEndian.PutUint32(lenHdr[:], uint32(total))
	frame := make([]byte, 0, total)
	frame = append(frame, lenHdr[:]...)
	frame = append(frame, bizHdr[:]...)
	frame = append(frame, m.Msg...)
	return frame
}

func (f *DefaultFramer) DeFrame(conn net.Conn, bufPool *BufPool) ([]byte, bool, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		logger.Debug("ConnectContext disconnected:", conn.RemoteAddr(), err)
		return nil, false, err
	}

	length := binary.BigEndian.Uint32(header)
	if length < 4 || length > MaxPacketSize {
		logger.Error("Invalid length:", length)
		return nil, false, io.ErrUnexpectedEOF
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
		return nil, false, err
	}
	return body, true, nil // ⚠️ 调用方负责 bufPool.Put(body)
}
