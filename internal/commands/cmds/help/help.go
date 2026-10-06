package help

import (
	"context"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type Help struct{}

func (Help) Name() string           { return "help" }
func (Help) Aliases() []string      { return nil }
func (Help) Regexes() []string      { return nil }
func (Help) Description() string    { return "Get help using the bot." }
func (Help) Usage() string          { return "!help <command name>" }
func (Help) Timeout() time.Duration { return 3 * time.Second }

func (Help) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	return adapter.Response{Text: "Ping mobasity, this isnt set up yet"}, nil

}
