package db

import (
	"context"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"time"
)

var MongoInstance *MongoDB

type (
	MongoDB struct {
		component   *Component
		MongoClient *mongo.Client
	}
)

func InitDefaultDB(c *config.Config) {
	MongoInstance = &MongoDB{}
	MongoInstance.InitDb(c)
}

func (m *MongoDB) InitDb(c *config.Config) {
	m.component = &Component{}
	m.component.Name = "MongoDB"
	var poolSize uint64 = 8
	if c.Mongo.MaxPoolSize > 0 {
		poolSize = c.Mongo.MaxPoolSize
	}
	if c.Mongo.Uri == "" {
		panic("MongoDB URI is required")
	}
	var clientOptions *options.ClientOptions
	if c.Mongo.Username != "" && c.Mongo.Password != "" {
		clientOptions = options.Client().SetMaxPoolSize(poolSize).
			ApplyURI(c.Mongo.Uri). // 不带用户名和密码的 URI
			SetAuth(options.Credential{
				Username:   c.Mongo.Username, // 设置用户名
				Password:   c.Mongo.Password, // 设置密码
				AuthSource: "admin",          // 指定认证数据库 (通常为 "admin")
			})
	} else {
		clientOptions = options.Client().SetMaxPoolSize(poolSize).ApplyURI(c.Mongo.Uri)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	m.MongoClient = client
	system.RegisterExitHandler(m)
	logger.Infof("Connected to MongoDB Successfully, URI: %s", c.Mongo.Uri)
}

func (m *MongoDB) OnSystemExit() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := m.MongoClient.Disconnect(ctx)
	if err != nil {
		logger.Errorf("Failed to disconnect from MongoDB: %v", err)
	}
	logger.Info("Disconnected from MongoDB")
}

func (m *MongoDB) Get(connectionName string) *mongo.Collection {
	return m.MongoClient.Database(config.GlobalConfig.Mongo.Database).Collection(connectionName)
}

func (m *MongoDB) DropCollection(connectionName string) error {
	return m.Get(connectionName).Drop(context.Background())
}
