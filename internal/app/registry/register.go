package registry

import (
	"fmt"
	"regexp"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/commands/cmds/catfacts"
	"github.com/lsariol/lys/internal/commands/cmds/chatgpt"
	"github.com/lsariol/lys/internal/commands/cmds/help"
	"github.com/lsariol/lys/internal/commands/cmds/letterboxd"
	"github.com/lsariol/lys/internal/commands/cmds/oneuppuzzle"
	"github.com/lsariol/lys/internal/commands/cmds/ping"
	"github.com/lsariol/lys/internal/commands/cmds/randomdog"
	"github.com/lsariol/lys/internal/commands/cmds/request"
	"github.com/lsariol/lys/internal/commands/cmds/settings"
	"github.com/lsariol/lys/internal/commands/cmds/sleeper"
	"github.com/lsariol/lys/internal/commands/cmds/uptime"
)

// Register all functions into the registry
func (r *Registry) RegisterAll() {

	r.Register(catfacts.CatFact{})
	r.Register(help.Help{})
	r.Register(ping.Ping{})
	r.Register(randomdog.RandomDog{})
	r.Register(request.Request{})
	r.Register(uptime.UpTime{})
	r.Register(oneuppuzzle.OneUpPuzzle{})
	r.Register(settings.Settings{})
	r.Register(chatgpt.ChatGPT{})
	r.Register(letterboxd.Letterboxd{})
	r.Register(sleeper.Sleeper{})

}

// Register a single command into the registry
func (r *Registry) Register(command commands.Command) {

	//Creating a helper function called add
	add := func(k string, c *RegistryCommand) {

		if _, exists := r.masterRegistry[k]; exists {
			panic(fmt.Sprintf("duplicate command registration %s", k))
		}

		r.ReadRegistry.PrefixMap[k] = k
		r.masterRegistry[k] = c

		for _, p := range c.Aliases {
			r.ReadRegistry.PrefixMap[p] = k
			r.masterRegistry[p] = c
		}

		for _, e := range c.Regexes {
			r.ReadRegistry.RegexMap[e] = k
		}
	}

	rCMD := r.buildCommand(command)
	add(command.Name(), rCMD)
}

func (r *Registry) buildCommand(c commands.Command) *RegistryCommand {

	var expressions []*regexp.Regexp
	for _, v := range c.Regexes() {
		expression := regexp.MustCompile(v)

		expressions = append(expressions, expression)
	}

	cmd := RegistryCommand{
		Name:    c.Name(),
		Aliases: c.Aliases(),
		Regexes: expressions,
		Cmd:     c,
	}

	return &cmd
}
