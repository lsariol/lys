package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// undefinedTable is Postgres's error code for a missing table
const undefinedTable = "42P01"

// migrationsTable is where goose records applied migrations. It lives in the
// lys schema so everything Lys owns stays in one place.
const migrationsTable = "lys.goose_db_version"

// Migrate applies all pending migrations. connString must log in as the
// migrator role (lys_migrator), which acts as lys_owner.
func Migrate(ctx context.Context, connString string) error {
	provider, db, err := newMigrationProvider(ctx, connString)
	if err != nil {
		return err
	}
	defer db.Close()

	results, err := provider.Up(ctx)
	for _, r := range results {
		log.Printf("migration: %s", r)
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	if len(results) == 0 {
		log.Println("migration: database is up to date")
	}
	return nil
}

// PrintMigrationStatus writes each migration and whether it has been applied.
func PrintMigrationStatus(ctx context.Context, connString string, w io.Writer) error {
	provider, db, err := newMigrationProvider(ctx, connString)
	if err != nil {
		return err
	}
	defer db.Close()

	statuses, err := provider.Status(ctx)
	if err != nil {
		return fmt.Errorf("migration status: %w", err)
	}

	for _, s := range statuses {
		appliedAt := "-"
		if !s.AppliedAt.IsZero() {
			appliedAt = s.AppliedAt.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%-8s %-19s %s\n", s.State, appliedAt, s.Source.Path)
	}
	return nil
}

// CheckSchemaVersion returns an error if the database hasn't had every embedded
// migration applied. It runs with the app role, so Lys refuses to start against
// an old schema instead of failing on its first query.
func (d *Database) CheckSchemaVersion(ctx context.Context) error {
	want, err := latestMigrationVersion()
	if err != nil {
		return err
	}

	const fix = "store LYS_DATABASE_MIGRATOR_URL in Cove so Lys applies its migrations on startup, or run `lys migrate up`"

	var have int64
	const query = `SELECT COALESCE(MAX(version_id), 0) FROM lys.goose_db_version WHERE is_applied`
	if err := d.Pool.QueryRow(ctx, query).Scan(&have); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == undefinedTable {
			return fmt.Errorf("database: no migrations have been applied yet: %s", fix)
		}
		return fmt.Errorf("database: read the schema version: %w", err)
	}

	if have < want {
		return fmt.Errorf("database: the schema is at version %d, but this version of Lys needs %d: %s", have, want, fix)
	}
	return nil
}

func newMigrationProvider(ctx context.Context, connString string) (*goose.Provider, *sql.DB, error) {
	db, err := sql.Open("pgx", connString)
	if err != nil {
		return nil, nil, fmt.Errorf("open migration connection: %w", err)
	}

	// goose creates its version table before running any migration, so the
	// schema that holds it has to exist first.
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS lys"); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("database: prepare migrations with LYS_DATABASE_MIGRATOR_URL: %w", err)
	}

	fsys, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		db.Close()
		return nil, nil, err
	}

	// Prevents two Lys instances from migrating at the same time.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		db.Close()
		return nil, nil, err
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithTableName(migrationsTable),
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("create migration provider: %w", err)
	}

	return provider, db, nil
}

func latestMigrationVersion() (int64, error) {
	names, err := fs.Glob(embeddedMigrations, "migrations/*.sql")
	if err != nil {
		return 0, err
	}

	var latest int64
	for _, name := range names {
		v, err := goose.NumericComponent(name)
		if err != nil {
			return 0, fmt.Errorf("migration %q: %w", name, err)
		}
		latest = max(latest, v)
	}
	return latest, nil
}
