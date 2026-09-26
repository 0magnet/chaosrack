package racklayout

import (
	"strings"
	"testing"
)

// What is written down has to come back. This is a preference nobody can see
// any other way — there is no readout of "which modules are in the rack" — so
// a round trip that quietly lost the hidden set would look exactly like a
// rack nobody had rearranged.
func TestRackLayoutRoundTrips(t *testing.T) {
	in := Layout{
		Order:  []string{"console", "parameters", "gen x", "model out"},
		Hidden: []string{"record", "style"},
	}
	got := Decode(in.Encode())
	for _, c := range []struct {
		what      string
		got, want []string
	}{
		{"order", got.Order, in.Order},
		{"hidden", got.Hidden, in.Hidden},
	} {
		if strings.Join(c.got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s came back %v, want %v", c.what, c.got, c.want)
		}
	}
}

// An empty rack encodes and decodes to an empty rack, rather than to one
// module called "".
func TestRackLayoutEmpty(t *testing.T) {
	got := Decode(Layout{}.Encode())
	if len(got.Order)+len(got.Hidden) != 0 {
		t.Errorf("an empty layout round-tripped to %+v", got)
	}
	// And so does a record that was never written.
	if got := Decode(""); len(got.Order)+len(got.Hidden) != 0 {
		t.Errorf("no record at all decoded to %+v", got)
	}
}

// A field a newer build wrote must not stop an older one reading the fields it
// does know. The alternative is a record that poisons every downgrade.
func TestRackLayoutIgnoresUnknownFields(t *testing.T) {
	got := Decode("order=console,view;colors=blue;hidden=record;junk")
	if strings.Join(got.Order, ",") != "console,view" {
		t.Errorf("order %v", got.Order)
	}
	if strings.Join(got.Hidden, ",") != "record" {
		t.Errorf("hidden %v", got.Hidden)
	}
}

// A module key carrying a separator would write a record that read back as
// two modules, and the rack would spend every boot restoring one that does not
// exist. It is dropped instead.
func TestRackLayoutDropsSeparatorsInKeys(t *testing.T) {
	l := Layout{Order: []string{"console", "gen x, y", "view;style", "a=b", "  ", "params"}}
	got := Decode(l.Encode())
	if strings.Join(got.Order, "|") != "console|params" {
		t.Errorf("order came back %v, want just the two clean keys", got.Order)
	}
}

// The saved order is applied to the modules that are actually there.
func TestMergeModuleOrder(t *testing.T) {
	cases := []struct {
		name           string
		saved, present []string
		want           string
	}{
		{
			"saved order is honored",
			[]string{"view", "console", "colors"},
			[]string{"console", "colors", "view"},
			"view|console|colors",
		},
		{
			// The important one. rack-go's SetOrder appends every key it is
			// given, so a module left OUT of the list stays put and ends up in
			// front of the whole rack. A build that added a module would have
			// pushed the Console out of the first slot for everyone who had
			// ever dragged anything.
			"a module the record never heard of goes LAST",
			[]string{"console", "colors"},
			[]string{"console", "colors", "presets"},
			"console|colors|presets",
		},
		{
			"a module that no longer exists is skipped",
			[]string{"console", "gone", "colors"},
			[]string{"colors", "console"},
			"console|colors",
		},
		{
			"a duplicate in the record is placed once",
			[]string{"console", "console", "colors"},
			[]string{"colors", "console"},
			"console|colors",
		},
		{
			"no saved order leaves the rack as built",
			nil,
			[]string{"console", "colors", "view"},
			"console|colors|view",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(MergeModuleOrder(c.saved, c.present), "|"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A record from before the Console lost its module switches still decodes:
// the field it no longer writes is ignored, and the order survives.
func TestRackLayoutIgnoresTheRetiredSwitchesField(t *testing.T) {
	got := Decode("order=console,view;hidden=;switches=scope-on,tpl-on")
	if strings.Join(got.Order, "|") != "console|view" {
		t.Errorf("order came back %v", got.Order)
	}
}
