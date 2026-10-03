package manual

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// goldmarkHTML is md as goldmark renders it, the reference miniRender is
// held to. A test dependency only: the program does not carry it.
func goldmarkHTML(t *testing.T, md string, page bool) string {
	t.Helper()
	g := goldmark.New()
	if page {
		g = goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	}
	var b bytes.Buffer
	if err := g.Convert([]byte(md), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The manual is rendered by miniRender rather than a markdown library. Held
// to the text goldmark gives, entry by entry and page by page, so a tooltip
// or a chapter says what a full CommonMark renderer would have it say.
func TestMiniRenderSaysWhatGoldmarkSays(t *testing.T) {
	for _, k := range Keys() {
		e, _ := Lookup(k)
		gold, mini := plainOf(goldmarkHTML(t, e.Markdown, false)), plainOf(Render(e.Markdown))
		if gold != mini {
			t.Errorf("entry %s:\n goldmark %q\n mini     %q", k, gold, mini)
		}
	}
	for _, f := range Files() {
		src := markers.ReplaceAllString(Sources()[f], "")
		gold, mini := plainOf(goldmarkHTML(t, src, true)), plainOf(renderPage(src))
		if gold == mini {
			continue
		}
		// The first line that differs, not two whole chapters.
		g, m := strings.Split(gold, "\n"), strings.Split(mini, "\n")
		for i := range min(len(g), len(m)) {
			if g[i] != m[i] {
				t.Errorf("page %s, line %d:\n goldmark %q\n mini     %q", f, i+1, g[i], m[i])
				break
			}
		}
		if len(g) != len(m) {
			t.Errorf("page %s: %d lines from goldmark, %d from mini", f, len(g), len(m))
		}
	}
}

// And the ids a page's headings are given, which the chapters link by.
func TestMiniRenderHeadingIDsAreGoldmarks(t *testing.T) {
	for _, f := range Files() {
		src := markers.ReplaceAllString(Sources()[f], "")
		gold, mini := headingIDs(goldmarkHTML(t, src, true)), headingIDs(renderPage(src))
		if strings.Join(gold, " ") != strings.Join(mini, " ") {
			t.Errorf("page %s:\n goldmark %v\n mini     %v", f, gold, mini)
		}
	}
}

// headingIDs is every id="…" on a heading in h, in order.
func headingIDs(h string) []string {
	var out []string
	for _, part := range strings.Split(h, "<h")[1:] {
		if i := strings.Index(part, `id="`); i >= 0 && i < 4 {
			rest := part[i+4:]
			out = append(out, rest[:strings.IndexByte(rest, '"')])
		}
	}
	return out
}
