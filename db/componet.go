package db

import "github.com/smart1986/go-quick/config"

type (
	Component struct {
		Name     string
		DataBase string
	}
	IDbInit interface {
		InitDb(c *config.Config)
		InitDbWithIndex(c *config.Config, indexes []IIndex)
	}

	IIndex interface {
		CreateIndividualIndexes()
	}
)
