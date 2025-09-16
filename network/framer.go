package network

import (
	"encoding/binary"
	"errors"
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

var ErrProtocol = errors.New("protocol error")

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
	// 栈上 4 字节头，避免每帧分配
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		// 原样把错误上抛：上层可区分 timeout/EOF/closed
		return nil, false, err
	}

	length := binary.BigEndian.Uint32(header[:]) // 总长（含4字节长度本身）
	if length < 4 {
		return nil, false, fmt.Errorf("%w: length<4 (%d)", ErrProtocol, length)
	}

	// 用 uint64 做边界检查，防止 int 溢出；同时限制到 MaxPacketSize
	bodyLen64 := uint64(length - 4)
	// 进程可分配的最大 int
	const maxInt = int(^uint(0) >> 1)
	if bodyLen64 > uint64(maxInt) || bodyLen64 > uint64(MaxPacketSize) {
		return nil, false, fmt.Errorf("%w: body too large (%d)", ErrProtocol, bodyLen64)
	}
	bodyLen := int(bodyLen64)

	// 从池中取；不足则归还并新建
	body := bufPool.Get(bodyLen)
	if cap(body) < bodyLen {
		bufPool.Put(body)
		body = make([]byte, bodyLen)
	} else {
		body = body[:bodyLen]
	}

	// 空载荷直接返回（例如心跳帧）
	if bodyLen == 0 {
		return body, true, nil // 调用方负责 Put(body)
	}

	if _, err := io.ReadFull(conn, body); err != nil {
		// 读取失败要把刚取出的 buffer 归还
		bufPool.Put(body)
		return nil, false, err
	}
	return body, true, nil // ⚠️ 调用方负责 bufPool.Put(body)
}
