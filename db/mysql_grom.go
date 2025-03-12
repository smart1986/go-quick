package db

import (
	"database/sql"
	"fmt"
	"github.com/smart1986/go-quick/config"
	"github.com/smart1986/go-quick/logger"
	"github.com/smart1986/go-quick/system"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"time"
)

var MysqlInstance *QMysql

type QMysql struct {
	SqlDb  *sql.DB
	GormDB *gorm.DB
}

func InitMysql(c *config.Config) {
	quickMysql := &QMysql{}
	quickMysql.InitMysql(c)
	MysqlInstance = quickMysql

}

func (q *QMysql) InitMysql(c *config.Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		c.Mysql.Username, c.Mysql.Password, c.Mysql.Host, c.Mysql.Port, c.Mysql.Database, c.Mysql.Params)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	q.GormDB = db
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	maxOpenConnect := 8
	if c.Mysql.MaxOpenConns > 0 {
		maxOpenConnect = c.Mysql.MaxOpenConns
	}
	maxIdleConnect := 2
	if c.Mysql.MaxIdleConns > 0 {
		maxIdleConnect = c.Mysql.MaxIdleConns
	}
	maxLifeTime := 30 * time.Second
	if c.Mysql.ConnMaxLifetime > 0 {
		maxLifeTime = time.Duration(c.Mysql.ConnMaxLifetime) * time.Second
	}
	sqlDB.SetMaxOpenConns(maxOpenConnect)
	sqlDB.SetMaxIdleConns(maxIdleConnect)
	sqlDB.SetConnMaxLifetime(maxLifeTime)
	q.SqlDb = sqlDB
	system.RegisterExitHandler(q)
	logger.Info("Connected to Mysql Successfully, Addr: ", c.Mysql.Host, " Port: ", c.Mysql.Port, " Database: ", c.Mysql.Database)
}

func (q *QMysql) OnSystemExit() {
	q.SqlDb.Close()
	logger.Info("Disconnected from Mysql")
}
