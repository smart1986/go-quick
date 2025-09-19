package thrid

import (
	"context"
	"time"

	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type (
	EtcdClient struct {
		*clientv3.Client
		ChangeHandler func(event *clientv3.Event, key string, value []byte)
		NodesInfo     map[string][]byte
	}
)

var InstanceEtcd *EtcdClient

func (e *EtcdClient) OnSystemExit() {
	_ = e.Close()
	logger.Info("Disconnected from Etcd")
}

func InitEtcd(config *config.Config) {
	client := &EtcdClient{}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   config.Etcd.Endpoints,
		DialTimeout: time.Duration(config.Etcd.DialTimeout) * time.Second,
	})
	if err != nil {
		panic(err)
	}
	client.Client = cli
	system.RegisterExitHandler(client)
	InstanceEtcd = client
}

func (e *EtcdClient) RegisterAndWatch(node string, selfAddr string, value string, changeHandler func(event *clientv3.Event, key string, value []byte)) {
	e.ChangeHandler = changeHandler
	leaseResp, err := InstanceEtcd.Grant(context.Background(), 2)
	if err != nil {
		panic(err)
	}
	resp, err := InstanceEtcd.Get(context.Background(), node, clientv3.WithPrefix())
	if err != nil {
		panic(err)
	}
	e.NodesInfo = make(map[string][]byte)
	for _, kv := range resp.Kvs {
		e.NodesInfo[string(kv.Key)] = kv.Value
		logger.Debug("Key:", string(kv.Key), " Value:", string(kv.Value))
	}
	// Put a key with the lease
	key := node + "/" + selfAddr
	_, err = InstanceEtcd.Put(context.Background(), key, value, clientv3.WithLease(leaseResp.ID))
	if err != nil {
		panic(err)
	}

	// Keep the lease alive
	ch, keepErr := InstanceEtcd.KeepAlive(context.Background(), leaseResp.ID)
	if keepErr != nil {
		panic(keepErr)
	}
	go func() {
		for {
			<-ch
		}
	}()
	go func() {
		rch := InstanceEtcd.Watch(context.Background(), node, clientv3.WithPrefix()) // <-chan WatchResponse
		for watchResp := range rch {
			if watchResp.Err() != nil {
				logger.Debugf("Watch error: %v", watchResp.Err())
				continue
			}
			for _, ev := range watchResp.Events {
				logger.Debugf("Type: %s Key:%s Value:%s", ev.Type, ev.Kv.Key, ev.Kv.Value)
				if ev.Type == clientv3.EventTypePut {
					e.NodesInfo[string(ev.Kv.Key)] = ev.Kv.Value
				} else if ev.Type == clientv3.EventTypeDelete {
					delete(e.NodesInfo, string(ev.Kv.Key))
				} else {
					logger.Warnf("Unknown event type %s", ev.Type)
				}
				if e.ChangeHandler != nil {
					e.ChangeHandler(ev, string(ev.Kv.Key), ev.Kv.Value)
				}
			}
		}

	}()

}
