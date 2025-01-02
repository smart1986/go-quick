package mytest

import (
	"context"
	"github.com/smart1986/go-quick/config"
	clientv3 "go.etcd.io/etcd/client/v3"
	"testing"
	"time"
)

func TestEtcd(t *testing.T) {
	config.InitConfig("./config.yml", &config.Config{})
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   config.GlobalConfig.Etcd.Endpoints,
		DialTimeout: time.Duration(config.GlobalConfig.Etcd.DialTimeout) * time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer cli.Close()
	leaseResp, err := cli.Grant(context.Background(), 5)
	if err != nil {
		panic(err)
	}

	// Put a key with the lease
	_, err = cli.Put(context.Background(), "/test/", "127.0.0.1", clientv3.WithLease(leaseResp.ID))
	if err != nil {
		panic(err)
	}

	// Keep the lease alive
	ch, kaerr := cli.KeepAlive(context.Background(), leaseResp.ID)
	if kaerr != nil {
		panic(kaerr)
	}

	// Process keep-alive responses
	go func() {
		for {
			<-ch
		}
	}()
	for {

	}

}
