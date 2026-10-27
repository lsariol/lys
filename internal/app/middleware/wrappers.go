package middleware

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

// Logging logs a command's error and marks the response as failed.
func Logging(next Handler) Handler {
	return func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
		resp, err := next(ctx, env, deps)
		if err != nil {
			log.Printf("[Router] command %q in %q: %v", env.Command, env.ChannelName, err)
			resp.Error = true
		}
		return resp, err
	}
}

// Recovery turns a panic in a command into an error reply, so one bad command can't stop the bot.
func Recovery(next Handler) Handler {
	return func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (resp adapter.Response, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Router] command %q in %q panicked: %v\n%s", env.Command, env.ChannelName, r, debug.Stack())
				resp = adapter.Response{Text: "Something went wrong running that command.", Error: true}
				err = fmt.Errorf("command %q panicked: %v", env.Command, r)
			}
		}()
		return next(ctx, env, deps)
	}
}
