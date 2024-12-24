package db

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
)

var RedisInstance *redis.Client

type Redis struct {
	Client *redis.Client
}

func InitRedis(c *config.Config) {
	ctx := context.Background()
	poolSize := 8
	if c.Redis.PoolSize > 0 {
		poolSize = c.Redis.PoolSize
	}
	RedisInstance = redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.Db,
		PoolSize: poolSize,
	})
	_, err := RedisInstance.Ping(ctx).Result()
	if err != nil {
		panic(err)
	}
	r := &Redis{
		Client: RedisInstance,
	}
	system.RegisterExitHandler(r)
	logger.Infof("Connected to Redis Successfully, Addr: %s", c.Redis.Addr)
}

func (r *Redis) OnSystemExit() {
	_ = r.Client.Close()
	logger.Info("Disconnected from Redis")
}
