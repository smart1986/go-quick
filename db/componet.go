package db

import "github.com/smart1986/go-quick/config"

type (
	Component struct {
		Name     string
		DataBase string
	}
	IDbInit interface {
		InitDb(c *config.Config)
	}
)
