//go:build !js

// Package audioroute inserts the temporary PulseAudio routing that lets the
// FVF wobbulator be heard out the speakers, and takes it away again.
//
// What it solves is a loop, not a preference. Capturing "what this machine is
// playing" and then playing the processed result back means the result is
// captured too, a buffer later, and wobbulated again. Breaking it needs the
// source app and the browser's own output on DIFFERENT sinks: a null sink is
// made the default, so every app lands in something silent that can be
// captured, and the browser's playback stream is moved back out to the real
// speakers, where nothing records it.
//
// It is a package because both servers want it and it is a fact about neither.
// It was cmd/audiows's private business for as long as cmd/audiows was the only
// thing that captured audio; the root server's --audio ended that, and leaving
// the routing behind would have meant the two halves of one feature could never
// be in one process -- stop one server, start another, reload the tab.
//
// Everything here is Linux/PulseAudio (or PipeWire-pulse) and shells out to
// pactl. Deliberately: the operations are load-module, set-default-sink and
// move-sink-input, the pulse client library this repo already uses exposes none
// of them, and a routing change an operator may have to undo by hand should be
// made with the command they would undo it with.
package audioroute

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Defaults for Options. The sink name is what shows up in pavucontrol and what
// a -source can be pointed at explicitly (fvf_in.monitor).
const (
	DefaultSinkName = "fvf_in"
	DefaultOutApps  = "brave,chrome,chromium,firefox,edge,vivaldi,opera"
)

// Options configure the routing. The zero value is usable.
type Options struct {
	// SinkName is the null sink to create and make default.
	SinkName string

	// OutApps is a comma-separated list of application-name substrings whose
	// playback is moved back off the null sink onto the real speakers -- i.e.
	// the browser running the page, whose Listen output is the thing that must
	// be heard rather than re-captured. The app being wobbulated is a different
	// app, so it stays on the null sink and keeps being captured.
	OutApps string
}

func (o Options) withDefaults() Options {
	if o.SinkName == "" {
		o.SinkName = DefaultSinkName
	}
	if o.OutApps == "" {
		o.OutApps = DefaultOutApps
	}
	return o
}

// Session is one installed routing. Stop restores what was there before and is
// safe to call more than once, from any goroutine.
type Session struct {
	opts     Options
	moduleID string
	prevSink string
	done     chan struct{}
	once     sync.Once
}

// SinkName is the null sink this session created; capture its monitor.
func (s *Session) SinkName() string { return s.opts.SinkName }

// PrevSink is the default sink that was in place before, and that Stop puts
// back.
func (s *Session) PrevSink() string { return s.prevSink }

// Available reports whether the routing can be installed at all -- i.e. whether
// there is a pactl to install it with. Callers use it to decide whether to
// OFFER the control, which is better than offering one that always fails.
func Available() bool {
	_, err := exec.LookPath("pactl")
	return err == nil
}

// Start installs the routing: a null sink, made default, with a watcher moving
// the browser's own output back to the real speakers.
//
// The capture source that goes with this is "monitor" -- the monitor of the
// default sink, which is now the null one. Callers that let a source be
// configured have to override it for the duration, or they will be recording
// the sink the audio no longer goes to.
func Start(o Options) (*Session, error) {
	o = o.withDefaults()
	if !Available() {
		return nil, errors.New("pactl is not on PATH: the FVF routing needs PulseAudio or PipeWire-pulse")
	}

	// A run that was killed rather than asked to stop leaves its null sink
	// loaded and default. Starting on top of that would stack a second one and
	// record the FIRST one as the thing to restore -- after which no Stop can
	// ever get the machine back to a real sink. Clear it before reading what
	// the default is.
	if n, err := Recover(o.SinkName); err != nil {
		return nil, err
	} else if n > 0 {
		log.Printf("audioroute: cleared %d stale %q sink(s) left by an earlier run", n, o.SinkName)
	}

	prevSink, err := pactl("get-default-sink")
	if err != nil {
		return nil, fmt.Errorf("get-default-sink: %w", err)
	}
	prevSink = strings.TrimSpace(prevSink)

	moduleID, err := pactl("load-module", "module-null-sink",
		"sink_name="+o.SinkName,
		"sink_properties=device.description=FVF_in")
	if err != nil {
		return nil, fmt.Errorf("load null sink %q: %w", o.SinkName, err)
	}
	moduleID = strings.TrimSpace(moduleID)

	if _, err := pactl("set-default-sink", o.SinkName); err != nil {
		_, _ = pactl("unload-module", moduleID) //nolint:errcheck // teardown on the way out; nothing is left to report a failure to
		return nil, fmt.Errorf("set-default-sink %s: %w", o.SinkName, err)
	}

	s := &Session{opts: o, moduleID: moduleID, prevSink: prevSink, done: make(chan struct{})}
	go s.watch(prevSink)

	log.Printf("audioroute: on -- all system audio now routes through %q (was %q)", o.SinkName, prevSink)
	log.Printf("audioroute:   the page's own Listen output is auto-routed back to %q (apps: %s)", prevSink, o.OutApps)
	return s, nil
}

// Stop restores the previous default sink and removes the null sink.
func (s *Session) Stop() {
	s.once.Do(func() {
		close(s.done)
		// Default first, module second: unloading a sink that is still the
		// default leaves PulseAudio to pick a replacement, and what it picks is
		// not necessarily what was there before.
		if s.prevSink != "" {
			_, _ = pactl("set-default-sink", s.prevSink) //nolint:errcheck // teardown on the way out; nothing is left to report a failure to
		}
		_, _ = pactl("unload-module", s.moduleID) //nolint:errcheck // teardown on the way out; nothing is left to report a failure to
		log.Printf("audioroute: off -- default sink restored to %q", s.prevSink)
	})
}

// Recover removes any null sink of this name left loaded by an earlier run, and
// returns how many it removed.
//
// It exists because Stop cannot run for a process that was killed: SIGKILL, an
// OOM, a pulled plug. What that leaves behind is a machine whose default sink is
// a black hole -- every app plays into silence -- and no obvious way to see why,
// because the sink looks like a real one in pavucontrol. Both entry points call
// it: Start, so a restart is a repair, and the operator's off switch, so "off"
// means off however the last run ended.
func Recover(sinkName string) (int, error) {
	if !Available() {
		return 0, nil
	}
	if sinkName == "" {
		sinkName = DefaultSinkName
	}
	out, err := pactl("list", "short", "modules")
	if err != nil {
		return 0, fmt.Errorf("list modules: %w", err)
	}
	ids := parseNullSinkModules(out, sinkName)
	if len(ids) == 0 {
		return 0, nil
	}
	for _, id := range ids {
		_, _ = pactl("unload-module", id) //nolint:errcheck // best-effort repair; the check below is what decides whether it worked
	}
	restoreSomeDefaultSink(sinkName)
	return len(ids), nil
}

// hasArg reports whether a pactl module argument string sets key to exactly
// value. Exactly, because "fvf_in" must not match a "fvf_in2" somebody else
// loaded: unloading another program's sink would be a worse bug than the one
// this is here to fix.
func hasArg(args, key, value string) bool {
	for _, f := range strings.Fields(args) {
		if f == key+"="+value {
			return true
		}
	}
	return false
}

// restoreSomeDefaultSink points the default at a real sink after a stale null
// one was unloaded. Which real sink is a guess -- the process that knew died
// without saying -- so it takes the first one, and any sink at all beats the one
// that was just removed.
func restoreSomeDefaultSink(removed string) {
	cur, err := pactl("get-default-sink")
	if err == nil {
		if c := strings.TrimSpace(cur); c != "" && c != removed {
			return // PulseAudio already picked something, and it is not the corpse
		}
	}
	out, err := pactl("list", "short", "sinks")
	if err != nil {
		return
	}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 && f[1] != removed {
			_, _ = pactl("set-default-sink", f[1]) //nolint:errcheck // best effort; there is nobody left to report to
			log.Printf("audioroute: default sink set to %q after clearing a stale %q", f[1], removed)
			return
		}
	}
}

// watch continuously moves any browser playback stream (the page's Listen
// output) off the null sink and onto the real speakers, so the wobbulated result
// is heard rather than fed back into the capture. Runs until Stop.
//
// A poll rather than a subscription because the stream appears when someone
// clicks Listen, which is not an event this process can be told about, and
// because `pactl subscribe` would be a second long-lived child process to
// supervise for a one-second reaction time.
func (s *Session) watch(target string) {
	var apps []string
	for _, a := range strings.Split(s.opts.OutApps, ",") {
		if a = strings.TrimSpace(strings.ToLower(a)); a != "" {
			apps = append(apps, a)
		}
	}
	if len(apps) == 0 || target == "" {
		return
	}
	logged := map[string]bool{}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
		}
		nullIdx := sinkIndexByName(s.opts.SinkName)
		if nullIdx == "" {
			continue
		}
		for _, si := range listSinkInputs() {
			if si.sink != nullIdx || !matchesAny(si.app, apps) {
				continue
			}
			if _, err := pactl("move-sink-input", si.index, target); err == nil && !logged[si.index] {
				log.Printf("audioroute: routed %q output (stream #%s) to %s (heard, not re-captured)", si.app, si.index, target)
				logged[si.index] = true
			}
		}
	}
}

func matchesAny(app string, subs []string) bool {
	al := strings.ToLower(app)
	for _, s := range subs {
		if strings.Contains(al, s) {
			return true
		}
	}
	return false
}

type sinkInput struct{ index, sink, app string }

// listSinkInputs parses `pactl list sink-inputs` into (index, sink index, app
// name) triples. Text parsing rather than the pulse client because the library
// exposes playback/record streams, not full sink-input introspection.
func listSinkInputs() []sinkInput {
	out, err := pactl("list", "sink-inputs")
	if err != nil {
		return nil
	}
	return parseSinkInputs(out)
}

// parseSinkInputs is the parsing half, split from the pactl call so it can be
// tested on a machine with no sound server. The bugs in this kind of code are
// all in the block handling -- a key before the first block, a block with no
// Sink: line, the last block -- and none of them need PulseAudio to reproduce.
func parseSinkInputs(out string) []sinkInput {
	var res []sinkInput
	var cur *sinkInput
	flush := func() {
		if cur != nil {
			res = append(res, *cur)
			cur = nil
		}
	}
	for _, ln := range strings.Split(out, "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "Sink Input #"):
			flush()
			cur = &sinkInput{index: strings.TrimPrefix(t, "Sink Input #")}
		case cur == nil:
			// between/before blocks
		case strings.HasPrefix(t, "Sink:"):
			cur.sink = strings.TrimSpace(strings.TrimPrefix(t, "Sink:"))
		case strings.HasPrefix(t, "application.name = "):
			cur.app = strings.Trim(strings.TrimPrefix(t, "application.name = "), "\"")
		}
	}
	flush()
	return res
}

// parseNullSinkModules returns the ids of every module-null-sink in `pactl list
// short modules` output that was loaded with exactly this sink_name.
func parseNullSinkModules(out, sinkName string) []string {
	var ids []string
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Split(ln, "\t")
		if len(f) < 3 || f[1] != "module-null-sink" || !hasArg(f[2], "sink_name", sinkName) {
			continue
		}
		ids = append(ids, f[0])
	}
	return ids
}

// sinkIndexByName returns the numeric index of a sink given its name.
func sinkIndexByName(name string) string {
	out, err := pactl("list", "short", "sinks")
	if err != nil {
		return ""
	}
	return parseSinkIndex(out, name)
}

// parseSinkIndex reads `pactl list short sinks` output for one sink's index.
func parseSinkIndex(out, name string) string {
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 && f[1] == name {
			return f[0]
		}
	}
	return ""
}

// pactl runs a pactl subcommand and returns its stdout.
func pactl(args ...string) (string, error) {
	out, err := exec.Command("pactl", args...).Output() //nolint:gosec // fixed binary name; the args are constants and configured sink/app names
	return string(out), err
}
