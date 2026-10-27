package middleware

import (
	"context"
	"testing"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

func TestRecovery(t *testing.T) {
	panics := func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
		var args []string
		_ = args[1]
		return adapter.Response{}, nil
	}

	resp, err := Recovery(panics)(context.Background(), adapter.Envelope{Command: "test"}, nil)
	if err == nil {
		t.Fatal("got no error from a panicking command")
	}
	if !resp.Error || resp.Text == "" {
		t.Fatalf("got %+v, want an error reply with text", resp)
	}
}

func TestRecoveryPassesThrough(t *testing.T) {
	ok := func(ctx context.Context, env adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {
		return adapter.Response{Text: "hi"}, nil
	}

	resp, err := Recovery(ok)(context.Background(), adapter.Envelope{}, nil)
	if err != nil || resp.Text != "hi" {
		t.Fatalf("got %+v, %v; want the command's reply", resp, err)
	}
}
