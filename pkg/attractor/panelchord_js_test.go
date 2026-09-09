//go:build js && wasm

package attractor

import "testing"

func TestParseChord(t *testing.T) {
	for _, tc := range []struct {
		spec string
		ok   bool
		want chord
	}{
		{"ctrl+alt+r", true, chord{ctrl: true, alt: true, key: "r"}},
		{"Ctrl+Alt+R", true, chord{ctrl: true, alt: true, key: "r"}},
		{" ctrl + alt + r ", true, chord{ctrl: true, alt: true, key: "r"}},
		{"shift+meta+K", true, chord{shift: true, meta: true, key: "k"}},
		{"cmd+k", true, chord{meta: true, key: "k"}},
		{"f2", true, chord{key: "f2"}},
		// All modifiers and no key would bind every Ctrl press, so it is
		// rejected outright rather than half-honored.
		{"", false, chord{}},
		{"ctrl+alt", false, chord{ctrl: true, alt: true}},
	} {
		got, ok := parseChord(tc.spec)
		if ok != tc.ok {
			t.Errorf("parseChord(%q) ok = %v, want %v", tc.spec, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseChord(%q) = %+v, want %+v", tc.spec, got, tc.want)
		}
	}
}

// A chord must not fire on a superset of itself. Ctrl+Shift+R is the browser's
// hard reload, and a page that swallowed it because it wanted Ctrl+R would be a
// worse bug than the one the chord fixes.
func TestChordMatchesExactModifiers(t *testing.T) {
	c, ok := parseChord("ctrl+alt+r")
	if !ok {
		t.Fatal("parseChord failed")
	}
	if !c.matches("r", true, true, false, false) {
		t.Error("exact chord did not match")
	}
	if !c.matches("R", true, true, false, false) {
		t.Error("key match should be case-insensitive")
	}
	for _, tc := range []struct {
		name                   string
		key                    string
		ctrl, alt, shift, meta bool
	}{
		{"extra shift", "r", true, true, true, false},
		{"extra meta", "r", true, true, false, true},
		{"missing alt", "r", true, false, false, false},
		{"missing ctrl", "r", false, true, false, false},
		{"no modifiers", "r", false, false, false, false},
		{"wrong key", "s", true, true, false, false},
	} {
		if c.matches(tc.key, tc.ctrl, tc.alt, tc.shift, tc.meta) {
			t.Errorf("%s: matched but should not have", tc.name)
		}
	}
}

// An unset chord leaves the host alone: initPanelRevealChord must not hide
// anything, which is what keeps every existing page unchanged.
func TestEmptyChordRevealsNothing(t *testing.T) {
	if _, ok := parseChord(PanelRevealChord); PanelRevealChord == "" && ok {
		t.Fatal("empty PanelRevealChord parsed as a usable chord")
	}
}
