// Package markdown shows glamour rendering markdown to a terminal, which
// brings chroma with it for the fenced code block.
//
// Both compile for js/wasm unchanged. glamour is the heaviest thing here —
// goldmark for parsing, chroma for highlighting — and it still needed no
// adapter, which is a fair sign of how much of the ecosystem is portable once
// it stops asking about the tty.
package markdown

import (
	"io"

	"github.com/0magnet/glamour"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name: "markdown",
		Desc: "glamour + chroma — rendered markdown, highlighted code",
		Text: run,
	})
}

const doc = "# glamour, in a browser\n" +
	"\n" +
	"Markdown rendered to ANSI by **glamour**, with the code block\n" +
	"highlighted by *chroma*. Neither needed a line of glue.\n" +
	"\n" +
	"The adapter this page needs is for `tcell`, which paints cells and\n" +
	"reads keys. Anything that only writes styled text — like this — needs\n" +
	"somewhere to write and nothing else.\n" +
	"\n" +
	"```go\n" +
	"// the whole of what a text demo has to satisfy\n" +
	"func run(w io.Writer, cols, rows int) error {\n" +
	"    out, err := glamour.Render(doc, \"dark\")\n" +
	"    if err != nil {\n" +
	"        return err\n" +
	"    }\n" +
	"    _, err = io.WriteString(w, out)\n" +
	"    return err\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"> The terminal underneath is xterm-go — a VT emulator ported to Go and\n" +
	"> compiled to wasm, so the escape sequences above are parsed by Go too.\n" +
	"\n" +
	"| library   | needed |\n" +
	"| --------- | ------ |\n" +
	"| glamour   | nothing |\n" +
	"| chroma    | nothing |\n" +
	"| goldmark  | nothing |\n"

func run(w io.Writer, cols, _ int) error {
	if cols <= 20 {
		cols = 80
	}
	// Wrap to the terminal rather than glamour's default, or the output is
	// the wrong width on every screen but one.
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(cols-2),
	)
	if err != nil {
		return err
	}
	out, err := r.Render(doc)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}
