package middleware

import (
	"context"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func Logging(next Handler) Handler {
	return func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
		resp, err := next(ctx, env, deps)
		if err != nil {
			resp.Error = true
			return resp, err
		}

		return resp, err
	}
}

func Recovery(next Handler) Handler {
	return func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
		resp, err := next(ctx, env, deps)
		if err != nil {
			resp.Error = true
			return resp, err
		}

		return resp, err
	}
}
