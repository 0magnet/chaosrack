package attractor

import (
	"strings"
	"testing"
)

// What is written down has to come back. This is a preference nobody can see
// any other way — there is no readout of "which modules are in the rack" — so
// a round trip that quietly lost the hidden set would look exactly like a
// rack nobody had rearranged.
func TestRackLayoutRoundTrips(t *testing.T) {
	in := rackLayout{
		Order:    []string{"console", "parameters", "gen x", "model out"},
		Hidden:   []string{"record", "style"},
		Switches: []string{"keys-on", "tm-on"},
	}
	got := decodeRackLayout(in.encode())
	for _, c := range []struct {
		what      string
		got, want []string
	}{
		{"order", got.Order, in.Order},
		{"hidden", got.Hidden, in.Hidden},
		{"switches", got.Switches, in.Switches},
	} {
		if strings.Join(c.got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s came back %v, want %v", c.what, c.got, c.want)
		}
	}
}

// An empty rack encodes and decodes to an empty rack, rather than to one
// module called "".
func TestRackLayoutEmpty(t *testing.T) {
	got := decodeRackLayout(rackLayout{}.encode())
	if len(got.Order)+len(got.Hidden)+len(got.Switches) != 0 {
		t.Errorf("an empty layout round-tripped to %+v", got)
	}
	// And so does a record that was never written.
	if got := decodeRackLayout(""); len(got.Order)+len(got.Hidden)+len(got.Switches) != 0 {
		t.Errorf("no record at all decoded to %+v", got)
	}
}

// A field a newer build wrote must not stop an older one reading the fields it
// does know. The alternative is a record that poisons every downgrade.
func TestRackLayoutIgnoresUnknownFields(t *testing.T) {
	got := decodeRackLayout("order=console,view;colors=blue;hidden=record;junk")
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
	l := rackLayout{Order: []string{"console", "gen x, y", "view;style", "a=b", "  ", "params"}}
	got := decodeRackLayout(l.encode())
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
			if got := strings.Join(mergeModuleOrder(c.saved, c.present), "|"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The list of switches the record carries, checked for the two ways it can
// silently go wrong: a duplicate (saved twice, restored twice) and the rack
// bay creeping in. The bay is the frame the modules sit in, not a module, and
// it has persisted under its own key through setRackBay since before this
// record existed; listing it here would give it two owners.
//
// That these ids are also in the permalink's table is checked in
// racklayout_js_test.go, where permaCtls is visible.
func TestConsoleModuleSwitchesAreDistinctAndNotTheBay(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range consoleModuleSwitches {
		if id == "handles-on" {
			t.Errorf("the rack bay is not a module; it persists through setRackBay")
		}
		if !strings.HasSuffix(id, "-on") {
			t.Errorf("%q does not look like a module switch id", id)
		}
		if seen[id] {
			t.Errorf("%q is listed twice, so it would be saved twice", id)
		}
		seen[id] = true
	}
	if len(seen) == 0 {
		t.Error("no console module switches are persisted at all")
	}
}

// Both new stores live under the same "wasmstuff-" prefix every other
// preference in this app uses — the dock edge, the interface size, the rack
// bay, the patch bank. The prefix is how they are found: it is what a host
// page embedding this panel would clear to reset it, and a key outside the
// family is one that survives that and then restores a rack nobody asked for.
func TestPersistenceKeysAreInTheFamily(t *testing.T) {
	for _, k := range []string{rackLayoutKey, presetStoreKey} {
		if !strings.HasPrefix(k, "wasmstuff-") {
			t.Errorf("localStorage key %q is outside the wasmstuff- family", k)
		}
	}
	if rackLayoutKey == presetStoreKey {
		t.Error("the layout and the presets would overwrite each other")
	}
}
