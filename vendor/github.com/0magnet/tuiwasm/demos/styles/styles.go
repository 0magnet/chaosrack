// Package styles shows lipgloss laying out styled blocks.
//
// lipgloss needed no adapter at all — it compiles for js/wasm unchanged. What
// it does need is to be told the terminal is capable: its color profile is
// probed through x/term, which is a stub under js/wasm that reports "not a
// terminal", so left alone it renders everything in plain text.
package styles

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name: "styles",
		Desc: "lipgloss — borders, color, alignment",
		Text: run,
	})
}

func run(w io.Writer, cols, _ int) error {
	if cols <= 0 {
		cols = 80
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#f5f5f5")).
		Background(lipgloss.Color("#7d56f4")).
		Padding(0, 3).
		Render("lipgloss")

	card := func(name, body, color string) string {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(color)).
			Padding(0, 1).
			Width(22).
			Render(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(name) +
				"\n" + body)
	}

	cards := []string{
		card("rounded", "a border and\nsome padding", "#04b575"),
		card("color", "truecolor, straight\nthrough to xterm-go", "#ff5f87"),
		card("layout", "joined side by\nside by lipgloss", "#00b7c3"),
	}

	// Three cards need about 70 columns. Past that they have to stack, or the
	// terminal wraps each one mid-border and the boxes come apart — which is
	// what a window narrower than its contents looks like.
	var row string
	if cols >= 72 {
		row = lipgloss.JoinHorizontal(lipgloss.Top, cards[0], " ", cards[1], " ", cards[2])
	} else {
		row = lipgloss.JoinVertical(lipgloss.Left, cards...)
	}

	note := lipgloss.NewStyle().
		Faint(true).
		Render("no adapter needed — lipgloss builds for js/wasm as it is")

	out := lipgloss.JoinVertical(lipgloss.Left, title, "", row, "", note)
	_, err := io.WriteString(w, strings.ReplaceAll(out, "\n", "\n")+"\n")
	return err
}
