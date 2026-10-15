package manual

import (
	"strings"
	"unicode/utf8"
)

// A bay's page of the manual, the service manual's page for the part of the
// rack in front of you: each module on the bay, what it is, and each of its
// controls by address with its entry. The panel reads which modules and
// controls are there, and their addresses, off itself (an address is where a
// control is, and only the panel knows that); what each one is comes from
// here. The page shows it in its Info window (infomanual_js.go), a terminal
// panel in its Info pane, and both use what is below.

// Module is one module of a bay as the panel reads it: its header's name,
// what its header's tooltip says, the addresses it covers, its addressed
// controls, and the keys the manual may have it under.
type Module struct {
	Name  string   `json:"h"`
	About string   `json:"t"`
	Range string   `json:"r"`
	Cells []Cell   `json:"c"`
	Keys  []string `json:"k"`
}

// Cell is one addressed control: its address, what its tooltip says, and
// the keys the manual may have it under, nearest first.
type Cell struct {
	Loc  string   `json:"l"`
	Tip  string   `json:"t"`
	Keys []string `json:"k"`
}

// EntryKey is which of keys a part's tooltip tip is the entry of: a tooltip
// is its entry as plain text, unless the panel has rewritten it since, and
// then it is the newer of the two and no key is.
func EntryKey(tip string, keys []string) (string, bool) {
	for _, k := range keys {
		if Text(k) == tip {
			return k, true
		}
	}
	return "", false
}

// PartLists are the lists a part's page carries under it: a pin matrix's
// columns and rows, each of which is an entry of its own (the tooltip over
// it) and none of which is a control with an address.
var PartLists = map[string][]string{
	"mixer-matrix": {"mix-group", "mix-src", "mix-row"},
	"mod-grid":     {"mod-group", "mod-src"},
}

// EntryText is a part's page as text: its entry, when tip is the entry of
// one of keys, else the tooltip as it stands; and under it a selector's
// positions, or a matrix's lists, as bullets.
func EntryText(tip string, keys []string) string {
	var b strings.Builder
	k, found := EntryKey(tip, keys)
	if found {
		b.WriteString(Text(k))
		for _, l := range PartLists[k] {
			b.WriteString(bullets(Positions(l)))
		}
	} else {
		b.WriteString(tip)
	}
	for _, k := range keys {
		if ps := Positions(k); len(ps) > 0 {
			b.WriteString(bullets(ps))
			break
		}
	}
	return b.String()
}

// bullets is entries keys as a list, a bullet each.
func bullets(keys []string) string {
	var b strings.Builder
	for _, k := range keys {
		b.WriteString("\n• " + strings.TrimSpace(Text(k)))
	}
	return b.String()
}

// Line is one line of a page as text: what it says, how far in it stands,
// and what kind of line it is. A long line is wrapped under its own indent
// (Wrap).
type Line struct {
	Text   string
	Indent int
	Kind   LineKind
	// Loc is the address of the control the line is about, on its first
	// line, so a reader can be scrolled to it.
	Loc string
}

// LineKind is what a line of a page is.
type LineKind int

// The kinds of line.
const (
	LineBody    LineKind = iota
	LineTitle            // the page's title
	LineHeading          // a module's name and addresses
	LineAddress          // a control's address, leading its entry
)

// BayLines is bay page mods as lines of text, under title: each module's
// name and addresses, what it is, then each of its controls by address with
// its entry. A tooltip given already in the module refers back to where.
func BayLines(title string, mods []Module) []Line {
	out := []Line{{Text: title, Kind: LineTitle}}
	for _, m := range mods {
		h := m.Name
		if m.Range != "" {
			h += "   " + m.Range
		}
		out = append(out, Line{}, Line{Text: h, Kind: LineHeading})
		if m.About != "" {
			out = append(out, paragraphs(EntryText(m.About, m.Keys), 0)...)
		}
		said := map[string]string{}
		for _, c := range m.Cells {
			text := EntryText(c.Tip, c.Keys)
			if at, ok := said[c.Tip]; ok {
				text = "As " + at + "."
			} else {
				said[c.Tip] = c.Loc
			}
			out = append(out, Line{Text: c.Loc, Indent: 1, Kind: LineAddress, Loc: c.Loc})
			out = append(out, paragraphs(text, 3)...)
		}
	}
	return out
}

// TextLines is text, paragraphs apart by blank lines, as lines under a
// title: a model's description.
func TextLines(title, text string) []Line {
	return append([]Line{{Text: title, Kind: LineTitle}}, paragraphs(text, 0)...)
}

// paragraphs is text as body lines indent in, a blank line between its
// paragraphs.
func paragraphs(text string, indent int) []Line {
	var out []Line
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		out = append(out, Line{Text: strings.TrimRight(l, " "), Indent: indent})
	}
	return out
}

// Wrap is lines wrapped to width columns, each under its own indent, a
// bullet's text under its text.
func Wrap(lines []Line, width int) []Line {
	var out []Line
	for _, l := range lines {
		pad := l.Indent
		if strings.HasPrefix(l.Text, "• ") {
			pad += 2 // under the text, not the bullet
		}
		first := true
		rest := l.Text
		w := max(width-l.Indent, 10)
		for {
			if utf8.RuneCountInString(rest) <= w || l.Text == "" {
				out = append(out, Line{Text: rest, Indent: l.Indent, Kind: l.Kind, Loc: firstLoc(l, first)})
				break
			}
			cut := breakAt(rest, w)
			out = append(out, Line{Text: strings.TrimRight(rest[:cut], " "), Indent: l.Indent, Kind: l.Kind, Loc: firstLoc(l, first)})
			rest = strings.TrimLeft(rest[cut:], " ")
			if first {
				first = false
				l.Indent, w = pad, max(width-pad, 10)
			}
		}
	}
	return out
}

func firstLoc(l Line, first bool) string {
	if first {
		return l.Loc
	}
	return ""
}

// breakAt is where to break s to fit w columns: after the last space that
// fits, or at w when no space does.
func breakAt(s string, w int) int {
	at, n := -1, 0
	for i, r := range s {
		if n == w {
			if at > 0 {
				return at
			}
			return i
		}
		if r == ' ' {
			at = i + 1
		}
		n++
	}
	return len(s)
}

// FirstSentence is the first sentence of text, for a one-line hint: up to
// the first full stop that ends a sentence, or the first line.
func FirstSentence(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	for i := 0; i+1 < len(text); i++ {
		if text[i] == '.' && text[i+1] == ' ' {
			return text[:i+1]
		}
	}
	return text
}
