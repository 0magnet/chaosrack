// Package tables shows go-pretty rendering a table.
//
// This is the compat matrix for the libraries themselves, which makes the
// demo its own documentation: what you are reading in the table is the reason
// the table can be drawn at all.
package tables

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name: "tables",
		Desc: "go-pretty — the wasm compatibility matrix",
		Text: run,
	})
}

func run(w io.Writer, _, _ int) error {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.SetStyle(table.StyleRounded)
	t.Style().Options.SeparateRows = false
	t.Style().Color.Header = text.Colors{text.Bold, text.FgHiCyan}

	t.AppendHeader(table.Row{"library", "js/wasm", "what it needed"})
	for _, r := range []table.Row{
		{"tcell", "yes", "ships a wasm screen of its own"},
		{"tview", "yes", "nothing — it is tcell underneath"},
		{"termdash", "yes", "nothing — also tcell"},
		{"lipgloss", "yes", "nothing"},
		{"glamour", "yes", "nothing"},
		{"chroma", "yes", "nothing"},
		{"go-pretty", "yes", "nothing"},
		{"asciigraph", "yes", "nothing"},
		{"progressbar", "yes", "nothing"},
		{"bubbletea", "one file", "4 symbols: resize, input, suspend"},
		{"pterm", "one file", "4 symbols in atomicgo.dev/keyboard"},
		{"bubbles / huh", "one file", "a clipboard for js"},
		{"readline", "no", "termios state, all the way down"},
		{"termbox-go", "no", "same, and archived upstream"},
		{"termui / gocui", "no", "both sit on termbox"},
		{"creack/pty", "never", "there are no ptys in a browser"},
	} {
		t.AppendRow(r)
	}
	t.Render()
	return nil
}
