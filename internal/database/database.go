package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lsariol/lys/internal/config"
)

type Database struct {
	Pool   *pgxpool.Pool
	Config *config.DatabaseConfig
}

func NewDatabase(c *config.DatabaseConfig) *Database {

	return &Database{
		Config: c,
	}
}

func (d *Database) Connect(ctx context.Context) error {

	pool, err := pgxpool.New(ctx, d.Config.ConnectionString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return fmt.Errorf("ping database: %w", err)
	}

	d.Pool = pool
	return nil
}

func (d *Database) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
}
