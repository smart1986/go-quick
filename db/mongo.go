package db

import (
	"context"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"go.mongodb.org/mongo-driver/bson"
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

func (m *MongoDB) InitDb(c *config.Config) {
	m.component = &Component{}
	m.component.Name = "MongoDB"
	var poolSize uint64 = 8
	if c.Mongo.MaxPoolSize > 0 {
		poolSize = c.Mongo.MaxPoolSize
	}
	clientOptions := options.Client().SetMaxPoolSize(poolSize).
		ApplyURI(c.Mongo.Uri). // 不带用户名和密码的 URI
		SetAuth(options.Credential{
			Username:   c.Mongo.Username, // 设置用户名
			Password:   c.Mongo.Password, // 设置密码
			AuthSource: "admin",          // 指定认证数据库 (通常为 "admin")
		})

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
	MongoInstance = m
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

func GetNextSequence(sequenceName string, initNum int64) (int64, error) {
	session, err := MongoInstance.MongoClient.StartSession()
	if err != nil {
		logger.Errorf("Failed to start session: %v", err)
		return 0, err
	}
	defer session.EndSession(context.Background())

	result, err := session.WithTransaction(context.Background(), func(sessCtx mongo.SessionContext) (interface{}, error) {
		collection := MongoInstance.Get("counters")
		filter := bson.M{"_id": sequenceName}
		update := bson.M{"$inc": bson.M{"seq": 1}}
		opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)

		var result struct {
			Seq int64 `bson:"seq"`
		}

		err := collection.FindOneAndUpdate(sessCtx, filter, update, opts).Decode(&result)
		if err != nil {
			logger.Errorf("Failed to get next sequence for %s: %v", sequenceName, err)
			return nil, err
		}

		// 如果计数器是新创建的，初始化为 initNum
		if result.Seq == 1 {
			_, err = collection.UpdateOne(sessCtx, filter, bson.M{"$set": bson.M{"seq": initNum}})
			if err != nil {
				logger.Errorf("Failed to initialize sequence for %s: %v", sequenceName, err)
				return nil, err
			}
			return initNum, nil
		}

		return result.Seq, nil
	})

	if err != nil {
		return 0, err
	}

	return result.(int64), nil
}
