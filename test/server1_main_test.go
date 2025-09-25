package mytest

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	"github.com/smart1986/go-quick/system"
)

func Test1Server(tt *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	logger.NewLogger(config.GlobalConfig)
	//go func() {
	//third.InitEtcd(config.GlobalConfig)
	//third.InstanceEtcd.RegisterAndWatch("/test/", "192.168.0.106", "", nil)
	//}()
	//value, err := third.InstanceEtcd.Get(context.Background(), "/106", clientv3.WithPrefix())
	//if err != nil {
	//	panic(err)
	//}
	//for _, kv := range value.Kvs {
	//	logger.Debug("Key:", string(kv.Key), " Value:", string(kv.Value))
	//}

	tcpNet := network.TcpServer{
		IdleTimeout:    1 * time.Minute,
		SessionHandler: &TestSessionHandler{},
		Framer:         &TestFramer{},
		Decoder:        &TestDecoder{},
	}

	t := &TestServerHandler{}
	network.RegisterMessageHandler[string, int](1, t, false)

	tcpNet.Start(config.GlobalConfig.Server.Addr)

	system.WaitElegantExit()
}

type (
	TestFramer struct {
		network.DefaultFramer
	}

	TestHeader struct {
		*network.DataHeader
		RequestId int32
	}

	TestDecoder struct{}
)

//var _ network.MessageExecutor[[]byte, int] = (*TestServerHandler)(nil)

func (f *TestFramer) WriteFrame(conn net.Conn, m *network.DataMessage) (int64, error) {
	h, ok := m.Header.(*TestHeader)
	if !ok {
		return 0, fmt.Errorf("invalid header type")
	}
	// 业务头
	var bizHdr [10]byte
	binary.BigEndian.PutUint32(bizHdr[0:4], uint32(m.Header.GetMsgId()))
	binary.BigEndian.PutUint16(bizHdr[4:6], uint16(m.Header.GetCode()))
	binary.BigEndian.PutUint32(bizHdr[6:10], uint32(h.RequestId))
	total := 4 + 10 + len(m.Msg)
	var lenHdr [4]byte
	binary.BigEndian.PutUint32(lenHdr[:], uint32(total))

	buffs := net.Buffers{lenHdr[:], bizHdr[:], m.Msg}
	return buffs.WriteTo(conn) // writev，零中间大包
}

func (f *TestFramer) CreateFrame(m *network.DataMessage) []byte {
	h, ok := m.Header.(*TestHeader)
	if !ok {
		logger.Error("DefaultFramer: invalid header type")
		return nil
	}
	var bizHdr [10]byte
	binary.BigEndian.PutUint32(bizHdr[0:4], uint32(m.Header.GetMsgId()))
	binary.BigEndian.PutUint16(bizHdr[4:6], uint16(m.Header.GetCode()))
	binary.BigEndian.PutUint32(bizHdr[6:10], uint32(h.Code))
	total := 4 + 10 + len(m.Msg)
	var lenHdr [4]byte
	binary.BigEndian.PutUint32(lenHdr[:], uint32(total))
	frame := make([]byte, 0, total)
	frame = append(frame, lenHdr[:]...)
	frame = append(frame, bizHdr[:]...)
	frame = append(frame, m.Msg...)
	return frame
}

func (d *TestDecoder) Decode(pool *network.BufPool, array []byte) *network.DataMessage {
	// 10B 头：4B msgId + 2B code + 4B requestId
	if len(array) < 10 {
		return nil
	}

	// 解析头
	msgId := int32(binary.BigEndian.Uint32(array[:4]))
	code := int16(binary.BigEndian.Uint16(array[4:6]))
	requestId := int32(binary.BigEndian.Uint32(array[6:10]))

	// 载荷长度
	n := len(array) - 10
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
		copied := copy(payload, array[10:])
		if copied != n {
			// 理论不该发生；出于健壮性考虑可记录一下
			// logger.Warn("payload copy short:", copied, "expect:", n)
			return nil
		}
	} else {
		// 空载荷：避免占用池资源
		payload = nil // 或者 []byte{}
	}

	msg := &network.DataMessage{
		Header: &TestHeader{
			DataHeader: &network.DataHeader{MsgId: msgId, Code: code},
			RequestId:  requestId,
		},
		Msg: payload,
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
