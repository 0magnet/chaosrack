//go:build !js

package subcmds

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// renderArgs runs render with args, as the command line would.
func renderArgs(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRenderCmd()
	t.Cleanup(func() { newRenderCmd() }) // rebinding puts the flags back to their defaults
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	oldOut, oldErr := stdout, stderr
	stdout, stderr = &out, &out
	t.Cleanup(func() { stdout, stderr = oldOut, oldErr })
	cmd.SetArgs(args)
	return cmd.Execute()
}

// probe is ffprobe's answer for one stream of a file: its codec, frame count
// and duration.
func probe(t *testing.T, name, stream string) (codec string, frames int, secs float64) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-count_frames", "-select_streams", stream, //nolint:gosec // a test's own temporary file
		"-show_entries", "stream=codec_name,nb_read_frames,duration", "-of", "csv=p=0", name).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", name, err)
	}
	f := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(f) < 3 {
		t.Fatalf("ffprobe %s %s: %q", name, stream, out)
	}
	// WebM and Matroska keep the duration on the container, not the stream.
	if secs, err = strconv.ParseFloat(f[1], 64); err != nil && f[1] != "N/A" {
		t.Fatalf("ffprobe %s %s: duration %q", name, stream, f[1])
	}
	if frames, err = strconv.Atoi(f[2]); err != nil {
		t.Fatalf("ffprobe %s %s: frames %q", name, stream, f[2])
	}
	return f[0], frames, secs
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	for _, p := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip(p + " is not installed")
		}
	}
}

// TestAVideoOfASignalKeepsTimeWithIt: a video of an embedding has a frame for
// every 1/fps of the signal, at a rate no GIF can hold, and the signal as its
// sound, as long as the picture.
func TestAVideoOfASignalKeepsTimeWithIt(t *testing.T) {
	needFFmpeg(t)
	for _, engine := range []string{"rack", "figure"} {
		out := filepath.Join(t.TempDir(), "takens.mp4")
		if err := renderArgs(t, "--model", "takens", "--signal", "swp", "--audio", "1.5", "--fps", "60",
			"--width", "96", "--height", "96", "--engine", engine, "-o", out); err != nil {
			t.Fatalf("%s: %v", engine, err)
		}
		vc, vf, vs := probe(t, out, "v:0")
		ac, _, as := probe(t, out, "a:0")
		if vc != "h264" || ac != "aac" {
			t.Errorf("%s: streams %s and %s, want h264 and aac", engine, vc, ac)
		}
		if vf != 90 {
			t.Errorf("%s: %d frames, want 90 (1.5 s at 60 fps)", engine, vf)
		}
		// AAC pads to its own frames: within one of them, 1024 samples.
		if d := as - vs; d < -0.03 || d > 0.03 {
			t.Errorf("%s: sound %.3fs, picture %.3fs", engine, as, vs)
		}
	}
}

// TestAVideoTakesThePanelsControls: --set reaches every control the panel
// has, refusing one it does not and a value outside the knob's range.
func TestAVideoTakesThePanelsControls(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	ok := []string{"--model", "lorenz", "--frames", "6", "--fps", "30", "--width", "64", "--height", "64",
		"--set", "persist-trail=on", "--set", "phosphor=P31 green", "--set", "camera-zoom=20", "-o", filepath.Join(dir, "a.webm")}
	if err := renderArgs(t, ok...); err != nil {
		t.Fatal(err)
	}
	if c, n, _ := probe(t, filepath.Join(dir, "a.webm"), "v:0"); c != "vp9" || n != 6 {
		t.Errorf("a.webm: %s with %d frames, want vp9 with 6", c, n)
	}
	for _, bad := range [][]string{
		{"--set", "no-such-control=1"},
		{"--set", "camera-zoom=500"},
		{"--set", "phosphor=P999"},
		{"--turn", "0,1,0"}, // the figure renderer's
	} {
		args := append([]string{"--model", "lorenz", "--frames", "2", "-o", filepath.Join(dir, "b.mp4")}, bad...)
		if err := renderArgs(t, args...); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
}

// TestTheRacksGeneratorsAreHeard: with no signal given, an audio model on
// the rack draws what the rack's own generators make, and the video carries
// their sound.
func TestTheRacksGeneratorsAreHeard(t *testing.T) {
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "xy.mkv")
	if err := renderArgs(t, "--model", "xy", "--duration", "0.5", "--fps", "20", "--width", "64", "--height", "64",
		"--set", "gen-x-wave=sine", "--set", "gen-x-out=L", "--set", "gen-y-wave=sine", "--set", "gen-y-out=R", "-o", out); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := probe(t, out, "a:0"); c != "aac" {
		t.Errorf("no sound: %q", c)
	}
}
