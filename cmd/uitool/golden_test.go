package main

import (
	"strings"
	"testing"
)

// The permalink is a mode name followed by key=value pairs. The mode is the
// one token without an equals sign, and only in first position — a bare token
// later on is a flag, not a second mode.
func TestParseHash(t *testing.T) {
	got := parseHash("#lorenz&a=1&b=2.5&flag")
	for k, want := range map[string]string{
		"_mode": "lorenz",
		"a":     "1",
		"b":     "2.5",
		"flag":  "",
	} {
		if got[k] != want {
			t.Errorf("%q = %q, want %q", k, got[k], want)
		}
	}
	if len(got) != 4 {
		t.Errorf("parsed %d keys, want 4: %v", len(got), got)
	}
}

func TestParseHashWithoutTheLeadingHash(t *testing.T) {
	withHash := parseHash("#lorenz&a=1")
	without := parseHash("lorenz&a=1")
	if len(withHash) != len(without) || withHash["_mode"] != without["_mode"] || withHash["a"] != without["a"] {
		t.Errorf("the leading hash changed the parse: %v vs %v", withHash, without)
	}
}

func TestParseHashOnAnEmptyPermalink(t *testing.T) {
	for _, in := range []string{"", "#"} {
		if got := parseHash(in); len(got) != 0 {
			t.Errorf("parseHash(%q) = %v, want nothing", in, got)
		}
	}
}

// A value containing an equals sign has to survive: only the first one splits.
func TestParseHashSplitsOnTheFirstEqualsOnly(t *testing.T) {
	got := parseHash("mode&expr=a=b=c")
	if got["expr"] != "a=b=c" {
		t.Errorf("expr = %q, want a=b=c", got["expr"])
	}
}

func TestParseHashSkipsEmptyTokens(t *testing.T) {
	got := parseHash("mode&&a=1&&")
	if got["a"] != "1" {
		t.Errorf("a = %q, want 1", got["a"])
	}
	if _, ok := got[""]; ok {
		t.Error("an empty token became a key")
	}
}

// A permalink that starts with a key=value has no bare mode, and inventing one
// would compare a run against the wrong reference.
func TestParseHashWithNoBareMode(t *testing.T) {
	got := parseHash("a=1&b=2")
	if _, ok := got["_mode"]; ok {
		t.Errorf("a permalink with no bare first token got the mode %q", got["_mode"])
	}
}

// ── valEquiv ─────────────────────────────────────────────────────────────────

func TestValEquivOnIdenticalStrings(t *testing.T) {
	for _, s := range []string{"", "abc", "1.5", "1,2,3"} {
		if !valEquiv(s, s) {
			t.Errorf("%q is not equivalent to itself", s)
		}
	}
}

// The point is that a permalink number that drifted in the last decimal is
// still the same value — floats round-tripped through a URL do that.
func TestValEquivToleratesSmallNumericDrift(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"1.0", "1.02"},
		{"0", "0.04"},
		{"1,2,3", "1.01,2.02,3.03"},
		{"-5", "-5.03"},
	} {
		if !valEquiv(tc.a, tc.b) {
			t.Errorf("%q and %q were treated as different", tc.a, tc.b)
		}
	}
}

// The tolerance is relative as well as absolute, so a big number may drift
// further than a small one — but not without limit.
func TestValEquivRejectsRealDifferences(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"1", "2"},
		{"0", "1"},
		{"1,2,3", "1,2,9"},
		{"100", "200"},
	} {
		if valEquiv(tc.a, tc.b) {
			t.Errorf("%q and %q were treated as the same", tc.a, tc.b)
		}
	}
}

// A different number of components is a different value whatever the numbers,
// or a truncated list compares equal to a full one.
func TestValEquivRejectsDifferentLengths(t *testing.T) {
	if valEquiv("1,2,3", "1,2") {
		t.Error("a three-component value matched a two-component one")
	}
	if valEquiv("1", "1,1") {
		t.Error("a scalar matched a pair")
	}
}

// Text that is not numeric falls back to exact comparison rather than being
// parsed as zero, which would make every two words equal.
func TestValEquivOnNonNumericText(t *testing.T) {
	if valEquiv("lorenz", "rossler") {
		t.Error("two different names compared equal")
	}
	if valEquiv("abc", "") {
		t.Error("a name compared equal to nothing")
	}
	if !valEquiv("lorenz", "lorenz") {
		t.Error("the same name compared unequal")
	}
	// One numeric and one not is not equal either.
	if valEquiv("1", "one") {
		t.Error("a number compared equal to a word")
	}
}

// ── hashDiff ─────────────────────────────────────────────────────────────────

// Two identical permalinks have nothing to report; anything printed here ends
// up in a failure message about a run that did not change.
func TestHashDiffOnIdenticalPermalinks(t *testing.T) {
	h := "lorenz&a=1&b=2,3"
	if got := hashDiff(h, h); got != "" {
		t.Errorf("identical permalinks reported %q", got)
	}
}

func TestHashDiffReportsAChangedValue(t *testing.T) {
	got := hashDiff("lorenz&a=1", "lorenz&a=9")
	if got == "" {
		t.Fatal("a changed value was not reported")
	}
	if !strings.Contains(got, "a") {
		t.Errorf("the difference does not name the key: %q", got)
	}
}

// A value that drifted within tolerance is not a difference, or every run
// fails on floating point noise.
func TestHashDiffIgnoresDriftWithinTolerance(t *testing.T) {
	if got := hashDiff("lorenz&a=1.0", "lorenz&a=1.02"); got != "" {
		t.Errorf("numeric drift was reported as a difference: %q", got)
	}
}

func TestHashDiffReportsAddedAndRemovedKeys(t *testing.T) {
	if got := hashDiff("lorenz&a=1", "lorenz&a=1&b=2"); got == "" {
		t.Error("an added key was not reported")
	}
	if got := hashDiff("lorenz&a=1&b=2", "lorenz&a=1"); got == "" {
		t.Error("a removed key was not reported")
	}
}

func TestHashDiffReportsAChangedMode(t *testing.T) {
	if got := hashDiff("lorenz&a=1", "rossler&a=1"); got == "" {
		t.Error("a changed mode was not reported")
	}
}

// ── small conversions ────────────────────────────────────────────────────────

// These read values out of the untyped map a CDP eval returns, where anything
// numeric arrives as a float64 and anything missing arrives as nil.
func TestValueConversionsFallBackRatherThanPanic(t *testing.T) {
	if got := toInt(float64(42)); got != 42 {
		t.Errorf("toInt(42.0) = %d", got)
	}
	if got := toInt(nil); got != 0 {
		t.Errorf("toInt(nil) = %d, want 0", got)
	}
	if got := toInt("not a number"); got != 0 {
		t.Errorf("toInt of a string = %d, want 0", got)
	}
	if got := toF(float64(1.5)); got != 1.5 {
		t.Errorf("toF(1.5) = %v", got)
	}
	if got := toF(nil); got != 0 {
		t.Errorf("toF(nil) = %v, want 0", got)
	}
	if got := str("hello"); got != "hello" {
		t.Errorf("str = %q", got)
	}
	if got := str(nil); got != "" {
		t.Errorf("str(nil) = %q, want empty", got)
	}
	if got := str(float64(1)); got != "" {
		t.Errorf("str of a number = %q, want empty", got)
	}
}

// A float that is not whole truncates rather than rounding, which is what the
// pixel measurements it reads expect.
func TestToIntTruncates(t *testing.T) {
	if got := toInt(float64(9.99)); got != 9 {
		t.Errorf("toInt(9.99) = %d, want 9", got)
	}
	if got := toInt(float64(-1.5)); got != -1 {
		t.Errorf("toInt(-1.5) = %d, want -1", got)
	}
}
