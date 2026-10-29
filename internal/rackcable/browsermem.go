//go:build !js

package rackcable

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/cdp"
	"github.com/0magnet/chaosrack/pkg/memreport"
)

// browserMem asks the browser for a memory dump and reads the process that
// renders the page whose target id is target.
//
// Which process that is, the browser says in the same trace: the
// devtools.timeline category's TracingStartedInBrowser lists every frame with
// its process, and a page's top frame has its target's id. The dump itself is
// memory-infra's, what chrome://tracing records: per process, the private
// footprint (Brave's number for the tab) and the size of each allocator. A
// light dump: a detailed one breaks the same numbers down further for every
// process in the browser, and took 40 s and a 140 MB trace to read; a light
// one takes a few seconds.
func browserMem(c *cdp.Client, target string) (*memreport.Browser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ev, stop := c.Events(4096)
	defer stop()
	cfg := map[string]any{
		"traceConfig": map[string]any{
			"includedCategories": []string{"disabled-by-default-memory-infra", "disabled-by-default-devtools.timeline"},
			"memoryDumpConfig":   map[string]any{"triggers": []any{}},
		},
		"transferMode": "ReturnAsStream",
	}
	if err := c.Do(ctx, "Tracing.start", cfg, nil); err != nil {
		return nil, fmt.Errorf("starting a trace: %w", err)
	}
	var dumped struct {
		Success bool `json:"success"`
	}
	err := c.Do(ctx, "Tracing.requestMemoryDump", map[string]any{"levelOfDetail": "light"}, &dumped)
	if endErr := c.Do(ctx, "Tracing.end", nil, nil); err == nil {
		err = endErr
	}
	if err != nil {
		return nil, fmt.Errorf("dumping memory: %w", err)
	}
	if !dumped.Success {
		return nil, errors.New("the browser did not dump its memory")
	}
	stream, err := traceStream(ctx, ev)
	if err != nil {
		return nil, err
	}
	raw, err := readStream(ctx, c, stream)
	if err != nil {
		return nil, err
	}
	return parseDump(raw, target)
}

// traceStream waits for the trace to be handed over.
func traceStream(ctx context.Context, ev <-chan cdp.Event) (string, error) {
	for {
		select {
		case e := <-ev:
			if e.Method != "Tracing.tracingComplete" {
				continue
			}
			var p struct {
				Stream string `json:"stream"`
			}
			if err := json.Unmarshal(e.Params, &p); err != nil {
				return "", err
			}
			return p.Stream, nil
		case <-ctx.Done():
			return "", fmt.Errorf("waiting for the trace: %w", ctx.Err())
		}
	}
}

// readStream reads a trace stream to its end.
func readStream(ctx context.Context, c *cdp.Client, stream string) ([]byte, error) {
	var buf strings.Builder
	for {
		var chunk struct {
			Data          string `json:"data"`
			EOF           bool   `json:"eof"`
			Base64Encoded bool   `json:"base64Encoded"`
		}
		if err := c.Do(ctx, "IO.read", map[string]any{"handle": stream, "size": 4 << 20}, &chunk); err != nil {
			return nil, fmt.Errorf("reading the trace: %w", err)
		}
		if chunk.Base64Encoded {
			b, err := base64.StdEncoding.DecodeString(chunk.Data)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		} else {
			buf.WriteString(chunk.Data)
		}
		if chunk.EOF {
			break
		}
	}
	_ = c.Do(ctx, "IO.close", map[string]any{"handle": stream}, nil) //nolint:errcheck // the trace is read
	return []byte(buf.String()), nil
}

// traceEvent is the part of a trace event the dump is read from.
type traceEvent struct {
	Ph   string          `json:"ph"`
	Pid  int             `json:"pid"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// parseDump finds target's process in a trace and reads its dump.
func parseDump(raw []byte, target string) (*memreport.Browser, error) {
	var tr struct {
		TraceEvents []traceEvent `json:"traceEvents"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("reading the trace: %w", err)
	}
	pid, pages := 0, map[int]int{}
	var listed []int // every process the browser names for the page's frame
	for _, e := range tr.TraceEvents {
		if e.Name != "TracingStartedInBrowser" {
			continue
		}
		var a struct {
			Data struct {
				Frames []struct {
					Frame     string `json:"frame"`
					Parent    string `json:"parent"`
					ProcessID int    `json:"processId"`
				} `json:"frames"`
			} `json:"data"`
		}
		if err := json.Unmarshal(e.Args, &a); err != nil {
			continue
		}
		for _, f := range a.Data.Frames {
			if f.Parent != "" {
				continue
			}
			pages[f.ProcessID]++
			if strings.EqualFold(f.Frame, target) {
				listed = append(listed, f.ProcessID)
			}
		}
	}
	pid = drawing(tr.TraceEvents, target, listed)
	if pid == 0 {
		return nil, errors.New("the trace does not say which process renders the page")
	}
	// A process's dump comes in more than one event: its totals in one, its
	// allocators in others.
	b := &memreport.Browser{Pid: pid, Pages: pages[pid]}
	found := false
	for _, e := range tr.TraceEvents {
		if e.Ph == "v" && e.Pid == pid && readDump(e.Args, b) {
			found = true
		}
	}
	if !found || b.Footprint == 0 {
		return nil, fmt.Errorf("the trace has no memory dump of process %d", pid)
	}
	sort.Slice(b.Parts, func(i, j int) bool { return b.Parts[i].N > b.Parts[j].N })
	return b, nil
}

// readDump adds what one event of a process's dump holds to b: its totals,
// its top-level allocators, or both. It reports whether the event was a dump.
func readDump(args json.RawMessage, b *memreport.Browser) bool {
	var a struct {
		Dumps struct {
			Totals     map[string]json.RawMessage `json:"process_totals"`
			Allocators map[string]struct {
				Attrs map[string]struct {
					Value json.RawMessage `json:"value"`
				} `json:"attrs"`
			} `json:"allocators"`
		} `json:"dumps"`
	}
	if err := json.Unmarshal(args, &a); err != nil || (a.Dumps.Totals == nil && a.Dumps.Allocators == nil) {
		return false
	}
	// Sizes are hex strings; other values (a flag is a JSON bool) read as
	// nothing.
	hex := func(v json.RawMessage) int64 {
		var s string
		if json.Unmarshal(v, &s) != nil {
			return 0
		}
		n, _ := strconv.ParseInt(s, 16, 64) //nolint:errcheck // a value that is not hex reads as nothing
		return n
	}
	if t := a.Dumps.Totals; t != nil {
		b.Footprint = hex(t["private_footprint_bytes"])
		b.PeakRSS = hex(t["peak_resident_set_size"])
	}
	for name, al := range a.Dumps.Allocators {
		if name == "blink_gc/main/heap" {
			if v, ok := al.Attrs["allocated_objects_size"]; ok {
				b.BlinkObjects = hex(v.Value)
			}
		}
		if strings.Contains(name, "/") {
			continue
		}
		size, ok := al.Attrs["size"]
		if !ok {
			continue
		}
		if n := hex(size.Value); n >= 1<<20 {
			b.Parts = append(b.Parts, memreport.Count{Name: name, N: n})
		}
	}
	return true
}

// drawing picks, of the processes the browser names for a page's frame, the
// one that draws it. After the tab goes to another site the frame has a new
// process, and the old one can stay named while the back-forward cache keeps
// the page it had: reading that one reported a page that was not the tab's,
// with numbers that never moved. Only the live page emits timeline events
// tagged with the frame (a paint, a layout, an animation frame), so the
// process with the most of them is the one. With none in the trace, the last
// one named.
func drawing(events []traceEvent, target string, listed []int) int {
	if len(listed) == 0 {
		return 0
	}
	if len(listed) == 1 {
		return listed[0]
	}
	named := map[int]bool{}
	for _, p := range listed {
		named[p] = true
	}
	tagged := map[int]int{}
	for _, e := range events {
		if !named[e.Pid] || len(e.Args) == 0 {
			continue
		}
		var a struct {
			Data struct {
				Frame string `json:"frame"`
			} `json:"data"`
		}
		if json.Unmarshal(e.Args, &a) == nil && strings.EqualFold(a.Data.Frame, target) {
			tagged[e.Pid]++
		}
	}
	best := listed[len(listed)-1]
	for _, p := range listed {
		if tagged[p] > tagged[best] {
			best = p
		}
	}
	return best
}
