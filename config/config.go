package config

import (
	"gopkg.in/yaml.v3"
	"log"
	"os"
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

	Etcd struct {
		Endpoints   []string `yaml:"endpoints"`
		DialTimeout int      `yaml:"timeout"`
	} `yml:"etcd"`
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
