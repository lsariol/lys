package commands

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Registry looks commands up by name, alias or pattern.
type Registry struct {
	byName   map[string]Command // names and aliases, lower case
	commands []Command
	patterns []pattern // in registration order, so the first match wins predictably
}

type pattern struct {
	re  *regexp.Regexp
	cmd Command
}

func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Command)}
}

// Register adds a command; a name or alias that's already taken is an error.
func (r *Registry) Register(c Command) error {
	spec := c.Spec()
	if spec.Name == "" {
		return fmt.Errorf("commands: a command has no name")
	}

	names := append([]string{spec.Name}, spec.Aliases...)
	for _, n := range names {
		if _, taken := r.byName[strings.ToLower(n)]; taken {
			return fmt.Errorf("commands: %q is registered twice", n)
		}
	}

	var patterns []pattern
	for _, p := range spec.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("commands: %q has an invalid pattern %q: %w", spec.Name, p, err)
		}
		patterns = append(patterns, pattern{re: re, cmd: c})
	}

	for _, n := range names {
		r.byName[strings.ToLower(n)] = c
	}
	r.commands = append(r.commands, c)
	r.patterns = append(r.patterns, patterns...)
	return nil
}

// Lookup finds a command by its name or an alias, in any case.
func (r *Registry) Lookup(name string) (Command, bool) {
	c, ok := r.byName[strings.ToLower(name)]
	return c, ok
}

// Match returns the first command whose pattern matches text, with the submatches.
func (r *Registry) Match(text string) (Command, []string, bool) {
	for _, p := range r.patterns {
		if m := p.re.FindStringSubmatch(text); m != nil {
			return p.cmd, m[1:], true
		}
	}
	return nil, nil, false
}

// Commands returns every registered command, sorted by name.
func (r *Registry) Commands() []Command {
	out := append([]Command(nil), r.commands...)
	sort.Slice(out, func(i, j int) bool { return out[i].Spec().Name < out[j].Spec().Name })
	return out
}
