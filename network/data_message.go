package network

import "strconv"

type (
	IDataHeader interface {
		GetMsgId() int32
		GetCode() int16
		SetCode(code int16)
		ToString() string
	}
	DataHeader struct {
		MsgId int32
		Code  int16
	}
	DataMessage struct {
		Header  IDataHeader
		Msg     []byte
		Release func()
	}
)

func (d *DataHeader) GetMsgId() int32 {
	return d.MsgId
}
func (d *DataHeader) GetCode() int16 {
	return d.Code
}
func (d *DataHeader) SetCode(code int16) {
	d.Code = code
}
func (d *DataHeader) ToString() string {
	return "DataHeader{MsgId: " + strconv.Itoa(int(d.MsgId)) + ", Code: " + strconv.Itoa(int(d.Code)) + "}"
}

func (m *DataMessage) Close() {
	if m != nil && m.Release != nil {
		m.Release()
		m.Release = nil
	}
}

func NewDataMessage(header IDataHeader, msg []byte) *DataMessage {
	return &DataMessage{
		Header: header,
		Msg:    msg,
	}
}
func NewFailDataMessage(header IDataHeader, code int16) *DataMessage {
	if dataHeader, ok := header.(*DataHeader); ok {
		dataHeader.SetCode(code)
	} else if dataHeader, ok := header.(IDataHeader); ok {
		dataHeader.SetCode(code)
	} else {
		return nil
	}
	return &DataMessage{
		Header: header,
		Msg:    nil,
	}

}
