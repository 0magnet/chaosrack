//go:build !js

package audioroute

import "testing"

// The stale-sink sweep unloads modules by id, which means a wrong match unloads
// something that belongs to another program. Prefix matching would do exactly
// that to anyone who has an "fvf_in2", so the comparison is on whole arguments.
func TestParseNullSinkModulesMatchesTheWholeArgument(t *testing.T) {
	const out = "6\tmodule-device-restore\t\t\n" +
		"31\tmodule-null-sink\tsink_name=fvf_in sink_properties=device.description=FVF_in\t\n" +
		"32\tmodule-null-sink\tsink_name=fvf_in2 sink_properties=device.description=Someone_Else\t\n" +
		"33\tmodule-null-sink\tsink_name=other\t\n"
	got := parseNullSinkModules(out, "fvf_in")
	if len(got) != 1 || got[0] != "31" {
		t.Errorf("parseNullSinkModules = %v, want [31] — fvf_in2 and other are not ours", got)
	}
}

// Two of them can be loaded if a run was killed and another started before this
// sweep existed; both have to go, or the second Start still stacks on a corpse.
func TestParseNullSinkModulesFindsEveryStaleCopy(t *testing.T) {
	const out = "31\tmodule-null-sink\tsink_name=fvf_in\t\n" +
		"37\tmodule-null-sink\tsink_name=fvf_in sink_properties=device.description=FVF_in\t\n"
	if got := parseNullSinkModules(out, "fvf_in"); len(got) != 2 {
		t.Errorf("parseNullSinkModules = %v, want both copies", got)
	}
}

func TestHasArg(t *testing.T) {
	const args = "sink_name=fvf_in sink_properties=device.description=FVF_in"
	for _, c := range []struct {
		key, value string
		want       bool
	}{
		{"sink_name", "fvf_in", true},
		{"sink_name", "fvf", false}, // not a prefix match
		{"sink_name", "fvf_in2", false},
		{"sink_properties", "device.description=FVF_in", true},
		{"format", "fvf_in", false}, // right value, wrong key
	} {
		if got := hasArg(args, c.key, c.value); got != c.want {
			t.Errorf("hasArg(%q, %q) = %v, want %v", c.key, c.value, got, c.want)
		}
	}
}

// The sink-input parse decides which stream gets moved to the speakers, so a
// block boundary it gets wrong is audio that loops back and howls.
func TestParseSinkInputs(t *testing.T) {
	const out = `Sink Input #12
	Driver: protocol-native.c
	Sink: 1
	Properties:
		application.name = "Brave"
Sink Input #13
	Sink: 0
	Properties:
		application.name = "VLC media player"
`
	got := parseSinkInputs(out)
	if len(got) != 2 {
		t.Fatalf("parsed %d sink inputs, want 2: %+v", len(got), got)
	}
	if got[0] != (sinkInput{index: "12", sink: "1", app: "Brave"}) {
		t.Errorf("first = %+v", got[0])
	}
	// The last block has no block after it, so it is only recorded if the
	// parser flushes at the end — the classic way to lose exactly one entry.
	if got[1] != (sinkInput{index: "13", sink: "0", app: "VLC media player"}) {
		t.Errorf("last = %+v", got[1])
	}
}

// A key line before any "Sink Input #" header has no block to belong to.
// Attributing it to a nil block is a panic, not a mis-parse.
func TestParseSinkInputsIgnoresKeysBeforeTheFirstBlock(t *testing.T) {
	const out = "	Sink: 3\n	application.name = \"stray\"\nSink Input #1\n	Sink: 0\n"
	got := parseSinkInputs(out)
	if len(got) != 1 || got[0].index != "1" || got[0].sink != "0" {
		t.Errorf("parseSinkInputs = %+v, want one block #1 on sink 0", got)
	}
}

func TestParseSinkIndex(t *testing.T) {
	const out = "0\talsa_output.pci-0000_00_1f.3.analog-stereo\tPipeWire\ts32le 2ch 48000Hz\tRUNNING\n" +
		"1\tfvf_in\tPipeWire\tfloat32le 2ch 48000Hz\tIDLE\n"
	if got := parseSinkIndex(out, "fvf_in"); got != "1" {
		t.Errorf("parseSinkIndex(fvf_in) = %q, want 1", got)
	}
	if got := parseSinkIndex(out, "nothing"); got != "" {
		t.Errorf("parseSinkIndex(nothing) = %q, want empty", got)
	}
}

func TestOptionsDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.SinkName != DefaultSinkName || o.OutApps != DefaultOutApps {
		t.Errorf("withDefaults gave %+v", o)
	}
	kept := Options{SinkName: "mine", OutApps: "vlc"}.withDefaults()
	if kept.SinkName != "mine" || kept.OutApps != "vlc" {
		t.Errorf("withDefaults overwrote what the caller set: %+v", kept)
	}
}
