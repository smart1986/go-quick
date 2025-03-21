package config

import (
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"time"
)

var GlobalConfig *Config

type Config struct {
	Server struct {
		Addr      string `yaml:"addr"`
		AdminAddr string `yaml:"adminAddr"`
	} `yml:"server"`
	Log struct {
		Level         string `yaml:"level"`
		File          string `yaml:"file"`
		FileEnable    bool   `yaml:"fileEnable"`
		ConsoleEnable bool   `yaml:"consoleEnable"`
		MaxSize       int    `yaml:"maxSize"`
		MaxAge        int    `yaml:"maxAge"`
		MaxBack       int    `yaml:"maxBack"`
	} `yml:"log"`
	Mongo struct {
		Uri         string `yaml:"uri"`
		Username    string `yaml:"username"`
		Password    string `yaml:"password"`
		Database    string `yaml:"database"`
		MaxPoolSize uint64 `yaml:"maxPoolSize"`
	} `yml:"mongo"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		Db       int    `yaml:"db"`
		PoolSize int    `yaml:"poolSize"`
	} `yml:"redis"`
	Mysql struct {
		Host            string `yaml:"host"`
		Username        string `yaml:"username"`
		Password        string `yaml:"password"`
		Database        string `yaml:"database"`
		Port            int    `yaml:"port"`
		Params          string `yaml:"params"`
		MaxIdleConns    int    `yaml:"maxIdleConns"`
		MaxOpenConns    int    `yaml:"maxOpenConns"`
		ConnMaxLifetime int    `yaml:"connMaxLifetime"`
	} `yml:"mysql"`

	Etcd struct {
		Endpoints   []string `yaml:"endpoints"`
		DialTimeout int      `yaml:"timeout"`
	} `yml:"etcd"`

	LastModifyTime time.Time
}

func InitConfig(fullName string, config *Config) {
	file, err := os.ReadFile(fullName)
	if err != nil {
		log.Fatalf("read config file error: %v", err)
	}
	err = yaml.Unmarshal(file, config)
	if err != nil {
		log.Fatalf("unmarshal config file error: %v", err)
	}
	GlobalConfig = config

	fileInfo, err := os.Stat(fullName)
	if err != nil {
		log.Fatalf("stat config file error: %v", err)
	}
	config.LastModifyTime = fileInfo.ModTime()
}
func InitConfigCustomize(fullName string, config interface{}) {
	file, err := os.ReadFile(fullName)
	if err != nil {
		log.Fatalf("read config file error: %v", err)
	}
	err = yaml.Unmarshal(file, config)
	if err != nil {
		log.Fatalf("unmarshal config file error: %v", err)
	}
}
func InitConfigWithString(content string, config *Config) {
	err := yaml.Unmarshal([]byte(content), config)
	if err != nil {
		log.Fatalf("unmarshal config file error: %v", err)
	}
	GlobalConfig = config
}
func InitConfigCustomizeWithString(content string, config interface{}) {
	err := yaml.Unmarshal([]byte(content), config)
	if err != nil {
		log.Fatalf("unmarshal config file error: %v", err)
	}
}
