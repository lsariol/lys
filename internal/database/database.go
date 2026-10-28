package database

import (
	"context"
	"fmt"
	"strings"
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

// poolSize is the most connections Lys opens, unless the URL sets
// pool_max_conns. pgxpool's default (the larger of 4 and the CPU count) left 3
// on a 4-CPU server once the channel listener holds one.
const poolSize = 12

func (d *Database) Connect(ctx context.Context) error {

	cfg, err := pgxpool.ParseConfig(d.Config.ConnectionString)
	if err != nil {
		return fmt.Errorf("database: the connection URL doesn't parse (check LYS_DATABASE_URL in Cove): %w", err)
	}
	if !strings.Contains(d.Config.ConnectionString, "pool_max_conns") {
		cfg.MaxConns = poolSize
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database: %w", err)
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
