package uptime

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
)

type UpTime struct{}

func (UpTime) Name() string           { return "uptime" }
func (UpTime) Aliases() []string      { return nil }
func (UpTime) Regexes() []string      { return nil }
func (UpTime) Description() string    { return "Latency check." }
func (UpTime) Usage() string          { return "!ping" }
func (UpTime) Timeout() time.Duration { return 3 * time.Second }

func (UpTime) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	uptime := time.Since(deps.BootTime)
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	seconds := int(uptime.Seconds()) % 60

	formatted := fmt.Sprintf("%dd %02dh %02dm %02ds", days, hours, minutes, seconds)
	return adapter.Response{Text: formatted}, nil

}
