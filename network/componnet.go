package network

type (
	IConnectContext interface {
		Execute(msg *DataMessage)
		SendMessage(msg *DataMessage)
		WriteSession(key string, value interface{})
		GetSession(key string) interface{}
		DeleteSession(key string)
		GetConnectId() string
	}
)
