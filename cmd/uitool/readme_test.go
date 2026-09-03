package main

import "testing"

// The anchors are what every contents link points at. If the slug rule drifts
// from GitHub's, the table of contents still renders and every entry is a dead
// link — which is exactly the kind of breakage nobody notices in review.
func TestAnchor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Run", "run"},
		{"What's inside", "whats-inside"},
		{"Testing & tooling", "testing--tooling"},
		{"FVF — Harmonic Wobbulator", "fvf--harmonic-wobbulator"},
		{"Related / prior art", "related--prior-art"},
		{"Rabinovich-Fabrikant", "rabinovich-fabrikant"},
		// An ASCII hyphen survives; an en dash does not, and takes no space
		// with it, so the two words run together.
		{"Sprott A (Nosé–Hoover)", "sprott-a-noséhoover"},
		{"Cube (Hexahedron)", "cube-hexahedron"},
		{"**Bold** heading", "bold-heading"},
		{"A `code` heading", "a-code-heading"},
		{"[Linked](https://example.com) heading", "linked-heading"},
	}
	for _, c := range cases {
		if got := anchor(c.in); got != c.want {
			t.Errorf("anchor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// GitHub numbers repeated headings; the models reference has several (every
// category lists its models, and two models appear in two categories).
func TestUniqueAnchor(t *testing.T) {
	counts := map[string]int{}
	want := []string{"custom", "custom-1", "custom-2"}
	for i, w := range want {
		if got := uniqueAnchor("Custom", counts); got != w {
			t.Errorf("call %d = %q, want %q", i, got, w)
		}
	}
}

func TestHeading(t *testing.T) {
	cases := []struct {
		line  string
		level int
		text  string
	}{
		{"## Run", 2, "Run"},
		{"#### Lorenz", 4, "Lorenz"},
		{"#NoSpace", 0, ""},
		{"####### too deep", 0, ""},
		{"not a heading", 0, ""},
		{"### Trailing   ", 3, "Trailing"},
	}
	for _, c := range cases {
		l, txt := heading(c.line)
		if l != c.level || txt != c.text {
			t.Errorf("heading(%q) = (%d,%q), want (%d,%q)", c.line, l, txt, c.level, c.text)
		}
	}
}

// A heading inside a fenced code block is a shell comment, not a heading —
// the README has several, and listing them would put nonsense in the contents.
func TestTableOfContentsIgnoresCodeFences(t *testing.T) {
	doc := "## Real\n\n```\n## Not a heading\n```\n\n### Also real\n"
	got := tableOfContents(doc)
	if want := "- [Real](#real)\n  - [Also real](#also-real)\n"; !contains(got, want) {
		t.Errorf("got:\n%s\nwant it to contain:\n%s", got, want)
	}
	if contains(got, "Not a heading") {
		t.Error("a heading inside a code fence was listed")
	}
}

func TestReplaceRegion(t *testing.T) {
	src := "before\n<!-- BEGIN TOC -->\nold\n<!-- END TOC -->\nafter\n"
	got, err := replaceRegion(src, "TOC", "new")
	if err != nil {
		t.Fatal(err)
	}
	want := "before\n<!-- BEGIN TOC -->\n\nnew\n\n<!-- END TOC -->\nafter\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := replaceRegion("no markers here", "TOC", "x"); err == nil {
		t.Error("missing markers should be an error, not a silent no-op")
	}
}

// The equations trailing a description belong in a code block; a second
// paragraph of prose does not.
func TestSplitDescription(t *testing.T) {
	prose, eq := splitDescription("Lorenz Attractor — a thing.\n\ndx/dt = y\ndy/dt = x")
	if prose != "Lorenz Attractor — a thing." || eq != "dx/dt = y\ndy/dt = x" {
		t.Errorf("equations not split: prose=%q eq=%q", prose, eq)
	}
	prose, eq = splitDescription("First paragraph.\n\nSecond paragraph, no equals sign.")
	if eq != "" {
		t.Errorf("prose was treated as equations: %q", eq)
	}
	if !contains(prose, "Second paragraph") {
		t.Errorf("second paragraph was dropped: %q", prose)
	}
}

func contains(hay, needle string) bool {
	return len(needle) == 0 || len(hay) >= len(needle) && indexOf(hay, needle) >= 0
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
