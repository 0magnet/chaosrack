package subcmds

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/memreport"
)

func newMemCmd(d Dialer) *cobra.Command {
	var (
		asJSON bool
		top    int
		every  time.Duration
		count  int
	)
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "how much memory the rack's page uses, and on what",
		Long: `Print how much memory the running rack's page uses, and on what.

It lists:
  - the wasm heap: the Go runtime's own account of it, and the linear memory
    it lives in, which only grows;
  - the wasm module, which the browser keeps besides its compiled code;
  - the page's JavaScript heap;
  - its elements, per module;
  - its canvases;
  - what the program does to the browser: its calls into JavaScript, the
    JavaScript values it holds (each keeps its object alive until Go
    collects), and the frames the browser reports as long.

From a host, with --attach, it also asks the browser for the tab's process:
its private footprint, the number Brave shows over the tab, and what that is
made of — Blink's heap, the JavaScript heap, WebGL, the GPU's upload
buffers, the compositor's tiles.

A page opened with ?diag in its address also counts the calls by property
name and the DOM calls that make objects in Blink's heap.

--watch reads again every interval and prints a line each time, with rates
since the reading before: a single reading catches a heap that rises and
falls at either end.

  chaosrack mem
  chaosrack mem --top 0
  chaosrack mem --json
  chaosrack mem --watch 10s
  chaosrack mem --watch 30s --count 20`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := d.Dial()
			if err != nil {
				return err
			}
			if every > 0 {
				return watchMem(runCtx, c.Mem, every, count)
			}
			rep, err := c.Mem()
			if err != nil {
				return err
			}
			if asJSON {
				b, err := json.MarshalIndent(rep, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(stdout, string(b))
				return err
			}
			return rep.Write(stdout, top)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the reading as JSON")
	cmd.Flags().IntVar(&top, "top", 8, "list this many modules, canvases and browser parts, the largest first (0 for all)")
	cmd.Flags().DurationVar(&every, "watch", 0, "read again every interval, a line each time, with rates (e.g. 10s)")
	cmd.Flags().IntVar(&count, "count", 0, "with --watch: stop after this many readings (0: until interrupted)")
	if d.Flags != nil {
		d.Flags(cmd.Flags())
	}
	return cmd
}

// watchMem reads every interval and prints a line of each reading with its
// rates since the one before, then the footprint's low and high.
func watchMem(ctx context.Context, read func() (memreport.Report, error), every time.Duration, count int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(stdout, format, a...) } //nolint:errcheck // a terminal
	p("%-8s %9s %9s %8s %8s %10s %7s %8s %8s %10s\n",
		"time", "footprint", "blink", "wasm", "go live", "alloc/min", "gc/min", "calls/s", "handles", "long/min")
	var prev *memreport.Report
	var lo, hi int64
	for n := 0; count == 0 || n < count; n++ {
		if n > 0 {
			select {
			case <-ctx.Done():
				return summary(p, lo, hi)
			case <-time.After(every):
			}
		}
		rep, err := read()
		if err != nil {
			return err
		}
		foot, blink, handles := "-", "-", "-"
		if b := rep.Browser; b != nil {
			foot = fmt.Sprintf("%.0f MB", float64(b.Footprint)/(1<<20))
			for _, part := range b.Parts {
				if part.Name == "blink_gc" {
					blink = fmt.Sprintf("%.0f MB", float64(part.N)/(1<<20))
				}
			}
			if lo == 0 || b.Footprint < lo {
				lo = b.Footprint
			}
			hi = max(hi, b.Footprint)
		}
		if rep.Bridge != nil && rep.Bridge.Handles != nil {
			handles = fmt.Sprint(rep.Bridge.Handles.Live)
		}
		rate := "  (first reading)"
		if prev != nil {
			r := memreport.Rates(*prev, rep)
			rate = fmt.Sprintf(" %10.1f %7.1f %8.0f %8s %10.1f", r.AllocMBPerMin, r.GCPerMin, r.CallsPerSec, handles, r.StallsPerMin)
		}
		p("%-8s %9s %9s %5.0f MB %5.1f MB%s\n", time.Now().Format("15:04:05"), foot, blink,
			float64(rep.WasmMemory)/(1<<20), float64(rep.Go.HeapAlloc)/(1<<20), rate)
		prev = &rep
	}
	return summary(p, lo, hi)
}

// summary is the footprint's range over the watch, when the browser gave one.
func summary(p func(string, ...any), lo, hi int64) error {
	if hi > 0 {
		p("footprint ranged %.0f–%.0f MB\n", float64(lo)/(1<<20), float64(hi)/(1<<20))
	}
	return nil
}
