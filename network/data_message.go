package network

type (
	IDataHeader interface {
		GetMsgId() int32
		GetCode() int16
		SetCode(code int16)
	}
	DataHeader struct {
		MsgId int32
		Code  int16
	}
	DataMessage struct {
		Header interface{}
		Msg    []byte
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

func NewDataMessage(header interface{}, msg []byte) *DataMessage {
	return &DataMessage{
		Header: header,
		Msg:    msg,
	}
}
