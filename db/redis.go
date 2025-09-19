package db

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
)

var RedisInstance *QRedis

type QRedis struct {
	Client        *redis.Client
	RedisSubjects map[string]func(data string)
	subscribe     *redis.PubSub
}

func InitRedis(c *config.Config) {
	quickRedis := &QRedis{}
	quickRedis.InitRedis(c)
	RedisInstance = quickRedis

}
func (r *QRedis) InitRedis(c *config.Config) {
	ctx := context.Background()
	poolSize := 8
	if c.Redis.PoolSize > 0 {
		poolSize = c.Redis.PoolSize
	}
	client := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.Db,
		PoolSize: poolSize,
	})
	_, err := client.Ping(ctx).Result()
	if err != nil {
		panic(err)
	}
	r.Client = client
	system.RegisterExitHandler(r)
	logger.Infof("Connected to QRedis Successfully, Addr: %s", c.Redis.Addr)
}

func (r *QRedis) RegisterSubject(key string, handler func(data string)) {
	if r.RedisSubjects == nil {
		r.RedisSubjects = make(map[string]func(data string))
	}
	r.RedisSubjects[key] = handler

}
func (r *QRedis) Publish(key string, value interface{}) (int64, error) {
	return r.Client.Publish(context.Background(), key, value).Result()
}
func (r *QRedis) StartSubjectListen() {
	keys := make([]string, 0, len(r.RedisSubjects))
	for s, _ := range r.RedisSubjects {
		keys = append(keys, s)
	}
	subscribe := r.Client.Subscribe(context.Background(), keys...)
	go func() {
		for msg := range subscribe.Channel() {
			if f, ok := r.RedisSubjects[msg.Channel]; ok {
				go f(msg.Payload)
			}
		}
	}()
	r.subscribe = subscribe
}

func (r *QRedis) OnSystemExit() {
	if r.subscribe != nil {
		_ = r.subscribe.Close()
	}
	_ = r.Client.Close()
	logger.Info("Disconnected from QRedis")
}
