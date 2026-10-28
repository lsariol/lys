package settings

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

// Settings shows and changes a channel's settings.
type Settings struct {
	config core.ChannelConfig
}

func New(config core.ChannelConfig) *Settings { return &Settings{config: config} }

func (*Settings) Spec() commands.Spec {
	return commands.Spec{
		Name:        "settings",
		Aliases:     []string{"s"},
		Description: "Configure channel settings.",
		Usage:       "!settings prefix [set <new prefix>]",
		Timeout:     3 * time.Second,
	}
}

func (s *Settings) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	if len(e.Args) == 0 {
		return commands.Reply{}, nil
	}

	switch e.Args[0] {

	case "prefix":

		if len(e.Args) == 1 {
			return commands.Reply{Text: fmt.Sprintf("Commands in this channel use the '%s' prefix.", e.Prefix)}, nil
		}

		if len(e.Args) != 3 || e.Args[1] != "set" {
			return commands.Reply{Text: fmt.Sprintf("Invalid usage of the '%ssettings prefix' command. Usage: '%ssettings prefix set <new_prefix>'.", e.Prefix, e.Prefix)}, nil
		}

		if !e.Allowed(commands.Owner) {
			return commands.Refuse(commands.Owner, "change the prefix"), nil
		}

		newPrefix := e.Args[2]

		if len(newPrefix) >= 3 {
			return commands.Reply{Text: "Prefixes cannot be longer than 2 characters long."}, nil
		}

		if err := s.config.SetPrefix(ctx, e.Platform, e.Channel.ID, newPrefix); err != nil {
			return commands.Reply{Text: "There was an error storing this value. Please try again in a few seconds."}, err
		}

		return commands.Reply{Text: fmt.Sprintf("The prefix for this channel is now set to '%s'.", newPrefix)}, nil

	case "pause":
		return commands.Reply{}, nil

	default:
		return commands.Reply{Text: "Invalid arguments for settings."}, nil
	}
}
