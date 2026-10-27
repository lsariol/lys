package uptime

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// UpTime reports how long Lys has been running.
type UpTime struct {
	started time.Time
}

func New(started time.Time) *UpTime { return &UpTime{started: started} }

func (*UpTime) Spec() commands.Spec {
	return commands.Spec{Name: "uptime", Description: "How long the bot has been running.", Usage: "!uptime", Timeout: 3 * time.Second}
}

func (u *UpTime) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {
	return commands.Reply{Text: format(time.Since(u.started))}, nil
}

func format(uptime time.Duration) string {
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	seconds := int(uptime.Seconds()) % 60
	return fmt.Sprintf("%dd %02dh %02dm %02ds", days, hours, minutes, seconds)
}
