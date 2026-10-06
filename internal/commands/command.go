package commands

import (
	"context"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Command interface {
	Name() string
	Aliases() []string
	Regexes() []string
	Description() string
	Usage() string
	Timeout() time.Duration
	Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error)
}
