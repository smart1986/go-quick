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
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		FileEnable bool   `yaml:"fileEnable"`
		MaxSize    int    `yaml:"maxSize"`
		MaxAge     int    `yaml:"maxAge"`
		MaxBack    int    `yaml:"maxBack"`
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

func InitConfig(fullName ...string) {
	c := &Config{}
	configName := "config.yml"
	if len(fullName) > 0 {
		configName = fullName[0]
	}
	file, err := os.ReadFile(configName)
	if err != nil {
		log.Fatalf("read config file error: %v", err)
	}
	err = yaml.Unmarshal(file, c)
	if err != nil {
		log.Fatalf("unmarshal config file error: %v", err)
	}
	GlobalConfig = c
}
