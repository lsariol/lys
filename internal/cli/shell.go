package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// runShell is `lys shell`: commands one after another, with line editing,
// history (up and down, kept in memory) and Tab completion from the command
// table. exit, quit or Ctrl+D leaves it.
func runShell(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("shell takes no arguments.")
	}
	if !env.Terminal {
		return errors.New("lys shell needs a terminal: open it with docker exec -it lys /lys shell")
	}
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("lys shell: switch the terminal to raw mode: %w", err)
	}
	defer term.Restore(fd, old)

	t := term.NewTerminal(struct {
		io.Reader
		io.Writer
	}{os.Stdin, env.Err}, prompt(env))

	// In raw mode everything goes through the terminal, which turns "\n" into
	// the "\r\n" a raw terminal needs; questions are asked on its prompt.
	shell := *env
	shell.Out, shell.Err = t, t
	shell.readLine = func(question string) (string, error) {
		t.SetPrompt(question)
		defer t.SetPrompt(prompt(env))
		return t.ReadLine()
	}
	t.AutoCompleteCallback = func(line string, pos int, key rune) (string, int, bool) {
		if key != '\t' {
			return "", 0, false
		}
		return complete(&shell, line, pos, t)
	}

	for ctx.Err() == nil {
		line, err := t.ReadLine()
		if err != nil {
			return nil // Ctrl+D or Ctrl+C on the prompt
		}
		args, err := splitLine(line)
		if err != nil {
			shell.Warn("%v", err)
			continue
		}
		switch {
		case len(args) == 0:
			continue
		case args[0] == "exit" || args[0] == "quit":
			return nil
		case args[0] == "shell":
			shell.Warn("This is the shell already.")
			continue
		}
		Run(ctx, &shell, args)
	}
	return nil
}

// prompt is "lys (prod)> ", with prod in red on a coloured terminal.
func prompt(env *Env) string {
	name := strings.ToLower(os.Getenv("APP_ENV"))
	if name == "" {
		name = "dev"
	}
	if env.Color && name == "prod" {
		name = red + name + reset
	}
	return "lys (" + name + ")> "
}

// complete handles Tab: the words typed so far complete to a command name;
// after a command, to what that command's Complete offers. One match is
// completed; several complete their common prefix, and Tab again lists them.
func complete(env *Env, line string, pos int, out io.Writer) (string, int, bool) {
	head, tail := line[:pos], line[pos:]
	typed := strings.TrimLeft(head, " ")
	lead := head[:len(head)-len(typed)]

	// The text being completed, and the candidates that replace it: command
	// names first, since some continue another ("migrate" and "migrate status").
	text := typed
	var candidates []string
	for _, c := range table {
		for _, n := range c.Names {
			if strings.HasPrefix(n, typed) {
				candidates = append(candidates, n)
			}
		}
	}
	words := strings.Fields(typed)
	if cmd, rest := lookup(words); len(candidates) == 0 && cmd != nil && cmd.Complete != nil {
		end := afterWords(typed, len(words)-len(rest))
		text = strings.TrimLeft(typed[end:], " ")
		lead += typed[:len(typed)-len(text)]
		candidates = cmd.Complete(env, strings.Fields(text))
	}

	var matches []string
	for _, c := range candidates {
		if strings.HasPrefix(c, text) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		return "", 0, false
	}
	sort.Strings(matches)
	done := commonPrefix(matches)
	if len(matches) == 1 {
		done += " "
	} else if done == text {
		fmt.Fprintln(out, strings.Join(matches, "  "))
		return "", 0, false
	}
	return lead + done + tail, len(lead) + len(done), true
}

// afterWords is the index in s just after its first n words.
func afterWords(s string, n int) int {
	i := 0
	for range n {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		for i < len(s) && s[i] != ' ' {
			i++
		}
	}
	return i
}

func commonPrefix(words []string) string {
	prefix := words[0]
	for _, w := range words[1:] {
		for !strings.HasPrefix(w, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// splitLine splits a shell line into words; double quotes keep spaces.
func splitLine(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord, quoted := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
			inWord = true
		case r == ' ' && !quoted:
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quoted {
		return nil, errors.New(`a quote isn't closed: end it with "`)
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}
