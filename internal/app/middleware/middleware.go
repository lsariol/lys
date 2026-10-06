package middleware

import (
	"context"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Handler func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error)
type Middleware func(Handler) Handler
