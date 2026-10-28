package help

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// Help lists the commands, or describes one.
type Help struct {
	registry *commands.Registry
}

func New(registry *commands.Registry) *Help { return &Help{registry: registry} }

func (*Help) Spec() commands.Spec {
	return commands.Spec{Name: "help", Description: "Get help using the bot.", Usage: "!help [command]", Timeout: 3 * time.Second}
}

func (h *Help) Run(ctx context.Context, e commands.Request) (commands.Reply, error) {

	if len(e.Args) == 0 {
		var names []string
		for _, c := range h.registry.Commands() {
			names = append(names, e.Prefix+c.Spec().Name)
		}
		return commands.Reply{Text: fmt.Sprintf("Commands: %s. Try %shelp <command> for more.", strings.Join(names, ", "), e.Prefix)}, nil
	}

	name := strings.TrimPrefix(e.Args[0], e.Prefix)
	c, ok := h.registry.Lookup(name)
	if !ok {
		return commands.Reply{Text: fmt.Sprintf("There's no command called '%s'. Try %shelp for the list.", name, e.Prefix)}, nil
	}

	spec := c.Spec()
	text := fmt.Sprintf("%s%s: %s Usage: %s", e.Prefix, spec.Name, spec.Description, spec.Usage)
	if len(spec.Aliases) > 0 {
		text += fmt.Sprintf(" (also %s%s)", e.Prefix, strings.Join(spec.Aliases, ", "+e.Prefix))
	}
	if spec.Access != commands.Everyone {
		text += fmt.Sprintf(" For %s.", spec.Access.Who())
	}
	return commands.Reply{Text: text}, nil
}
