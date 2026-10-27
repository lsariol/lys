package config

import (
	"errors"
	"fmt"

	"github.com/lsariol/coveclient"
)

type DatabaseConfig struct {
	ConnectionString         string // LYS_DATABASE_URL: the app role
	MigratorConnectionString string // LYS_DATABASE_MIGRATOR_URL: the migrator role; empty skips migrations
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{}
}

func (c *Config) InitilizeDatabaseConfig(cove *coveclient.Client, env string) error {

	dbString, err := cove.GetSecret("LYS_DATABASE_URL")
	if err != nil {
		return fmt.Errorf("get secret LYS_DATABASE_URL: %w", err)
	}

	migratorString, err := cove.GetSecret("LYS_DATABASE_MIGRATOR_URL")
	if err != nil && !errors.Is(err, coveclient.ErrNotFound) {
		return fmt.Errorf("get secret LYS_DATABASE_MIGRATOR_URL: %w", err)
	}

	c.Database.ConnectionString = dbString
	c.Database.MigratorConnectionString = migratorString

	return nil
}
