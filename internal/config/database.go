package config

import (
	"fmt"

	"github.com/lsariol/coveclient"
)

type DatabaseConfig struct {
	ConnectionString string
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{}
}

func (c *Config) InitilizeDatabaseConfig(cove *coveclient.Client, env string) error {

	var dbString string
	var err error

	dbString, err = cove.GetSecret("LYS_DATABASE_URL")
	if err != nil {
		return fmt.Errorf("get secret LYS_DATABASE_URL: %w", err)
	}

	c.Database.ConnectionString = dbString

	return nil
}
