// Package commands defines chat commands and the registry the router looks
// them up in.
package commands

import (
	"context"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/core"
)

// Command is one chat command. Its dependencies are passed to its constructor.
type Command interface {
	Spec() Spec
	Run(ctx context.Context, req Request) (Reply, error)
}

// Spec describes a command to the registry, the router and !help.
type Spec struct {
	Name        string
	Aliases     []string
	Patterns    []string // regular expressions; a match anywhere in a message runs the command
	Description string
	Usage       string
	Timeout     time.Duration // 0 means DefaultTimeout

	Access       Access            // who may use it; Everyone by default
	Sub          map[string]Access // who may use a subcommand (the first argument), when that differs
	Cooldown     time.Duration     // between uses in a channel
	UserCooldown time.Duration     // between one person's uses in a channel
	Concurrency  int               // how many may run at once, across channels; 0 means no limit
}

// SubAccess is who may run the command with these arguments.
func (s Spec) SubAccess(args []string) Access {
	if len(args) > 0 {
		if a, ok := s.Sub[strings.ToLower(args[0])]; ok {
			return a
		}
	}
	return s.Access
}

// DefaultTimeout limits a command whose Spec sets none.
const DefaultTimeout = 10 * time.Second

// Request is a message that matched a command.
type Request struct {
	core.Message
	Identity   core.Identity   // the sender's Lys identity
	Connection core.Connection // the channel's connection and community; zero if unknown
	Command    string          // the command's name, even when called by an alias
	Args       []string        // the words after the command, or a pattern's submatches
	Prefix     string          // the channel's command prefix
	Pattern    bool            // true when a pattern matched instead of the prefix
}

// Reply is a command's answer; an empty Text sends nothing.
type Reply struct {
	Text string
	More []string // further messages, sent after Text in order
}
