// Package toilet renders text with FIGlet fonts, applies color and geometry
// filters to the result and writes it out in any of libcaca's export formats.
//
// It is a Go port of TOIlet 0.3 by Sam Hocevar, whose own rendering and export
// work is done by libcaca; the canvas and codecs come from
// github.com/0magnet/img2txt-go/caca and the FIGfont handling from the figlet
// package next door.
//
// Original C implementation copyright © 2006 Sam Hocevar <sam@hocevar.net>,
// released under the WTFPL.
package toilet

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/0magnet/img2txt-go/caca"
	"github.com/0magnet/toilet-go/canvas"
	"github.com/0magnet/toilet-go/figlet"
	"github.com/0magnet/toilet-go/fonts"
)

// Version is the TOIlet release this port follows.
const Version = "0.3"

// DefaultFontDir is where the toilet and figlet packages install their fonts.
// It is compiled into the original as FONTDIR.
const DefaultFontDir = "/usr/share/figlet"

// DefaultFont is the font toilet picks when none is named.
const DefaultFont = "ascii9"

// Context holds one rendering job's settings, mirroring toilet's context_t.
type Context struct {
	// Export is the name of a libcaca export format.
	Export string
	// Font is a font name, resolved inside Dir, or "term" for the built-in
	// one-cell-per-character renderer.
	Font string
	// Dir is the directory searched for fonts.
	Dir string
	// TermWidth is the column at which a render wraps.
	TermWidth int
	// Mode names the layout mode: default, smush, kern, none or overlap.
	Mode string
	// Filters lists the post-processing filters to run, in order.
	Filters []string

	// lines counts the rows written so far, and shifts the rainbow and metal
	// patterns. toilet initializes it and never advances it — the counter that
	// does advance lives inside libcaca's figfont state and is not the one the
	// filters read — so in practice every line is colored alike. Kept as it
	// is, because it is visible in the output.
	lines int

	drv driver
}

// New returns a context with toilet's defaults.
func New() *Context {
	return &Context{
		Export:    "utf8",
		Font:      DefaultFont,
		Dir:       DefaultFontDir,
		TermWidth: 80,
		Mode:      "default",
	}
}

// driver is a text-to-canvas renderer: either a FIGfont or the built-in "term"
// font, which is one canvas cell per character.
type driver interface {
	feed(ch rune, attr uint32)
	flush() *canvas.Canvas
}

// AddFilter appends one or more colon-separated filter names. It is
// filter_add(), including its right-to-left name matching, so that "180" is
// found before "left" would be tried.
func (c *Context) AddFilter(spec string) error {
	for {
		spec = strings.TrimLeft(spec, ":")
		if spec == "" {
			return nil
		}

		found := -1
		var n int
		for i := len(filters) - 1; i >= 0; i-- {
			if strings.HasPrefix(spec, filters[i].name) {
				found = i
				n = len(filters[i].name)
				break
			}
		}

		if found == -1 || (n < len(spec) && spec[n] != ':') {
			return fmt.Errorf("unknown filter near `%s'", spec)
		}

		c.Filters = append(c.Filters, filters[found].name)
		spec = spec[n:]
	}
}

// SetExport selects an export format, rejecting names libcaca does not know.
func (c *Context) SetExport(format string) error {
	c.Export = format
	for _, e := range caca.ExportList() {
		if e[0] == format {
			return nil
		}
	}
	return fmt.Errorf("unknown export format `%s'", format)
}

// FontFile resolves a font name to a path, searching the font directory and
// then the working directory, which is the order init_figlet() uses.
func (c *Context) FontFile() []string {
	return []string{filepath.Join(c.Dir, c.Font), "./" + c.Font}
}

// Init loads the font and prepares the renderer. It must be called before
// Render, which calls it for you if you have not.
//
// The error it returns for a font it cannot find is worded as toilet words it,
// leading "error: " and all, because the command prints it verbatim.
func (c *Context) Init() error {
	if strings.EqualFold(c.Font, "term") {
		c.drv = newTinyDriver(c.TermWidth)
		return nil
	}

	var font *figlet.Font
	for _, p := range c.FontFile() {
		if f, err := figlet.LoadFont(p); err == nil {
			font = f
			break
		}
	}
	if font == nil {
		// Nothing on disk matched, so fall back to the bundled collection.
		// The original has no such fallback.
		if data, ok := fonts.Get(c.Font); ok {
			if f, err := figlet.ParseFont(data); err == nil {
				font = f
			}
		}
	}
	if font == nil {
		return fmt.Errorf("error: could not load font %s", c.Font)
	}

	r := figlet.NewRenderer(font)
	r.SetMode(figlet.ParseMode(c.Mode))
	r.SetWidth(c.TermWidth)
	c.drv = &figletDriver{r: r}

	return nil
}

// figletDriver renders through a FIGfont. Input attributes are discarded, as
// they are in toilet's feed_figlet().
type figletDriver struct{ r *figlet.Renderer }

func (d *figletDriver) feed(ch rune, _ uint32) { d.r.PutChar(ch) }
func (d *figletDriver) flush() *canvas.Canvas  { return d.r.Flush() }

// Render writes the rendered form of args, or of r when args is empty, to w.
func (c *Context) Render(args []string, r io.Reader, w io.Writer) error {
	if c.drv == nil {
		if err := c.Init(); err != nil {
			return err
		}
	}
	if len(args) == 0 {
		return c.renderReader(r, w)
	}
	return c.renderList(args, w)
}

// maxInputLine is the size of toilet's stdin buffer. Longer lines are fed in
// pieces, each of which renders as a line of its own.
const maxInputLine = 1024

// renderReader renders one line at a time from r.
func (c *Context) renderReader(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)

	for {
		line, ok := readLine(br, maxInputLine)
		if !ok {
			return nil
		}
		// The original measures the line it just read with strlen(), so a
		// line with a NUL byte in it is cut short there.
		if i := bytes.IndexByte(line, 0); i >= 0 {
			line = line[:i]
		}
		c.feedText(line)
		if err := c.flush(w); err != nil {
			return err
		}
	}
}

// readLine reads up to max-1 bytes, stopping after a newline. It is fgets().
func readLine(br *bufio.Reader, max int) ([]byte, bool) {
	buf := make([]byte, 0, max)
	for len(buf) < max-1 {
		b, err := br.ReadByte()
		if err != nil {
			return buf, len(buf) > 0
		}
		buf = append(buf, b)
		if b == '\n' {
			break
		}
	}
	return buf, true
}

// renderList renders the command line arguments, joined by spaces, splitting a
// new output line at every embedded newline.
func (c *Context) renderList(args []string, w io.Writer) error {
	var parser string
	held := false

	for j := 0; j < len(args); {
		if !held {
			if j != 0 {
				// A space separates one argument from the next.
				c.drv.feed(' ', 0)
			}
			parser, held = args[j], true
		}

		cr := strings.IndexByte(parser, '\n')
		piece := parser
		if cr >= 0 {
			piece = parser[:cr+1]
		}

		c.feedText([]byte(piece))

		if cr >= 0 {
			parser = parser[cr+1:]
			if err := c.flush(w); err != nil {
				return err
			}
		} else {
			held = false
			j++
		}
	}

	return c.flush(w)
}

// feedText imports a run of text into a scratch canvas and feeds its first row
// to the driver, so that escape sequences in the input become cell attributes
// before they reach the renderer.
func (c *Context) feedText(text []byte) {
	cv := canvas.New(0, 0)
	cv.ImportUTF8(text)

	for i := 0; i < cv.Width; i++ {
		ch := cv.GetChar(i, 0)
		c.drv.feed(ch, cv.GetAttr(i, 0))
		if caca.IsFullwidth(ch) {
			i++
		}
	}
}

// flush renders the pending line, runs the filters over it and writes it out.
func (c *Context) flush(w io.Writer) error {
	cv := c.drv.flush()

	for _, name := range c.Filters {
		for _, f := range filters {
			if f.name == name {
				f.fn(cv, c.lines)
				break
			}
		}
	}

	buf, ok := cv.Export(c.Export)
	if !ok {
		return fmt.Errorf("unknown export format `%s'", c.Export)
	}
	_, err := w.Write(buf)
	return err
}
