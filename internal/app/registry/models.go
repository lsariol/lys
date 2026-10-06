package registry

import (
	"regexp"

	"github.com/lsariol/lys/internal/commands"
)

type RegistryCommand struct {
	Name    string
	Aliases []string
	Regexes []*regexp.Regexp
	Cmd     commands.Command
}

type ReadRegister struct {
	PrefixMap map[string]string
	RegexMap  map[*regexp.Regexp]string
}
