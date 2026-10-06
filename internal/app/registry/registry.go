package registry

import (
	"regexp"

	"github.com/lsariol/lys/internal/commands"
)

type Registry struct {
	// cmdName to Registry Command
	masterRegistry map[string]*RegistryCommand

	//Read Only maps
	ReadRegistry ReadRegister
}

func NewRegistry() *Registry {

	rr := ReadRegister{
		PrefixMap: make(map[string]string),
		RegexMap:  make(map[*regexp.Regexp]string),
	}

	return &Registry{
		masterRegistry: make(map[string]*RegistryCommand),
		ReadRegistry:   rr,
	}
}

func (r *Registry) Get(name string) (commands.Command, bool) {

	rCmd, ok := r.masterRegistry[name]
	if !ok {
		return nil, ok
	}

	return rCmd.Cmd, true
}

func (r *Registry) GetReadRegistry() *ReadRegister {
	return &r.ReadRegistry
}
