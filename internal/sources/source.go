// Package sources defines what feeds messages into Lys from outside chat:
// reminders, pollers, and the inlet other services post to.
package sources

import (
	"context"

	"github.com/lsariol/lys/internal/core"
)

// Publisher sends a message out through its platform's adapter.
type Publisher interface {
	Publish(ctx context.Context, out core.Outbound) error
}

// Source gathers messages and publishes them until ctx is cancelled.
type Source interface {
	Name() string
	Run(ctx context.Context, pub Publisher) error
}
