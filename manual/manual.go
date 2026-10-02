// Package manual is the rack's manual: every explanation the panel gives,
// written as markdown in this directory, one file to a module or a part of
// the rack. A control's tooltip is its entry as plain text, and the Info
// window's manual is the same entries rendered, so there is one copy of
// everything the rack says about itself, readable here as well as on the
// rack.
//
// An entry is marked by a comment naming its key, which the panel looks it
// up by, usually the control's element id:
//
//	### Solo
//	<!-- key: gen-solo -->
//	Solo — the one generator on the rack's signal and the speakers. …
//
// The entry is everything after the marker up to the next marker or heading.
// A marker at the start of a list item makes the item the entry, which is how
// a selector's positions are written, keyed by the selector and the
// position's value:
//
//   - <!-- key: color-map=5 --> heat — black through red and orange to white
//
// The comments do not show where markdown is rendered, so the files read as
// a manual on their own.
package manual

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
)

//go:embed *.md
var files embed.FS

// Entry is one keyed piece of the manual.
type Entry struct {
	Key      string
	File     string // the file it is in, without .md
	Heading  string // the heading it sits directly under, or ""
	Markdown string // the entry itself
}

var (
	once    sync.Once
	entries map[string]*Entry
	order   []string
	docs    map[string]string // each file's markdown, by name without .md
)

var (
	marker  = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+)?<!--\s*key:\s*(\S+)\s*-->\s?(.*)$`)
	heading = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	fence   = regexp.MustCompile("^\\s*(```|~~~)")
	markers = regexp.MustCompile(`<!--\s*key:\s*\S+\s*-->\s?`)
)

func load() {
	once.Do(func() {
		src := map[string]string{}
		names, err := fs.Glob(files, "*.md")
		if err != nil {
			return
		}
		for _, n := range names {
			if b, err := files.ReadFile(n); err == nil {
				src[strings.TrimSuffix(n, ".md")] = string(b)
			}
		}
		use(src)
	})
}

// use makes src, markdown by file name without .md, the manual.
func use(src map[string]string) {
	entries = map[string]*Entry{}
	docs = src
	order = nil
	texts.Clear()
	names := make([]string, 0, len(src))
	for n := range src {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, e := range Parse(name, src[name]) {
			if _, dup := entries[e.Key]; !dup {
				order = append(order, e.Key)
			}
			entries[e.Key] = e
		}
	}
}

// Use makes src, markdown by file name without .md, the manual in place of
// the copy built in: the files as they are on disk, for whoever is writing
// them and wants to see an edit without rebuilding.
func Use(src map[string]string) {
	once.Do(func() {})
	use(src)
}

// Sources is every file's markdown, by name without .md.
func Sources() map[string]string {
	load()
	out := make(map[string]string, len(docs))
	for k, v := range docs {
		out[k] = v
	}
	return out
}

// Parse is the entries in one file's markdown, in the order they are written.
func Parse(file, src string) []*Entry {
	var (
		out     []*Entry
		cur     *Entry
		body    []string
		item    string // a list item entry's indent; "" for a block entry
		head    string // the latest heading
		gap     bool   // only blank lines since that heading
		inFence bool
	)
	flush := func() {
		if cur != nil {
			cur.Markdown = strings.TrimSpace(strings.Join(body, "\n"))
			out = append(out, cur)
		}
		cur, body, item = nil, nil, ""
	}
	for _, line := range strings.Split(src, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		} else if !inFence {
			if m := heading.FindStringSubmatch(line); m != nil {
				flush()
				head, gap = m[1], true
				continue
			}
			if m := marker.FindStringSubmatch(line); m != nil {
				flush()
				cur = &Entry{Key: m[2], File: file}
				if gap {
					cur.Heading = head
				}
				gap = false
				if m[1] != "" {
					item = strings.Repeat(" ", len(m[1]))
				}
				body = []string{m[3]}
				continue
			}
			if strings.TrimSpace(line) != "" {
				gap = false
			}
		}
		if cur == nil {
			continue
		}
		// A list item runs on through the lines indented under it.
		if item != "" && !inFence && !strings.HasPrefix(line, item) {
			if strings.TrimSpace(line) == "" {
				body = append(body, "")
				continue
			}
			flush()
			continue
		}
		if item != "" {
			line = strings.TrimPrefix(line, item)
		}
		body = append(body, line)
	}
	flush()
	return out
}

// Lookup is the entry for key.
func Lookup(key string) (*Entry, bool) {
	load()
	e, ok := entries[key]
	return e, ok
}

// Has reports whether the manual has an entry for key.
func Has(key string) bool {
	_, ok := Lookup(key)
	return ok
}

// Keys is every key, in the order the files and their entries are written.
func Keys() []string {
	load()
	return append([]string(nil), order...)
}

// Text is key's entry as plain text, for a tooltip: paragraphs apart by a
// blank line, a line break kept, list items as bullets. "" when there is
// none.
func Text(key string) string {
	if t, ok := texts.Load(key); ok {
		return t.(string)
	}
	e, ok := Lookup(key)
	if !ok {
		return ""
	}
	t := PlainText(e.Markdown)
	texts.Store(key, t)
	return t
}

// texts is Text's answers: every tooltip on the panel is one, and the panel
// asks for most of them more than once.
var texts sync.Map

// HTML is key's entry rendered, for the Info window; "" when there is none.
func HTML(key string) string {
	e, ok := Lookup(key)
	if !ok {
		return ""
	}
	return Render(e.Markdown)
}

// Positions is the keys of a selector's positions, key=value, in the order
// they are written.
func Positions(key string) []string {
	load()
	var out []string
	for _, k := range order {
		if strings.HasPrefix(k, key+"=") {
			out = append(out, k)
		}
	}
	return out
}

// Files is the names of the manual's files, without .md, sorted.
func Files() []string {
	load()
	n := make([]string, 0, len(docs))
	for k := range docs {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Render is markdown as HTML. Raw HTML in it is left out.
func Render(md string) string {
	var b bytes.Buffer
	if err := goldmark.Convert([]byte(md), &b); err != nil {
		return "<p>" + html.EscapeString(md) + "</p>"
	}
	return b.String()
}

var (
	tagBreak = strings.NewReplacer(
		"<br>\n", "\n", "<br />\n", "\n", "<br>", "\n", "<br />", "\n",
		"</p>", "\n\n", "</pre>", "\n\n", "</li>", "\n", "<li>", "• ",
		"</h1>", "\n\n", "</h2>", "\n\n", "</h3>", "\n\n", "</h4>", "\n\n",
		"</h5>", "\n\n", "</h6>", "\n\n", "</ul>", "\n", "</ol>", "\n",
		"</blockquote>", "\n\n",
	)
	tags    = regexp.MustCompile(`<[^>]*>`)
	blanks  = regexp.MustCompile(`\n{3,}`)
	listGap = regexp.MustCompile(`\n\n• `)
)

// PlainText is markdown as the text a tooltip shows.
func PlainText(md string) string {
	if s, ok := plainFast(md); ok {
		return s
	}
	s := tagBreak.Replace(Render(md))
	s = html.UnescapeString(tags.ReplaceAllString(s, ""))
	s = blanks.ReplaceAllString(s, "\n\n")
	s = listGap.ReplaceAllString(s, "\n• ")
	return strings.TrimSpace(s)
}
