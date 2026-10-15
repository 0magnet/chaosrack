package subcmds

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/manual"
)

// `chaosrack manual` — the rack's manual in a terminal: a chapter, one
// entry, a bay's page as the Info window shows it, or the entries that say
// something. The same manual the page's tooltips and Info window read.

func newManualCmd(d Dialer) *cobra.Command {
	var bay, width int
	var model, search string
	cmd := &cobra.Command{
		Use:   "manual [chapter | control | key]",
		Short: "read the rack's manual",
		Long: `Read the rack's manual: the text the page's tooltips and Info window show.

With no argument, list the chapters. With a chapter's name, print it. With a
control's id or an entry's key, print what the manual says of it, its
positions under it for a selector.

  chaosrack manual
  chaosrack manual display
  chaosrack manual torus-R
  chaosrack manual --bay 3
  chaosrack manual --search lyapunov | less`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if width <= 0 {
				width = termWidth()
			}
			switch {
			case search != "":
				return manualSearch(search)
			case bay > 0:
				pg, err := bayPage(d, bay, model)
				if err != nil {
					return err
				}
				return printLines(manual.BayLines(pg.Title, pg.Mods), width)
			case len(args) == 0:
				return manualChapters()
			}
			return manualTopic(d, args[0], width)
		},
	}
	cmd.Flags().IntVar(&bay, "bay", 0, "print bay N's page, as the Info window shows it")
	cmd.Flags().StringVar(&model, "model", "", "with --bay: the model whose bank is on bay 1 (default: the rack's opening one)")
	cmd.Flags().StringVar(&search, "search", "", "list the entries that mention these words")
	cmd.Flags().IntVar(&width, "width", 0, "wrap to this many columns (default: the terminal's, or 80)")
	return cmd
}

// termWidth is the terminal's width as the shell says it, or 80.
func termWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 20 {
		return n
	}
	return 80
}

// manualChapters lists the chapters, by the name to ask for and title.
func manualChapters() error {
	for _, f := range manual.Files() {
		if _, err := fmt.Fprintf(stdout, "%-12s %s\n", f, manual.ChapterTitle(f)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(stdout, "\nchaosrack manual CHAPTER prints one; a control's id prints its entry.")
	return err
}

// manualTopic prints a chapter; the entry under a key; or a control's, as
// its bay's page gives it or, for a model's parameter, under p.<id>.
func manualTopic(d Dialer, arg string, width int) error {
	if text, ok := manual.Chapter(arg); ok {
		_, err := fmt.Fprint(stdout, text)
		return err
	}
	if manual.Has(arg) {
		return printLines(entryLines(arg), width)
	}
	if lines, ok := controlLines(d, arg); ok {
		return printLines(lines, width)
	}
	if manual.Has("p." + arg) {
		return printLines(entryLines("p."+arg), width)
	}
	return fmt.Errorf("the manual has no chapter or entry %q (chaosrack manual lists the chapters; --search finds entries)", arg)
}

// controlLines is the entry of the control id as a bay's page gives it,
// found among the bays' cells by the keys each carries (a control's id is
// one), with its address; false when no cell has it.
func controlLines(d Dialer, id string) ([]manual.Line, bool) {
	for n := 1; ; n++ {
		pg, err := bayPage(d, n, "")
		if err != nil || len(pg.Mods) == 0 {
			return nil, false
		}
		for _, m := range pg.Mods {
			for _, c := range m.Cells {
				if slices.Contains(c.Keys, id) {
					title := m.Name + " " + c.Loc + "   (" + id + ")"
					return manual.TextLines(title, manual.EntryText(c.Tip, c.Keys)), true
				}
			}
		}
	}
}

// entryLines is key's entry as lines: what it says, its positions under it.
func entryLines(key string) []manual.Line {
	title := key
	if e, ok := manual.Lookup(key); ok && e.Heading != "" {
		title = e.Heading + "   (" + key + ")"
	}
	return manual.TextLines(title, manual.EntryText(manual.Text(key), []string{key}))
}

// manualSearch lists the entries whose text has every word of words in it.
func manualSearch(words string) error {
	ws := strings.Fields(strings.ToLower(words))
	n := 0
	for _, k := range manual.Keys() {
		t := strings.ToLower(manual.Text(k))
		hit := len(ws) > 0
		for _, w := range ws {
			hit = hit && strings.Contains(t, w)
		}
		if !hit {
			continue
		}
		n++
		if _, err := fmt.Fprintf(stdout, "%-28s %s\n", k, manual.FirstSentence(manual.Text(k))); err != nil {
			return err
		}
	}
	if n == 0 {
		return fmt.Errorf("no entry mentions %q", words)
	}
	return nil
}

// printLines prints lines wrapped to width.
func printLines(lines []manual.Line, width int) error {
	for _, l := range manual.Wrap(lines, width) {
		text := strings.Repeat(" ", l.Indent) + l.Text
		switch l.Kind {
		case manual.LineTitle:
			text += "\n" + strings.Repeat("═", utf8.RuneCountInString(text))
		case manual.LineHeading:
			text += "\n" + strings.Repeat("─", utf8.RuneCountInString(text))
		}
		if _, err := fmt.Fprintln(stdout, text); err != nil {
			return err
		}
	}
	return nil
}
