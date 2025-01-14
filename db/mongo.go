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
		*Component
		MongoClient      *mongo.Client
		CounterInitValue int64
	}
)

func InitDefaultDB(c *config.Config) {
	MongoInstance = &MongoDB{
		Component: &Component{
			Name:     "MongoDB",
			DataBase: c.Mongo.Database,
		},
	}
	MongoInstance.InitDb(c)
}

func (m *MongoDB) InitDb(c *config.Config) {
	m.Name = "MongoDB"
	m.DataBase = c.Mongo.Database
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
	return m.MongoClient.Database(m.DataBase).Collection(connectionName)
}

func (m *MongoDB) DropCollection(connectionName string) error {
	return m.Get(connectionName).Drop(context.Background())
}

func (m *MongoDB) GetNextSequenceValue(sequenceName string) (int64, error) {
	session, err := m.MongoClient.StartSession()
	if err != nil {
		return 0, err
	}
	defer session.EndSession(context.Background())

	var result struct {
		SequenceValue int64 `bson:"sequence_value"`
	}

	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		collection := m.MongoClient.Database(m.DataBase).Collection("counters")
		filter := bson.M{"_id": sequenceName}
		update := bson.M{"$inc": bson.M{"sequence_value": 1}, "$setOnInsert": bson.M{"sequence_value": m.CounterInitValue}}
		opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)

		err := collection.FindOneAndUpdate(sessCtx, filter, update, opts).Decode(&result)
		if err != nil {
			return nil, err
		}
		return result.SequenceValue, nil
	}

	_, err = session.WithTransaction(context.Background(), callback)
	if err != nil {
		return 0, err
	}
	return result.SequenceValue, nil
}
