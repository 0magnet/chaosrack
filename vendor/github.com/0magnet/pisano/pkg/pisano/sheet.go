package pisano

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"
)

// Tile is one finished figure: SVG markup drawn inside a square box whose side
// is Size, plus the line that goes under it.
//
// Both renderers produce these, which is what lets one sheet hold circular
// designs and turtle paths side by side, and lets the same figures go to an SVG
// file or an HTML page without either renderer knowing which.
type Tile struct {
	Caption string
	Body    string
	Size    float64
}

// Theme selects the palette a sheet is drawn with.
//
// Auto is right for anything a person opens directly: the figure follows
// whatever the viewer has their system set to. It is wrong for anything that
// gets converted to a fixed image, or embedded somewhere that strips the style
// block, since a media query cannot survive either — hence the fixed choices.
type Theme int

const (
	ThemeAuto Theme = iota
	ThemeLight
	ThemeDark
)

// ParseTheme reads a theme name.
func ParseTheme(s string) (Theme, error) {
	switch s {
	case "", "auto":
		return ThemeAuto, nil
	case "light":
		return ThemeLight, nil
	case "dark":
		return ThemeDark, nil
	}
	return ThemeAuto, fmt.Errorf("unknown theme %q: want auto, light or dark", s)
}

// Sheet lays tiles out in a grid.
type Sheet struct {
	Cell   int  // pixels per tile
	Cols   int  // tiles per row; 0 picks a roughly square sheet
	Labels bool // caption each tile
	Theme  Theme
}

// TileSize is the side of the box a tile draws into, given the sheet's cell.
func (s Sheet) TileSize() float64 {
	cell := float64(s.cell())
	return cell - 2*s.pad() - s.labelHeight()
}

func (s Sheet) cell() int {
	if s.Cell <= 0 {
		return 180
	}
	return s.Cell
}

func (s Sheet) pad() float64 { return float64(s.cell()) * 0.06 }

func (s Sheet) labelHeight() float64 {
	if !s.Labels {
		return 0
	}
	return math.Max(11, float64(s.cell())*0.085)
}

func (s Sheet) cols(n int) int {
	c := s.Cols
	if c <= 0 {
		c = int(math.Ceil(math.Sqrt(float64(n))))
	}
	if c > n {
		c = n
	}
	if c < 1 {
		c = 1
	}
	return c
}

// Stroke is the line width tiles should draw with at this scale.
func (s Sheet) Stroke() float64 { return math.Max(0.6, float64(s.cell())/220) }

// captionSize shrinks a caption that would otherwise run past its tile and
// collide with its neighbor. SVG will not wrap or ellipsize text for you, and
// the captions here vary in length — "m=7 · 16 · open" against
// "triangular · 4 · open" — so the size has to come from the text.
//
// The 0.62 is the advance width of a monospace glyph as a fraction of the font
// size, which holds closely enough across the fallback stack in styleBlock.
func (s Sheet) captionSize(caption string) float64 {
	size := s.labelHeight() * 0.82
	n := len([]rune(caption))
	if n == 0 {
		return size
	}
	if fit := (float64(s.cell()) - s.pad()) / (0.62 * float64(n)); fit < size {
		size = fit
	}
	return math.Max(5, size)
}

// WriteSVG renders tiles as one SVG document.
func (s Sheet) WriteSVG(w io.Writer, tiles []Tile) error {
	if len(tiles) == 0 {
		return fmt.Errorf("sheet: nothing to render")
		// bufio.Writer latches its first error and turns every later write into a
		// no-op, so the Flush at the end is what reports a failure. That is why the
		// writes below are individually nolint:errcheck rather than each checked:
		// checking them would report the same error over and over.
	}
	bw := bufio.NewWriter(w)

	cell := float64(s.cell())
	cols := s.cols(len(tiles))
	rows := (len(tiles) + cols - 1) / cols
	width, height := float64(cols)*cell, float64(rows)*cell
	inner := s.TileSize()

	fmt.Fprintf(bw, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`+"\n", //nolint:errcheck
		width, height, width, height)
	fmt.Fprint(bw, styleBlock(s.Theme))                                                  //nolint:errcheck
	fmt.Fprintf(bw, `<rect class="bg" width="%.0f" height="%.0f"/>`+"\n", width, height) //nolint:errcheck

	for i, t := range tiles {
		ox := float64(i%cols) * cell
		oy := float64(i/cols) * cell
		fmt.Fprintf(bw, `<g transform="translate(%.2f,%.2f)">`+"\n", ox+(cell-inner)/2, oy+s.pad()) //nolint:errcheck
		fmt.Fprint(bw, t.Body)                                                                      //nolint:errcheck
		fmt.Fprint(bw, "</g>\n")                                                                    //nolint:errcheck
		if s.Labels && t.Caption != "" {
			fmt.Fprintf(bw, `<text class="cap" x="%.2f" y="%.2f" font-size="%.1f">%s</text>`+"\n", //nolint:errcheck
				ox+cell/2, oy+cell-s.pad()*0.5, s.captionSize(t.Caption), svgEscape(t.Caption))
		}
	}

	fmt.Fprint(bw, "</svg>\n") //nolint:errcheck
	return bw.Flush()
}

// Section is a titled group of tiles on an HTML page.
type Section struct {
	Title string
	Note  string
	Tiles []Tile
	Min   int // smallest tile width in px before the grid reflows; 0 for 180
}

// WriteHTML renders sections as a self-contained page: every figure is inline
// SVG, so there is nothing to fetch and nothing to break. The grid reflows, so
// the same page works as a gallery to browse and as a place to lift individual
// figures from for a site.
func WriteHTML(w io.Writer, title string, secs []Section) error {
	return WriteHTMLPage(w, title, "", secs)
}

// WriteHTMLPage is WriteHTML with a lead paragraph. The lead is raw HTML and is
// written verbatim, so it is for the caller's own markup — a link to somewhere
// else on the site, say — and not for anything that came from outside.
func WriteHTMLPage(w io.Writer, title, lead string, secs []Section) error {
	bw := bufio.NewWriter(w)

	fmt.Fprintf(bw, "<!doctype html>\n<html lang=\"en\">\n<meta charset=\"utf-8\">\n"+ //nolint:errcheck
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n"+
		"<title>%s</title>\n", svgEscape(title))
	fmt.Fprint(bw, htmlStyle())                        //nolint:errcheck
	fmt.Fprintf(bw, "<h1>%s</h1>\n", svgEscape(title)) //nolint:errcheck
	if lead != "" {
		fmt.Fprintf(bw, "<p class=\"lead\">%s</p>\n", lead) //nolint:errcheck
	}

	for _, sec := range secs {
		if len(sec.Tiles) == 0 {
			continue
		}
		min := sec.Min
		if min <= 0 {
			min = 180
		}
		if sec.Title != "" {
			fmt.Fprintf(bw, "<h2>%s</h2>\n", svgEscape(sec.Title)) //nolint:errcheck
		}
		if sec.Note != "" {
			fmt.Fprintf(bw, "<p class=\"note\">%s</p>\n", svgEscape(sec.Note)) //nolint:errcheck
		}
		fmt.Fprintf(bw, `<div class="grid" style="--min:%dpx">`+"\n", min) //nolint:errcheck
		for _, t := range sec.Tiles {
			fmt.Fprint(bw, `<figure>`)                                                          //nolint:errcheck
			fmt.Fprintf(bw, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.2f %.2f">`, //nolint:errcheck
				t.Size, t.Size)
			fmt.Fprint(bw, t.Body)   //nolint:errcheck
			fmt.Fprint(bw, `</svg>`) //nolint:errcheck
			if t.Caption != "" {
				fmt.Fprintf(bw, `<figcaption>%s</figcaption>`, svgEscape(t.Caption)) //nolint:errcheck
			}
			fmt.Fprint(bw, "</figure>\n") //nolint:errcheck
		}
		fmt.Fprint(bw, "</div>\n") //nolint:errcheck
	}

	fmt.Fprint(bw, "</html>\n") //nolint:errcheck
	return bw.Flush()
}

// palette is every color a sheet uses. Keeping them together is what makes a
// second theme a data change rather than a second stylesheet.
type palette struct {
	bg, ring, chord, pt, caption string
	pass                         [6]string
}

var (
	lightPalette = palette{
		bg: "#ffffff", ring: "#d8d8d8", chord: "#1b3a5c", pt: "#b03a2e", caption: "#666",
		pass: [6]string{"#1b3a5c", "#b03a2e", "#2e7d5b", "#7a5aa8", "#b8860b", "#2e6d8e"},
	}
	darkPalette = palette{
		bg: "#12141a", ring: "#2c313c", chord: "#8fc6f0", pt: "#e8825f", caption: "#7d8694",
		pass: [6]string{"#8fc6f0", "#e8825f", "#7fd1a8", "#b9a3e3", "#e0c060", "#6fb3d2"},
	}
)

// rules writes the palette as CSS declarations for one theme.
func (p palette) rules() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  .bg    { fill: %s; }\n", p.bg)
	fmt.Fprintf(&b, "  .ring  { stroke: %s; }\n", p.ring)
	fmt.Fprintf(&b, "  .chord, .path { stroke: %s; }\n", p.chord)
	fmt.Fprintf(&b, "  .pt    { fill: %s; }\n", p.pt)
	fmt.Fprintf(&b, "  .cap   { fill: %s; }\n", p.caption)
	for i, c := range p.pass {
		fmt.Fprintf(&b, "  .p%d { stroke: %s; }\n", i, c)
	}
	return b.String()
}

// styleBlock keeps color out of the geometry, so a figure is the same markup
// whatever palette it lands in.
func styleBlock(t Theme) string {
	const shape = `  .ring  { fill: none; stroke-width: 0.5; }
  .chord { fill: none; stroke-linecap: round; stroke-opacity: 0.85; }
  .path  { fill: none; stroke-linecap: round; stroke-linejoin: round; }
  .cap   { text-anchor: middle;
           font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
`
	var b strings.Builder
	b.WriteString("<style>\n")
	b.WriteString(shape)
	switch t {
	case ThemeLight:
		b.WriteString(lightPalette.rules())
	case ThemeDark:
		b.WriteString(darkPalette.rules())
	default:
		b.WriteString(lightPalette.rules())
		b.WriteString("  @media (prefers-color-scheme: dark) {\n")
		b.WriteString(darkPalette.rules())
		b.WriteString("  }\n")
	}
	b.WriteString("</style>\n")
	return b.String()
}

func htmlStyle() string {
	return `<style>
  :root { color-scheme: light dark;
          --bg: #ffffff; --fg: #1a1d23; --muted: #6b7280; --line: #e3e6ea; }
  @media (prefers-color-scheme: dark) {
    :root { --bg: #12141a; --fg: #e6e9ee; --muted: #7d8694; --line: #262b34; }
  }
  body { margin: 0 auto; padding: 2rem 1.25rem 4rem; max-width: 1200px;
         background: var(--bg); color: var(--fg);
         font: 15px/1.6 ui-sans-serif, system-ui, -apple-system, sans-serif; }
  h1 { font-size: 1.5rem; font-weight: 600; margin: 0 0 .25rem; }
  h2 { font-size: 1rem; font-weight: 600; margin: 2.5rem 0 .25rem;
       padding-bottom: .4rem; border-bottom: 1px solid var(--line); }
  .note { color: var(--muted); margin: .35rem 0 1.25rem; max-width: 62ch; }
  .lead { margin: .35rem 0 2rem; max-width: 62ch; }
  .lead a { color: inherit; }
  .grid { display: grid; gap: 1rem;
          grid-template-columns: repeat(auto-fill, minmax(var(--min, 180px), 1fr)); }
  figure { margin: 0; }
  figure svg { display: block; width: 100%; height: auto; overflow: visible; }
  figcaption { margin-top: .4rem; text-align: center; color: var(--muted);
               overflow-wrap: anywhere;
               font: 12px/1.4 ui-monospace, SFMono-Regular, Menlo, monospace; }
  .bg { fill: none; }
` + strings.TrimPrefix(strings.TrimSuffix(styleBlock(ThemeAuto), "</style>\n"), "<style>\n") + `</style>
`
}

func svgEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
