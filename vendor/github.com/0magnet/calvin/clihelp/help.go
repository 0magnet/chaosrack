package clihelp

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The help screen, rendered without text/template.
//
// cobra renders help by executing a template, and a template reaches its data
// through reflection: every {{.UsageString}}, every {{if .HasExample}}, every
// styling function a colorizer adds is a reflect call. TinyGo does not
// implement enough of reflect for that, and does not fail quietly —
//
//	panic: unimplemented: (reflect.Type).NumOut()
//
// — so in the browser build `pisano --help` printed the long description, hit
// the first method call, and took the shell down with it: no error, no prompt,
// a terminal that never came back. It is not a fixable configuration; the
// template engine cannot run there at all.
//
// So the help is written out directly. There is no less information in it than
// the template produced, it is the same on both hosts, and it is the reason the
// browser build can be kept — the alternative was shipping only the standard Go
// build, at four times the download.

// Styles are the colors the help is drawn in: bright blue throughout, bold for
// anything that is a name and plain for anything that describes one.
const (
	styleHeading = "\x1b[94;1m"
	styleName    = "\x1b[94;1m"
	styleDescr   = "\x1b[94m"
	styleExample = "\x1b[94;3m"
	styleReset   = "\x1b[0m"
)

// minNamePadding is cobra's, kept so the listing lines up where it used to.
const minNamePadding = 11

// paint wraps s in a style, or returns it as it is when the output is not going
// somewhere that can show color.
func paint(style, s string) string {
	if !Color || s == "" {
		return s
	}
	return style + s + styleReset
}

// writeHelp is what --help prints: what the command is for, then how to use it.
// The nolint:errcheck on the writes below is deliberate and repeated: this
// renders help to the command's own output, and there is no second channel
// on which to report that writing to it failed.
func writeHelp(w io.Writer, cmd *cobra.Command, usage bool) {
	if long := strings.TrimRight(firstOf(cmd.Long, cmd.Short), " \t\n"); long != "" {
		fmt.Fprintln(w, long) //nolint:errcheck
		// Prose and mechanics are separated the way the sections separate
		// themselves from each other.
		fmt.Fprintln(w) //nolint:errcheck
	}
	writeUsage(w, cmd, usage)
}

// writeUsage prints the mechanical part: the invocation, and every list that is
// not empty. Each section is separated from what came before by one blank line,
// and nothing trails the last one, so a command with no examples and no
// subcommands does not leave gaps where they would have been.
func writeUsage(w io.Writer, cmd *cobra.Command, usage bool) {
	s := &sections{w: w}

	if usage {
		s.open("Usage:")
		// The executable is a name, and the rest of the line is not.
		line := cmd.UseLine()
		if name := cmd.CommandPath(); strings.HasPrefix(line, name) {
			line = paint(styleName, name) + line[len(name):]
		}
		fmt.Fprintf(w, "  %s\n", line) //nolint:errcheck
	}

	if len(cmd.Aliases) > 0 {
		s.open("Aliases:")
		fmt.Fprintf(w, "  %s\n", cmd.NameAndAliases()) //nolint:errcheck
	}

	if cmd.HasExample() {
		s.open("Examples:")
		fmt.Fprintln(w, paint(styleExample, strings.TrimRight(cmd.Example, "\n"))) //nolint:errcheck
	}

	if cmd.HasAvailableSubCommands() {
		s.open("Available Commands:")
		width := namePadding(cmd)
		for _, sub := range cmd.Commands() {
			if sub.Name() == "completion" || !sub.IsAvailableCommand() {
				continue
			}
			// The padding is measured on the name, not on the name plus the
			// escapes that color it, or a colored listing would not line up.
			pad := strings.Repeat(" ", max(width-len(sub.Name()), 0))
			fmt.Fprintf(w, "  %s%s %s\n", //nolint:errcheck
				paint(styleName, sub.Name()), pad, paint(styleDescr, sub.Short))
		}
	}

	if cmd.HasAvailableLocalFlags() {
		s.open("Flags:")
		fmt.Fprintln(w, flagUsages(cmd.LocalFlags())) //nolint:errcheck
	}

	if cmd.HasAvailableInheritedFlags() {
		s.open("Global Flags:")
		fmt.Fprintln(w, flagUsages(cmd.InheritedFlags())) //nolint:errcheck
	}
}

// sections writes a heading, blank-line-separated from whatever preceded it.
type sections struct {
	w    io.Writer
	seen bool
}

func (s *sections) open(heading string) {
	if s.seen {
		fmt.Fprintln(s.w) //nolint:errcheck
	}
	s.seen = true
	fmt.Fprintln(s.w, paint(styleHeading, heading)) //nolint:errcheck
}

// namePadding is how wide the subcommand column is.
func namePadding(cmd *cobra.Command) int {
	width := minNamePadding
	for _, sub := range cmd.Commands() {
		if sub.Name() == "completion" || !sub.IsAvailableCommand() {
			continue
		}
		width = max(width, len(sub.Name())+2)
	}
	return width
}

// flagUsages is pflag's own listing, colored.
//
// pflag lays the block out and wraps it; re-implementing that to insert color
// would be re-implementing it to get it subtly wrong, so the lines it produces
// are painted afterwards. A line it formats differently than expected is passed
// through as it stands rather than mangled.
func flagUsages(set *pflag.FlagSet) string {
	lines := strings.Split(strings.TrimRight(set.FlagUsages(), " \t\n"), "\n")
	for i, line := range lines {
		lines[i] = paintFlagLine(line)
	}
	return strings.Join(lines, "\n")
}

// paintFlagLine styles the flags in one line of pflag's listing and the prose
// after them. A line with no flag on it is a wrapped continuation of the line
// above, so it is all description.
func paintFlagLine(line string) string {
	if !Color {
		return line
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	rest := line[indent:]
	if !strings.HasPrefix(rest, "-") {
		return line[:indent] + paint(styleDescr, rest)
	}
	// The flags and their type run up to the gap pflag leaves before the
	// description; two spaces is a gap, one is the space inside "-c, --cap".
	gap := strings.Index(rest, "   ")
	if gap < 0 {
		return line[:indent] + paint(styleName, rest)
	}
	names, descr := rest[:gap], rest[gap:]
	trimmed := strings.TrimLeft(descr, " ")
	return line[:indent] + paint(styleName, names) +
		descr[:len(descr)-len(trimmed)] + paint(styleDescr, trimmed)
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
