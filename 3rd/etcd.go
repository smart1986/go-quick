package thrid

import (
	"context"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"sync"
	"time"
)

var InstanceEtcd *clientv3.Client
var NodesInfo map[string][]byte
var changeHandler func(event *clientv3.Event, key string, value []byte)

func InitEtcd(node string, selfAddr string, value string, changeFunc func(event *clientv3.Event, key string, value []byte)) {
	changeHandler = changeFunc
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go doInit(node, selfAddr, wg, value)
	wg.Wait()
}
func doInit(node string, selfAddr string, wg *sync.WaitGroup, value string) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   config.GlobalConfig.Etcd.Endpoints,
		DialTimeout: time.Duration(config.GlobalConfig.Etcd.DialTimeout) * time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer func(cli *clientv3.Client) {
		_ = cli.Close()
	}(cli)

	InstanceEtcd = cli

	registerAndWatch(node, selfAddr, wg, value)
}

func registerAndWatch(node string, selfAddr string, wg *sync.WaitGroup, value string) {

	leaseResp, err := InstanceEtcd.Grant(context.Background(), 2)
	if err != nil {
		panic(err)
	}
	resp, err := InstanceEtcd.Get(context.Background(), "", clientv3.WithPrefix())
	if err != nil {
		panic(err)
	}
	NodesInfo = make(map[string][]byte)
	for _, kv := range resp.Kvs {
		NodesInfo[string(kv.Key)] = kv.Value
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
	//go func() {
	rch := InstanceEtcd.Watch(context.Background(), "", clientv3.WithPrefix()) // <-chan WatchResponse
	wg.Done()
	for watchResp := range rch {
		if watchResp.Err() != nil {
			logger.Debugf("Watch error: %v", watchResp.Err())
			continue
		}
		for _, ev := range watchResp.Events {
			logger.Debugf("Type: %s Key:%s Value:%s", ev.Type, ev.Kv.Key, ev.Kv.Value)
			if ev.Type == clientv3.EventTypePut {
				NodesInfo[string(ev.Kv.Key)] = ev.Kv.Value
			} else if ev.Type == clientv3.EventTypeDelete {
				delete(NodesInfo, string(ev.Kv.Key))
			} else {
				logger.Warnf("Unknown event type %s", ev.Type)
			}
			if changeHandler != nil {
				changeHandler(ev, string(ev.Kv.Key), ev.Kv.Value)
			}
		}
	}

	//}()

}
