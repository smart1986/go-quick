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

	ISessionHandler interface {
		OnAccept(context IConnectContext)
		OnClose(context IConnectContext)
		OnIdleTimeout(context IConnectContext)
	}

	IConnectIdentifyParser interface {
		ParseConnectIdentify(c IConnectContext) (interface{}, error)
	}
)
