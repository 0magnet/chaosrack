package manual

import (
	"html"
	"strconv"
	"strings"
)

// miniRender is the manual's markdown as HTML, without a markdown library
// (render.go says why).
//
// It covers what the manual is written in and nothing more: ATX headings,
// paragraphs with soft and hard breaks, flat bulleted and numbered lists
// (loose when a blank line parts two items), fenced code, code spans,
// links, **strong**, *emphasis* and backslash escapes. Anything else comes
// out as the text it is, escaped. Raw HTML is left out, as goldmark leaves
// it out. TestMiniRenderSaysWhatGoldmarkSays holds the two to the same text
// for every entry and page, so a tooltip reads the same in either build.
//
// ids gives each heading an id from its text, as goldmark's
// parser.WithAutoHeadingID does for Page.
func miniRender(md string, ids bool) string {
	r := miniRenderer{ids: ids, seen: map[string]int{}}
	for _, line := range strings.Split(md, "\n") {
		r.line(line)
	}
	if r.inCode {
		r.b.WriteString(html.EscapeString(r.code.String()))
		r.b.WriteString("</code></pre>\n")
	}
	r.flushPara()
	r.closeList()
	return r.b.String()
}

// miniRenderer is miniRender's state between lines.
type miniRenderer struct {
	b      strings.Builder
	ids    bool
	seen   map[string]int // heading ids given, for the -1, -2 goldmark adds
	para   []string       // the paragraph being read, by line
	list   string         // "ul" or "ol" while in a list
	items  []string       // its items, as markdown
	loose  bool           // a blank line has parted two of its items
	blank  bool           // the line before was blank, inside a list
	inCode bool
	code   strings.Builder
}

func (r *miniRenderer) line(line string) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
		if r.inCode {
			r.b.WriteString(html.EscapeString(r.code.String()))
			r.b.WriteString("</code></pre>\n")
			r.code.Reset()
			r.inCode = false
			return
		}
		r.flushPara()
		r.closeList()
		r.inCode = true
		if lang := strings.TrimSpace(t[3:]); lang != "" {
			r.b.WriteString(`<pre><code class="language-` + html.EscapeString(lang) + `">`)
		} else {
			r.b.WriteString("<pre><code>")
		}
		return
	}
	if r.inCode {
		r.code.WriteString(line)
		r.code.WriteByte('\n')
		return
	}
	if t == "" {
		r.flushPara()
		if r.list != "" {
			r.blank = true
		}
		return
	}
	if strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->") {
		return // raw HTML, which goldmark leaves out too
	}
	if n := headingLevel(t); n > 0 {
		r.flushPara()
		r.closeList()
		r.heading(n, strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[n:]), "#")))
		return
	}
	if kind, item, ok := listItem(t); ok {
		r.flushPara()
		if r.list != kind {
			r.closeList()
			r.list = kind
		} else if r.blank {
			r.loose = true
		}
		r.blank = false
		r.items = append(r.items, item)
		return
	}
	if r.list != "" && !r.blank && line != t {
		// An indented line continuing the item before it: the manual's
		// items are one paragraph, so it joins it.
		r.items[len(r.items)-1] += "\n" + t
		return
	}
	r.closeList()
	r.para = append(r.para, line)
}

func (r *miniRenderer) heading(n int, text string) {
	tag := "h" + strconv.Itoa(n)
	r.b.WriteString("<" + tag)
	if r.ids {
		id := headingID(text)
		if k := r.seen[id]; k > 0 {
			r.seen[id] = k + 1
			id += "-" + strconv.Itoa(k)
		} else {
			r.seen[id] = 1
		}
		r.b.WriteString(` id="` + id + `"`)
	}
	r.b.WriteString(">" + miniInline(text) + "</" + tag + ">\n")
}

// flushPara writes the paragraph read so far.
func (r *miniRenderer) flushPara() {
	if len(r.para) == 0 {
		return
	}
	r.b.WriteString("<p>")
	for i, l := range r.para {
		text, hard := strings.CutSuffix(l, "  ")
		if !hard {
			text, hard = strings.CutSuffix(l, "\\")
		}
		r.b.WriteString(miniInline(strings.TrimSpace(text)))
		if i < len(r.para)-1 {
			if hard {
				r.b.WriteString("<br />")
			}
			r.b.WriteByte('\n')
		}
	}
	r.b.WriteString("</p>\n")
	r.para = r.para[:0]
}

// closeList writes the list read so far: each item a paragraph of its own
// when the list is loose, as goldmark writes one.
func (r *miniRenderer) closeList() {
	if r.list == "" {
		return
	}
	r.b.WriteString("<" + r.list + ">\n")
	for _, it := range r.items {
		lines := strings.Split(it, "\n")
		for i := range lines {
			lines[i] = miniInline(lines[i])
		}
		body := strings.Join(lines, "\n")
		if r.loose {
			r.b.WriteString("<li>\n<p>" + body + "</p>\n</li>\n")
		} else {
			r.b.WriteString("<li>" + body + "</li>\n")
		}
	}
	r.b.WriteString("</" + r.list + ">\n")
	r.list, r.items, r.loose, r.blank = "", r.items[:0], false, false
}

// headingLevel is n for a line that is a level-n ATX heading, else 0.
func headingLevel(t string) int {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ') {
		return 0
	}
	return n
}

// listItem reports whether t starts a list item, of which kind ("ul" or
// "ol"), and its text.
func listItem(t string) (kind, item string, ok bool) {
	if len(t) > 1 && (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' ' {
		return "ul", strings.TrimSpace(t[2:]), true
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i < len(t)-1 && (t[i] == '.' || t[i] == ')') && t[i+1] == ' ' {
		return "ol", strings.TrimSpace(t[i+2:]), true
	}
	return "", "", false
}

// headingID is the id goldmark's auto heading IDs give text: lower case,
// letters and digits kept, spaces as hyphens, everything else dropped.
func headingID(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "heading"
	}
	return b.String()
}

// isASCIIPunct is whether c may be backslash-escaped: any ASCII punctuation.
func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// miniInline is one line's inline markup as HTML: backslash escapes, code
// spans, links, **strong** and *emphasis*, and the rest escaped.
func miniInline(s string) string {
	var b, plain strings.Builder
	// tag writes markup, after the text before it, escaped as a whole: a
	// byte at a time would split the manual's Greek and its dots.
	tag := func(t string) {
		b.WriteString(html.EscapeString(plain.String()))
		plain.Reset()
		b.WriteString(t)
	}
	strong, em := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			i++
			plain.WriteByte(s[i])
		case c == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				tag("<code>" + html.EscapeString(s[i+1:i+1+j]) + "</code>")
				i += j + 1
			} else {
				plain.WriteByte('`')
			}
		case c == '[':
			// [text](url)
			end := strings.Index(s[i:], "](")
			if end < 0 {
				plain.WriteByte(c)
				break
			}
			shut := strings.IndexByte(s[i+end+2:], ')')
			if shut < 0 {
				plain.WriteByte(c)
				break
			}
			text, url := s[i+1:i+end], s[i+end+2:i+end+2+shut]
			tag(`<a href="` + html.EscapeString(url) + `">` + miniInline(text) + "</a>")
			i += end + 2 + shut
		case c == '*' && i+1 < len(s) && s[i+1] == '*' && (strong || strings.Contains(s[i+2:], "**")):
			if strong {
				tag("</strong>")
			} else {
				tag("<strong>")
			}
			strong = !strong
			i++
		case c == '*' && (em || (i+1 < len(s) && s[i+1] != ' ' && strings.IndexByte(s[i+1:], '*') >= 0)):
			if em {
				tag("</em>")
			} else {
				tag("<em>")
			}
			em = !em
		default:
			plain.WriteByte(c)
		}
	}
	tag("")
	if strong {
		b.WriteString("</strong>")
	}
	if em {
		b.WriteString("</em>")
	}
	return b.String()
}
