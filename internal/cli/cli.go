// Package cli is Lys's command line: one command table that parsing, help
// and completion all come from. `lys <command>` runs one command and exits.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// Command is one entry in the table.
type Command struct {
	Names    []string // the first is the name, the rest aliases; multi-word names are subcommands
	Group    string
	Summary  string
	Usage    []string // forms, without the leading "lys "
	Flags    []Flag
	Examples []Example
	Run      func(ctx context.Context, env *Env, in Input) error
	Complete func(env *Env, args []string) []string // Tab completions for the shell
}

// Flag is a command's option. With an empty Value it's a switch.
type Flag struct {
	Name    string
	Short   string
	Value   string // placeholder, e.g. "<text>"
	Summary string
}

// Example is one example invocation.
type Example struct {
	Command string
	Summary string
}

// Input is a parsed command line.
type Input struct {
	Args  []string
	Flags map[string]string // by long name; a switch is "true"
}

// Has reports whether a switch was given.
func (in Input) Has(name string) bool { return in.Flags[name] == "true" }

// Env is what commands use to talk to the operator, and to reach Lys's services.
type Env struct {
	Out, Err io.Writer
	In       *bufio.Reader
	Terminal bool // stdin and stderr are a terminal
	Color    bool

	// Services loads what commands need on first use, so `help` needs neither Cove nor a database.
	Services func(ctx context.Context) (*Services, error)

	readLine func(prompt string) (string, error) // in the shell, questions are asked on its prompt
}

// NewEnv returns an Env on the process's standard streams.
func NewEnv(services func(ctx context.Context) (*Services, error)) *Env {
	terminal := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
	_, noColor := os.LookupEnv("NO_COLOR")
	return &Env{
		Out:      os.Stdout,
		Err:      os.Stderr,
		In:       bufio.NewReader(os.Stdin),
		Terminal: terminal,
		Color:    terminal && !noColor,
		Services: services,
	}
}

// errUsage marks a wrong command line; Run prints the usage after it.
var errUsage = errors.New("wrong usage")

// usageError is wrong usage with what was wrong.
func usageError(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, a...))
}

// Run runs one command line and returns the exit code.
func Run(ctx context.Context, env *Env, args []string) int {
	cmd, rest := lookup(args)
	if cmd == nil {
		env.Warn("Unknown command %q. Run \"lys help\" for the list.", strings.Join(args, " "))
		return 1
	}

	in, err := parse(cmd, rest)
	if err == nil {
		err = cmd.Run(ctx, env, in)
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		env.Warn("%s", strings.TrimPrefix(err.Error(), errUsage.Error()+": "))
		for _, u := range cmd.Usage {
			fmt.Fprintf(env.Err, "Usage: lys %s\n", u)
		}
		return 1
	case errors.Is(err, errCancelled):
		return 1
	default:
		env.Error("%v", err)
		return 1
	}
}

// lookup finds the command that the first words name, preferring the longest match.
func lookup(args []string) (*Command, []string) {
	for n := min(len(args), 2); n >= 1; n-- {
		name := strings.Join(args[:n], " ")
		for i := range table {
			for _, alias := range table[i].Names {
				if alias == name {
					return &table[i], args[n:]
				}
			}
		}
	}
	return nil, nil
}

// parse splits flags from arguments; flags may come anywhere.
func parse(cmd *Command, args []string) (Input, error) {
	in := Input{Flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			in.Args = append(in.Args, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			in.Args = append(in.Args, a)
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := findFlag(cmd, name, !strings.HasPrefix(a, "--"))
		if f == nil {
			return Input{}, usageError("Unknown flag %q.", a)
		}
		switch {
		case f.Value == "" && hasValue:
			return Input{}, usageError("%q doesn't take a value.", "--"+f.Name)
		case f.Value == "":
			value = "true"
		case !hasValue:
			if i+1 >= len(args) {
				return Input{}, usageError("%q needs a value: %s.", "--"+f.Name, f.Value)
			}
			i++
			value = args[i]
		}
		in.Flags[f.Name] = value
	}
	return in, nil
}

func findFlag(cmd *Command, name string, short bool) *Flag {
	for i, f := range cmd.Flags {
		if (short && f.Short == name) || (!short && f.Name == name) {
			return &cmd.Flags[i]
		}
	}
	return nil
}

// Message symbols and their colours.
const (
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	cyan   = "\x1b[36m"
	reset  = "\x1b[0m"
)

func (env *Env) message(symbol, color, format string, a ...any) {
	if env.Color {
		symbol = color + symbol + reset
	}
	fmt.Fprintf(env.Err, symbol+" "+format+"\n", a...)
}

// Success, Warn and Error write a message to stderr.
func (env *Env) Success(format string, a ...any) { env.message("✓", green, format, a...) }
func (env *Env) Warn(format string, a ...any)    { env.message("!", yellow, format, a...) }
func (env *Env) Error(format string, a ...any)   { env.message("✗", red, format, a...) }

// errCancelled means the operator said no; the message is already printed.
var errCancelled = errors.New("cancelled")

// Confirm asks a yes/no question, defaulting to no. With --yes it doesn't ask;
// without a terminal it refuses rather than guess. verb names the action,
// e.g. "Block".
func (env *Env) Confirm(in Input, verb, question string) error {
	if in.Has("yes") {
		return nil
	}
	if !env.Terminal {
		env.Warn("%s cancelled: no answer to the confirmation. Use --yes to %s without asking.", verb, strings.ToLower(verb))
		return errCancelled
	}
	sym := "?"
	if env.Color {
		sym = cyan + sym + reset
	}
	answer, _ := env.read(fmt.Sprintf("%s %s (y/N) ", sym, question))
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		return nil
	}
	env.Warn("%s cancelled.", verb)
	return errCancelled
}

// Ask reads one line from the operator after showing question.
func (env *Env) Ask(question string) (string, error) {
	if !env.Terminal {
		return "", errors.New("no terminal to ask on")
	}
	sym := "?"
	if env.Color {
		sym = cyan + sym + reset
	}
	line, err := env.read(fmt.Sprintf("%s %s ", sym, question))
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// read shows prompt and reads one line of the operator's answer.
func (env *Env) read(prompt string) (string, error) {
	if env.readLine != nil {
		return env.readLine(prompt)
	}
	fmt.Fprint(env.Err, prompt)
	return env.In.ReadString('\n')
}

// groups lists the help groups in the order help shows them.
var groups = []string{"Channels", "Bot", "Database", "Help"}

// completeCommands offers command names that start with what's typed.
func completeCommands(env *Env, args []string) []string {
	prefix := strings.Join(args, " ")
	var out []string
	for _, c := range table {
		if strings.HasPrefix(c.Names[0], prefix) {
			out = append(out, c.Names[0])
		}
	}
	sort.Strings(out)
	return out
}
