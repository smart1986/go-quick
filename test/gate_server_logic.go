package mytest

import (
	"bytes"
	"encoding/binary"
	"github.com/google/uuid"
	third "github.com/smart1986/go-quick/3rd"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/network"
	clientv3 "go.etcd.io/etcd/client/v3"
	"math/rand"
	"strings"
)

type (
	GateRouter     struct{}
	GateDataHeader struct {
		*network.DataHeader
		ConnectId []byte
	}
	Gate2GameEncoder   struct{}
	Game2GateDecoder   struct{}
	Gate2ClientEncoder struct{}
)

var ConnectsMap = make(map[string][]*network.Connector)

func CreateClientPool(path string, addr string, size int) []*network.Connector {
	var connectors = ConnectsMap[path]
	if connectors == nil {
		connectors = make([]*network.Connector, 0)
		ConnectsMap[path] = connectors
	}
	if size <= 0 {
		size = 1
	}
	for i := 0; i < size; i++ {
		connector := network.NewConnector(addr, &network.DefaultHandlerPacket{}, &Gate2GameEncoder{}, &Game2GateDecoder{}, gameServerHandler)
		err := connector.Connect()
		if err != nil {
			logger.Error(err)
			return nil
		}
		connectors = append(connectors, connector)
	}
	ConnectsMap[path] = connectors
	return connectors
}

func CreateClientPoolFromEtcd(path string, size uint16) {
	for p := range third.NodesInfo {
		split := strings.Split(p, "/")
		if len(split) < 2 {
			continue
		}
		if split[0] != path {
			continue
		}
		CreateClientPool(path, split[1], int(size))
	}

}

func OnNodeChange(evType *clientv3.Event, key string, value []byte) {
	split := strings.Split(key, "/")
	if len(split) < 2 {
		return
	}
	path := split[0]
	addr := split[1]
	if evType.Type == clientv3.EventTypePut {
		CreateClientPoolFromEtcd(path, 1)
		logger.Info("path:", path, " connectorSize:", len(ConnectsMap[path]))
	}
	if evType.Type == clientv3.EventTypeDelete {
		connectors := ConnectsMap[path]
		var updatedConnectors []*network.Connector
		for _, connector := range connectors {
			if connector.ServerAddr == addr {
				logger.Info("close connector:", addr)
				connector.Close()
			} else {
				updatedConnectors = append(updatedConnectors, connector)
			}
		}
		ConnectsMap[path] = updatedConnectors
		logger.Info("path:", path, " connectorSize:", len(updatedConnectors))
	}
}

func gameServerHandler(message *network.DataMessage) {
	logger.Debug("Received message:", message)
	logger.Debug("Received message content:", string(message.Msg))
	header := message.Header.(*GateDataHeader)

	id, err := uuid.FromBytes(header.ConnectId)
	if err != nil {
		logger.Error("Invalid UUID:", err)
		return
	}

	uuid.NewString()
	if client, exists := network.Clients.Load(id.String()); exists {
		client.(*network.ConnectContext).SendMessage(message)
	}
}

func (g *GateRouter) Route(c *network.ConnectContext, dataMessage *network.DataMessage) {
	// Connects中随机一个
	Connects := ConnectsMap["game"]
	if len(Connects) == 0 {
		logger.Error("game server not found")
		return
	}

	randomIndex := rand.Intn(len(Connects))
	connect := Connects[randomIndex]
	oldHeader := dataMessage.Header.(*network.DataHeader)
	dataMessage.Header = &GateDataHeader{
		ConnectId:  c.ConnectId[:],
		DataHeader: oldHeader,
	}
	err := connect.SendMessage(dataMessage)
	if err != nil {

		logger.Error(err)
		return
	}
	logger.Debug("send message to game server", dataMessage)
}

func (g2gEncoder *Gate2GameEncoder) Encode(d *network.DataMessage) []byte {
	buf := new(bytes.Buffer)
	data := make([]byte, 4+2+16+len(d.Msg))
	header := d.Header.(*GateDataHeader)

	binary.BigEndian.PutUint32(data[0:4], uint32(header.DataHeader.MsgId))
	binary.BigEndian.PutUint16(data[4:6], uint16(header.DataHeader.Code))
	copy(data[6:22], header.ConnectId)
	copy(data[22:], d.Msg)
	buf.Write(data)
	return buf.Bytes()
}
func (g2cEncoder *Gate2ClientEncoder) Encode(d *network.DataMessage) []byte {
	buf := new(bytes.Buffer)
	data := make([]byte, 4+2+len(d.Msg))
	header := d.Header.(*GateDataHeader)

	binary.BigEndian.PutUint32(data[0:4], uint32(header.DataHeader.MsgId))
	binary.BigEndian.PutUint16(data[4:6], uint16(header.DataHeader.Code))

	copy(data[6:], d.Msg)
	buf.Write(data)
	return buf.Bytes()
}

func (g2gDecoder *Game2GateDecoder) Decode(array []byte) *network.DataMessage {
	buf := bytes.NewBuffer(array)

	var msgId int32
	if err := binary.Read(buf, binary.BigEndian, &msgId); err != nil {
		return nil
	}

	var code int16
	if err := binary.Read(buf, binary.BigEndian, &code); err != nil {
		return nil
	}

	connectId := make([]byte, 16)
	if _, err := buf.Read(connectId); err != nil {
		return nil
	}

	msgLength := int32(len(array) - 22)
	msg := make([]byte, msgLength)
	if err := binary.Read(buf, binary.BigEndian, msg); err != nil {
		return nil
	}
	header := &GateDataHeader{
		ConnectId: connectId,
		DataHeader: &network.DataHeader{
			MsgId: msgId,
			Code:  code,
		},
	}
	return &network.DataMessage{
		Header: header,
		Msg:    msg,
	}
}
