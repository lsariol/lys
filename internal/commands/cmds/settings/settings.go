package settings

import (
	"context"
	"fmt"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/commands"
)

type Settings struct{}

func (Settings) Name() string           { return "settings" }
func (Settings) Aliases() []string      { return []string{"s"} }
func (Settings) Regexes() []string      { return nil }
func (Settings) Description() string    { return "Configure channel settings." }
func (Settings) Usage() string          { return "!settings" }
func (Settings) Timeout() time.Duration { return 3 * time.Second }

func (Settings) Execute(ctx context.Context, e adapter.Envelope, deps *dependencies.Deps) (adapter.Response, error) {

	if len(e.Args) == 0 {
		return commands.SuppressedReply()
	}

	switch e.Args[0] {

	case "prefix":

		if len(e.Args) == 1 {

			return adapter.Response{Text: fmt.Sprintf("Commands in this channel use the '%s' prefix.", e.Prefix)}, nil
		}

		if len(e.Args) != 3 || e.Args[1] != "set" {

			return adapter.Response{Text: fmt.Sprintf("Invalid usage of the '%ssettings prefix' command. Usage: '%ssettings prefix set <new_prefix>'.", e.Prefix, e.Prefix)}, nil
		}

		if !commands.HasPrivilege(e.UserID, e.ChannelID) {

			return adapter.Response{Text: "You do not have privileges to use this command."}, nil
		}

		newPrefix := e.Args[2]

		if newPrefix == "" {

			return adapter.Response{Text: "Prefixes cannot be empty."}, nil
		}

		if len(e.Args[2]) >= 3 {

			return adapter.Response{Text: "Prefixes cannot be longer than 2 characters long."}, nil
		}

		err := deps.Settings.UpdateTwitchChannelPrefixSetting(ctx, e.ChannelID, newPrefix)
		if err != nil {

			return adapter.Response{Text: "There was an error storing this value. Please try again in a few seconds.", Error: true}, err
		}

		return adapter.Response{Text: fmt.Sprintf("The prefix for this channel is now set to '%s'.", e.Args[2])}, nil

	case "pause":

	default:
		return adapter.Response{Text: "Invalid arguments for settings."}, nil
	}

	return adapter.Response{}, nil
}
