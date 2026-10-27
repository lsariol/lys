package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// width is where help text wraps.
const width = 80

// guide is a multi-step task, shown by `lys help <guide>`.
type guide struct {
	Name    string
	Summary string
	Steps   []string
}

var guides = []guide{
	{
		Name:    "new-channel",
		Summary: "Bring Lys into a new Twitch channel",
		Steps: []string{
			`Add the channel: lys channel add <login>.`,
			`The broadcaster lets the bot speak: either they make the bot a moderator (/mod <bot>), or they authorize Lys's Twitch app with the channel:bot scope. Without one of these, Twitch refuses the bot's replies.`,
			`Check: lys channel list shows it as active, docker logs lys says "joined", and !ping in the channel gets an answer.`,
		},
	},
}

func runHelp(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) == 0 {
		printList(env.Out)
		return nil
	}
	for _, g := range guides {
		if len(in.Args) == 1 && in.Args[0] == g.Name {
			printGuide(env.Out, g)
			return nil
		}
	}
	cmd, rest := lookup(in.Args)
	if cmd == nil || len(rest) > 0 {
		return usageError("There's no command or guide called %q. Run \"lys help\" for the list.", strings.Join(in.Args, " "))
	}
	printCommand(env.Out, cmd)
	return nil
}

func printList(w io.Writer) {
	fmt.Fprintln(w, `Usage: lys <command> [flags]`)
	fmt.Fprintln(w, `Run "lys help <command>" for details. "lys" alone runs the bot.`)

	// One column width for every group, so they line up.
	left := 0
	for _, c := range table {
		left = max(left, len(c.Names[0]))
	}
	for _, g := range guides {
		left = max(left, len(g.Name))
	}

	for _, g := range groups {
		var rows [][2]string
		for _, c := range table {
			if c.Group == g {
				rows = append(rows, [2]string{c.Names[0], c.Summary})
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s:\n", g)
		columnsWidth(w, rows, 2, left)
	}
	fmt.Fprintln(w, "\nGuides:")
	var rows [][2]string
	for _, g := range guides {
		rows = append(rows, [2]string{g.Name, g.Summary})
	}
	columnsWidth(w, rows, 2, left)
}

func printCommand(w io.Writer, c *Command) {
	fmt.Fprintln(w, wrap(c.Summary+".", width))
	if len(c.Names) > 1 {
		fmt.Fprintf(w, "Also: %s\n", strings.Join(c.Names[1:], ", "))
	}

	fmt.Fprintln(w, "\nUsage:")
	for _, u := range c.Usage {
		fmt.Fprintf(w, "  lys %s\n", u)
	}

	if len(c.Flags) > 0 {
		fmt.Fprintln(w, "\nFlags:")
		var rows [][2]string
		for _, f := range c.Flags {
			left := "    --" + f.Name
			if f.Short != "" {
				left = "-" + f.Short + ", --" + f.Name
			}
			if f.Value != "" {
				left += " " + f.Value
			}
			rows = append(rows, [2]string{left, f.Summary})
		}
		columns(w, rows, 2)
	}

	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, e := range c.Examples {
			fmt.Fprintf(w, "  lys %s\n", e.Command)
			fmt.Fprintln(w, indent(wrap(e.Summary, width-6), "      "))
		}
	}
}

func printGuide(w io.Writer, g guide) {
	fmt.Fprintln(w, g.Summary+":")
	for i, s := range g.Steps {
		prefix := fmt.Sprintf("%d. ", i+1)
		fmt.Fprintln(w, prefix+strings.TrimLeft(indent(wrap(s, width-len(prefix)), strings.Repeat(" ", len(prefix))), " "))
	}
}

// columns prints two aligned columns, wrapping the right one at width. A left
// cell too long for the column puts its text on the next line.
func columns(w io.Writer, rows [][2]string, pad int) {
	left := 0
	for _, r := range rows {
		left = max(left, len(r[0]))
	}
	columnsWidth(w, rows, pad, left)
}

// columnsWidth is columns with the left column's width given.
func columnsWidth(w io.Writer, rows [][2]string, pad, left int) {
	left = min(left, 28)
	textWidth := width - pad - left - 2
	gap := strings.Repeat(" ", pad+left+2)

	for _, r := range rows {
		lines := strings.Split(wrap(r[1], textWidth), "\n")
		if len(r[0]) > left {
			fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", pad), r[0])
			for _, l := range lines {
				fmt.Fprintln(w, gap+l)
			}
			continue
		}
		fmt.Fprintf(w, "%s%-*s  %s\n", strings.Repeat(" ", pad), left, r[0], lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintln(w, gap+l)
		}
	}
}

// wrap breaks text into lines of at most n characters, at spaces.
func wrap(text string, n int) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && len(line)+1+len(word) > n {
			lines = append(lines, line)
			line = word
			continue
		}
		if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	return strings.Join(append(lines, line), "\n")
}

func indent(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}
