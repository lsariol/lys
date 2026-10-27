package connections

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lsariol/lys/internal/backoff"
)

// changedChannel is the Postgres notification channel for connection changes;
// the payload is the platform.
const changedChannel = "lys_connections_changed"

// notify tells every listening Lys that a platform's connections changed. It
// is sent when tx commits, and not at all if it rolls back.
func notify(ctx context.Context, tx pgx.Tx, platform string) error {
	_, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, changedChannel, platform)
	return err
}

// Watch calls changed with the platform whenever a connection changes, in this
// process or another (the CLI), and with "" every resync as a safety net and
// after a lost listener reconnects. It returns when ctx ends.
func (s *Store) Watch(ctx context.Context, resync time.Duration, changed func(platform string)) error {
	go func() {
		t := time.NewTicker(resync)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				changed("")
			}
		}
	}()

	for attempt := 1; ; attempt++ {
		err := s.listen(ctx, changed)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wait := backoff.Delay(attempt, time.Second, time.Minute)
		log.Printf("[Connections] listening for channel changes: %v; listening again in %s", err, wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		changed("") // changes may have been missed while it wasn't listening
	}
}

// listen holds one connection open with LISTEN until it fails or ctx ends.
func (s *Store) listen(ctx context.Context, changed func(platform string)) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	// The connection is closed, not returned to the pool, so it doesn't stay listening.
	defer func() {
		_ = conn.Conn().Close(context.Background())
		conn.Release()
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+changedChannel); err != nil {
		return fmt.Errorf("database: listen: %w", err)
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("database: wait for a notification: %w", err)
		}
		changed(n.Payload)
	}
}
