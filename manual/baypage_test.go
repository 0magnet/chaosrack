package manual

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A control whose tooltip is its entry gets the entry, and its positions
// under it; one whose tooltip the panel has rewritten keeps the tooltip.
func TestEntryText(t *testing.T) {
	key := ""
	for _, k := range Keys() {
		if len(Positions(k)) > 1 && Text(k) != "" {
			key = k
			break
		}
	}
	if key == "" {
		t.Skip("no entry with positions")
	}
	got := EntryText(Text(key), []string{"nothing-here", key})
	if !strings.HasPrefix(got, Text(key)) || strings.Count(got, "\n• ") < len(Positions(key)) {
		t.Errorf("%s's page is %q", key, got)
	}
	if got := EntryText("rewritten since", []string{key}); !strings.HasPrefix(got, "rewritten since") {
		t.Errorf("a rewritten tooltip became %q", got)
	}
}

// A bay's page heads each module, gives each control by address, and refers
// a repeated tooltip back to where it was given.
func TestBayLines(t *testing.T) {
	lines := BayLines("Bay 9", []Module{{
		Name: "Thing", Range: "9.1.1–9.1.2",
		Cells: []Cell{{Loc: "9.1.1", Tip: "a knob"}, {Loc: "9.1.2", Tip: "a knob"}},
	}})
	var text []string
	for _, l := range lines {
		text = append(text, l.Text)
	}
	got := strings.Join(text, "|")
	if want := "Bay 9||Thing   9.1.1–9.1.2|9.1.1|a knob|9.1.2|As 9.1.1."; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// Wrapping keeps every line inside the width, a line's continuation under
// its indent, and the address on the first line only.
func TestWrap(t *testing.T) {
	long := strings.Repeat("word ", 30)
	out := Wrap([]Line{{Text: long, Indent: 3, Loc: "1.1.1"}, {Text: "• " + long}}, 30)
	for i, l := range out {
		if n := l.Indent + utf8.RuneCountInString(l.Text); n > 30 {
			t.Errorf("line %d is %d wide: %q", i, n, l.Text)
		}
		if l.Loc != "" && i != 0 {
			t.Errorf("line %d carries the address", i)
		}
	}
	for _, l := range out[1:] {
		if strings.HasPrefix(l.Text, "• ") {
			break
		}
		if l.Indent != 3 {
			t.Errorf("a continuation stands %d in", l.Indent)
		}
	}
	if last := out[len(out)-1]; last.Indent != 2 {
		t.Errorf("a bullet's continuation stands %d in, want under its text", last.Indent)
	}
}

func TestFirstSentence(t *testing.T) {
	for in, want := range map[string]string{
		"One. Two.":           "One.",
		"Solo — the one 1.5x": "Solo — the one 1.5x",
		"line one\nline two":  "line one",
	} {
		if got := FirstSentence(in); got != want {
			t.Errorf("FirstSentence(%q) = %q, want %q", in, got, want)
		}
	}
}
