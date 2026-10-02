package manual

import (
	"html"
	"strings"
	"testing"
)

func TestAnEscapedStringComesBackAsItWas(t *testing.T) {
	for _, s := range []string{
		"Solo — the one generator on the rack's signal",
		"a_b *c* [d](e) <f> & `g` \\h",
		"# not a heading\n- not an item\n1. not numbered\n= not an underline",
		"Lorenz — the butterfly.\n\ndx/dt = σ(y − x)\ndy/dt = x(ρ − z) − y\ndz/dt = xy − βz",
		"one line\nand the next, kept apart",
		"L−R, 0–100 %, 3 > 2 < 4 &amp; done",
		"20 Hz…20 kHz; x² + y² ≤ 1",
	} {
		if got := PlainText(Escape(s)); got != s {
			t.Errorf("round trip of %q\n gave %q\n  via %q", s, got, Escape(s))
		}
	}
}

func TestEntriesAreFoundByTheirMarkers(t *testing.T) {
	src := strings.Join([]string{
		"# Mixer",
		"<!-- key: mixer -->",
		"Mixer — every **sound** on the rack.",
		"",
		"More about it.",
		"",
		"## SPK L",
		"",
		"<!-- key: mix-spkl -->",
		"The left speaker.",
		"",
		"- <!-- key: mix-src=g1 --> GEN 1 — the first",
		"  generator",
		"- <!-- key: mix-src=g2 --> GEN 2",
		"- an unkeyed item",
		"",
		"```text",
		"# not a heading",
		"```",
	}, "\n")
	es := Parse("mixer", src)
	if len(es) != 4 {
		t.Fatalf("%d entries, want 4: %+v", len(es), es)
	}
	want := []struct{ key, head, text string }{
		{"mixer", "Mixer", "Mixer — every sound on the rack.\n\nMore about it."},
		{"mix-spkl", "SPK L", "The left speaker."},
		{"mix-src=g1", "", "GEN 1 — the first\ngenerator"},
		{"mix-src=g2", "", "GEN 2"},
	}
	for i, w := range want {
		e := es[i]
		if e.Key != w.key || e.Heading != w.head || PlainText(e.Markdown) != w.text {
			t.Errorf("entry %d = %q under %q: %q; want %q under %q: %q", i, e.Key, e.Heading, PlainText(e.Markdown), w.key, w.head, w.text)
		}
	}
}

func TestEveryKeyIsWrittenOnce(t *testing.T) {
	seen := map[string]string{}
	for _, f := range Files() {
		for _, e := range Parse(f, docs[f]) {
			if prev, ok := seen[e.Key]; ok {
				t.Errorf("%s: key %q is also in %s", f, e.Key, prev)
			}
			seen[e.Key] = f
			if strings.TrimSpace(e.Markdown) == "" {
				t.Errorf("%s: key %q has nothing in it", f, e.Key)
			}
		}
	}
}

// The fast path gives what the parser gives, for every entry it takes, and
// takes most of them.
func TestThePlainFastPathAgreesWithTheParser(t *testing.T) {
	took, all := 0, 0
	for _, f := range Files() {
		for _, e := range Parse(f, docs[f]) {
			all++
			s, ok := plainFast(e.Markdown)
			if !ok {
				continue
			}
			took++
			slow := tagBreak.Replace(Render(e.Markdown))
			slow = strings.TrimSpace(listGap.ReplaceAllString(blanks.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(slow, "")), "\n\n"), "\n• "))
			if s != slow {
				t.Errorf("%s: fast %q\n parser %q", e.Key, s, slow)
			}
		}
	}
	if took < all*9/10 {
		t.Errorf("the fast path took %d of %d entries", took, all)
	}
}
