package attractor

import (
	"strconv"
	"strings"
	"testing"
)

func encodeDecode(ps []preset) []preset { return decodePresets(encodePresets(ps)) }

func TestPresetsRoundTrip(t *testing.T) {
	in := []preset{
		{"Blue Lorenz", "lorenz&cb=0000ff&z=12"},
		{"turtle 4242", "turtle&p.seed=4242"},
	}
	got := encodeDecode(in)
	if len(got) != len(in) {
		t.Fatalf("%d presets came back, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("preset %d came back %+v, want %+v", i, got[i], in[i])
		}
	}
}

// A preset with nothing in it is not a preset. Both halves have to be there
// or it is an entry nobody can recall and nobody can delete by name.
func TestPresetsDropIncompleteRecords(t *testing.T) {
	got := decodePresets(
		"good" + presetFieldSep + "lorenz" + presetRecSep +
			"noState" + presetFieldSep + presetRecSep +
			"noSeparatorAtAll" + presetRecSep +
			presetFieldSep + "lorenz&z=3")
	if len(got) != 1 || got[0].Name != "good" {
		t.Errorf("got %+v, want just the one complete record", got)
	}
}

// The name goes in a record whose fields are separated by control characters,
// so a name containing one would split the store. It is stripped, not escaped:
// there is no reason to want a tab in a preset name.
func TestCleanPresetName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Blue Lorenz  ", "Blue Lorenz"},
		{"a" + presetFieldSep + "b", "ab"},
		{"a" + presetRecSep + "b", "ab"},
		{"two\nlines", "twolines"},
		{"", ""},
		{"   ", ""},
		{strings.Repeat("x", presetNameMax+10), strings.Repeat("x", presetNameMax)},
	}
	for _, c := range cases {
		if got := cleanPresetName(c.in); got != c.want {
			t.Errorf("cleanPresetName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A name that is only over the limit because of the spaces on it is not
	// truncated into one that ends in a space.
	if got := cleanPresetName(strings.Repeat("y", presetNameMax) + "    "); got != strings.Repeat("y", presetNameMax) {
		t.Errorf("trailing space survived truncation: %q", got)
	}
}

// Saving over a name replaces that preset WHERE IT IS. Moving it to the end
// would reshuffle the list every time somebody re-tuned a view they already
// had, which is the opposite of what a named store is for.
func TestPutPresetReplacesInPlace(t *testing.T) {
	ps := []preset{{"a", "s1"}, {"b", "s2"}, {"c", "s3"}}
	got := putPreset(ps, "b", "s2new")
	if len(got) != 3 {
		t.Fatalf("%d presets after an overwrite, want 3", len(got))
	}
	if got[1].Name != "b" || got[1].State != "s2new" {
		t.Errorf("slot 1 is %+v, want b/s2new", got[1])
	}
	if got[0].State != "s1" || got[2].State != "s3" {
		t.Errorf("the neighbors moved: %+v", got)
	}
	// And the original is untouched — the store is read, modified and written
	// back as a whole, so an in-place edit of the caller's slice would be an
	// edit nothing asked for.
	if ps[1].State != "s2" {
		t.Errorf("putPreset scribbled on its input: %+v", ps)
	}
}

// The name is matched case-insensitively: it gets typed twice, once to save
// and once to save over, and two presets that look identical in the list is a
// way to lose work.
func TestPresetNameMatchIsCaseInsensitive(t *testing.T) {
	ps := putPreset([]preset{{"Blue Lorenz", "s1"}}, "blue lorenz", "s2")
	if len(ps) != 1 {
		t.Fatalf("%d presets, want the one overwritten", len(ps))
	}
	if ps[0].State != "s2" {
		t.Errorf("state %q, want the new one", ps[0].State)
	}
	if _, ok := findPreset(ps, "BLUE LORENZ"); !ok {
		t.Error("findPreset could not find it in another case")
	}
	if ps = deletePreset(ps, "BLUE lorenz"); len(ps) != 0 {
		t.Errorf("%d presets after deleting it, want 0", len(ps))
	}
}

// A save with nothing to file it under, or nothing to file, is refused rather
// than stored as an entry that can never be recalled.
func TestPutPresetRefusesEmpty(t *testing.T) {
	ps := []preset{{"a", "s1"}}
	if got := putPreset(ps, "   ", "state"); len(got) != 1 {
		t.Errorf("an empty name stored something: %+v", got)
	}
	if got := putPreset(ps, "name", ""); len(got) != 1 {
		t.Errorf("an empty state stored something: %+v", got)
	}
}

// At the cap the OLDEST goes, so the Save the user just pressed always takes.
func TestPutPresetCapDropsTheOldest(t *testing.T) {
	var ps []preset
	for i := 0; i < presetsMax+3; i++ {
		ps = putPreset(ps, "p"+strconv.Itoa(i), "s"+strconv.Itoa(i))
	}
	if len(ps) != presetsMax {
		t.Fatalf("%d presets, want the cap of %d", len(ps), presetsMax)
	}
	if ps[0].Name != "p3" {
		t.Errorf("oldest surviving is %q, want p3", ps[0].Name)
	}
	if last := ps[len(ps)-1].Name; last != "p"+strconv.Itoa(presetsMax+2) {
		t.Errorf("newest is %q, want the one just saved", last)
	}
}

func TestDeletePresetMissingIsANoOp(t *testing.T) {
	ps := []preset{{"a", "s1"}, {"b", "s2"}}
	if got := deletePreset(ps, "c"); len(got) != 2 {
		t.Errorf("deleting a preset that is not there changed the store: %+v", got)
	}
	got := deletePreset(ps, "a")
	if len(got) != 1 || got[0].Name != "b" {
		t.Errorf("after deleting a: %+v", got)
	}
}
