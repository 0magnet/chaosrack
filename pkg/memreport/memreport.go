// Package memreport says how much memory a running rack's page uses, and on
// what: the wasm heap the Go runtime manages, the module the browser keeps
// compiled, the page's JavaScript heap, its elements and its canvases.
//
// The page collects it (collect_js.go, behind rackctl.mem()) and anything
// holding a cable to the page prints it (`chaosrack mem`). The page also says
// what its program does to the browser (Bridge): how often Go calls into
// JavaScript, how many JavaScript values Go holds, and the frames it missed.
//
// What the page cannot see is the browser's own side: the tab's process
// footprint, Blink's heap, the GPU's buffers and the compositor's tiles. A
// host attached over the debugging port asks the browser for those (Browser);
// it is the number Brave shows over the tab.
package memreport

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Report is one reading of a page's memory.
type Report struct {
	// Compiler is runtime.Compiler: "gc" for the Go build, "tinygo".
	Compiler string  `json:"compiler"`
	Uptime   float64 `json:"uptime_s"`

	// WasmMemory is the instance's linear memory. It only grows: memory the
	// Go heap frees is reused, never given back to the browser.
	WasmMemory int64 `json:"wasm_memory"`
	// WasmModule is the module's size as fetched. The browser keeps a copy of
	// these bytes for as long as the page lives, besides its compiled code.
	WasmModule int64 `json:"wasm_module"`

	Go GoHeap `json:"go"`

	// JSHeapUsed and JSHeapTotal are performance.memory's, which only
	// Chromium has; zero elsewhere.
	JSHeapUsed  int64 `json:"js_heap_used"`
	JSHeapTotal int64 `json:"js_heap_total"`

	Elements  int        `json:"elements"`
	Modules   []Module   `json:"modules"`
	Canvases  []Canvas   `json:"canvases"`
	Resources []Resource `json:"resources"`

	// Bridge is what the page counted of its program (assets/diag.js); nil
	// for a page without the counters.
	Bridge *Bridge `json:"bridge,omitempty"`
	// Browser is the browser's account of the tab's process. Only a host
	// attached over the debugging port fills it in.
	Browser *Browser `json:"browser,omitempty"`
	// BrowserErr is why the browser's account could not be had, when it
	// was asked for.
	BrowserErr string `json:"browser_error,omitempty"`
}

// GoHeap is the runtime's own account (runtime.MemStats), in bytes.
type GoHeap struct {
	HeapAlloc    uint64 `json:"heap_alloc"`
	HeapInuse    uint64 `json:"heap_inuse"`
	HeapIdle     uint64 `json:"heap_idle"`
	HeapReleased uint64 `json:"heap_released"`
	HeapSys      uint64 `json:"heap_sys"`
	HeapObjects  uint64 `json:"heap_objects"`
	Sys          uint64 `json:"sys"`
	TotalAlloc   uint64 `json:"total_alloc"`
	NumGC        uint32 `json:"num_gc"`
	// PauseTotalNs is how long collections have stopped the program in all.
	// TinyGo does not keep it.
	PauseTotalNs uint64 `json:"pause_total_ns"`
}

// Module is one of the rack's modules and how many elements it is made of.
type Module struct {
	Name     string `json:"name"`
	Elements int    `json:"elements"`
	Hidden   bool   `json:"hidden"`
}

// Canvas is one canvas in the page and the size of its backing store.
type Canvas struct {
	Name   string `json:"name"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
}

// Bytes is the canvas's backing store at four bytes a pixel. A WebGL
// canvas's drawing buffer costs more (depth, multisampling) and lives mostly
// on the GPU side.
func (c Canvas) Bytes() int64 { return int64(c.Width) * int64(c.Height) * 4 }

// Resource is a large file the page fetched.
type Resource struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// mb formats bytes as megabytes.
func mb[T int64 | uint64](n T) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

// Write prints the report for a person to read. top limits the module and
// canvas lists; 0 prints them all.
func (r Report) Write(w io.Writer, top int) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	build := r.Compiler
	if build == "gc" {
		build = "go"
	}
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(tw, format, a...) } //nolint:errcheck // checked at Flush
	p("build\t%s, up %s\n", build, uptime(r.Uptime))
	p("wasm memory\t%s\tlinear memory; grows, never shrinks\n", mb(r.WasmMemory))
	var runtimeOwn uint64
	if r.Go.Sys > r.Go.HeapSys {
		runtimeOwn = r.Go.Sys - r.Go.HeapSys
	}
	p("  go heap in use\t%s\t%d objects; %s idle, %s for the runtime itself\n",
		mb(r.Go.HeapInuse), r.Go.HeapObjects, mb(r.Go.HeapIdle), mb(runtimeOwn))
	p("  go heap live\t%s\tafter %d collections, %s allocated in all\n", mb(r.Go.HeapAlloc), r.Go.NumGC, mb(r.Go.TotalAlloc))
	if r.Go.PauseTotalNs > 0 {
		p("  gc pauses\t%.0f ms\tin all\n", float64(r.Go.PauseTotalNs)/1e6)
	}
	if r.WasmModule > 0 {
		p("wasm module\t%s\tkept by the browser besides its compiled code\n", mb(r.WasmModule))
	}
	if r.JSHeapTotal > 0 {
		p("js heap\t%s\tof %s\n", mb(r.JSHeapUsed), mb(r.JSHeapTotal))
	}
	var canvas int64
	for _, c := range r.Canvases {
		canvas += c.Bytes()
	}
	p("canvases\t%s\t%d in the page, at 4 bytes a pixel\n", mb(canvas), len(r.Canvases))
	p("elements\t%d\tin %d modules\n", r.Elements, len(r.Modules))
	if b := r.Bridge; b != nil {
		p("js calls\t%s\t%s\n", short(total(b.Calls)), joinCounts(b.Calls, 4))
		if h := b.Handles; h != nil {
			p("js handles\t%d held\t%s, in a table of %d\n", h.Live, joinCounts(h.Kinds, 4), h.Table)
		}
		if s := b.Stalls; s.Kind != "" {
			p("long frames\t%d\t%.0f ms in all, the worst %.0f ms (%s)\n", s.N, s.Ms, s.Worst, s.Kind)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	mods := append([]Module(nil), r.Modules...)
	sort.SliceStable(mods, func(i, j int) bool { return mods[i].Elements > mods[j].Elements })
	if top > 0 && len(mods) > top {
		mods = mods[:top]
	}
	if len(mods) > 0 {
		p("\nmodule\telements\n")
		for _, m := range mods {
			hidden := ""
			if m.Hidden {
				hidden = "\thidden"
			}
			p("%s\t%d%s\n", m.Name, m.Elements, hidden)
		}
	}
	cvs := append([]Canvas(nil), r.Canvases...)
	sort.SliceStable(cvs, func(i, j int) bool { return cvs[i].Bytes() > cvs[j].Bytes() })
	if top > 0 && len(cvs) > top {
		cvs = cvs[:top]
	}
	if len(cvs) > 0 {
		p("\ncanvas\tsize\tbacking store\n")
		for _, c := range cvs {
			p("%s\t%dx%d\t%s\n", c.Name, c.Width, c.Height, mb(c.Bytes()))
		}
	}
	if len(r.Resources) > 0 {
		p("\nfetched\tsize\n")
		for _, f := range r.Resources {
			p("%s\t%s\n", f.Name, mb(f.Bytes))
		}
	}
	if r.BrowserErr != "" {
		p("\nbrowser\t\tnot read: %s\n", r.BrowserErr)
	}
	if br := r.Browser; br != nil {
		p("\nbrowser, pid %d\t%s\tprivate footprint: what Brave shows over the tab; peak %s resident\n", br.Pid, mb(br.Footprint), mb(br.PeakRSS))
		if br.Pages > 1 {
			p("\t\tshared by %d pages, counted together\n", br.Pages)
		}
		parts := br.Parts
		if top > 0 && len(parts) > top {
			parts = parts[:top]
		}
		for _, c := range parts {
			p("  %s\t%s\t%s\n", c.Name, mb(c.N), partNotes[c.Name])
		}
	}
	if b := r.Bridge; b != nil && b.Detail {
		for _, l := range []struct {
			title string
			cs    []Count
		}{{"\njs calls by name\tcalls", b.Names}, {"\nblink objects made by\tcalls", b.Blink}} {
			cs := l.cs
			if top > 0 && len(cs) > top {
				cs = cs[:top]
			}
			if len(cs) > 0 {
				p("%s\n", l.title)
				for _, c := range cs {
					p("%s\t%s\n", c.Name, short(c.N))
				}
			}
		}
	}
	return tw.Flush()
}

// uptime is seconds as the largest units that say it.
func uptime(s float64) string {
	d := int64(s)
	var parts []string
	for _, u := range []struct {
		n    int64
		name string
	}{{86400, "d"}, {3600, "h"}, {60, "m"}} {
		if d >= u.n {
			parts = append(parts, fmt.Sprintf("%d%s", d/u.n, u.name))
			d %= u.n
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%ds", d)
	}
	return strings.Join(parts, " ")
}

// Bridge is what the page counts of its program's dealings with the browser.
type Bridge struct {
	// Detail is set when the page was opened with ?diag, which also counts
	// Names and Blink.
	Detail bool `json:"detail"`
	// Calls are the calls into JavaScript, by the bridge import that made
	// them (valueGet, valueCall, …), since the page started.
	Calls []Count `json:"calls"`
	// Names are the properties and methods those calls named ("get width",
	// "call drawImage"), with ?diag.
	Names []Count `json:"names,omitempty"`
	// Blink are the page's calls that make objects in Blink's heap (set
	// textContent, getBoundingClientRect, …), with ?diag.
	Blink   []Count  `json:"blink,omitempty"`
	Handles *Handles `json:"handles,omitempty"`
	Stalls  Stalls   `json:"stalls"`
}

// Count is how many times something happened.
type Count struct {
	Name string `json:"name"`
	N    int64  `json:"n"`
}

// Handles is wasm_exec's table of the JavaScript values Go holds. Each keeps
// its object alive, and a DOM object its Blink object, until Go collects the
// handle: a program that collects rarely keeps more of them.
type Handles struct {
	Live  int     `json:"live"`
	Table int     `json:"table"`
	Kinds []Count `json:"kinds"`
}

// Stalls are the frames the browser reports as long: long animation frames
// where it has them, else long tasks.
type Stalls struct {
	Kind  string  `json:"kind"`
	N     int64   `json:"n"`
	Ms    float64 `json:"ms"`
	Worst float64 `json:"worst"`
}

// Browser is the browser's memory dump of the tab's renderer process.
type Browser struct {
	Pid int `json:"pid"`
	// Footprint is the process's private footprint: what Brave's tab hover
	// card and task manager show.
	Footprint int64 `json:"private_footprint"`
	PeakRSS   int64 `json:"peak_rss"`
	// Pages is how many pages share the process; their memory is counted
	// together.
	Pages int `json:"pages"`
	// Parts are the dump's top-level allocators, largest first.
	Parts []Count `json:"parts"`
}

// partNotes say what a browser allocator holds, where it is worth saying.
var partNotes = map[string]string{
	"blink_gc":        "Blink's heap: DOM, styles, events, streams; garbage until Blink collects",
	"v8":              "the JavaScript heap",
	"malloc":          "C++ allocations, the wasm module's bytes among them",
	"partition_alloc": "ArrayBuffers, strings, layout",
	"webgl":           "WebGL contexts' buffers and textures, this side",
	"gpu":             "buffers for commands and uploads to the GPU, held in shared_memory",
	"cc":              "the compositor's tiles",
	"canvas":          "2D canvases",
	"shared_memory":   "shared with the GPU and browser processes; holds the gpu buffers",
}

// Rate is how fast a page changed between two reports.
type Rate struct {
	Seconds       float64
	AllocMBPerMin float64 // Go heap allocated
	GCPerMin      float64
	PauseMsPerMin float64
	CallsPerSec   float64 // into JavaScript
	StallsPerMin  float64
	StallMsPerMin float64
}

// Rates is how b moved on from a, an earlier report of the same page.
func Rates(a, b Report) Rate {
	r := Rate{Seconds: b.Uptime - a.Uptime}
	if r.Seconds <= 0 {
		return r
	}
	mins := r.Seconds / 60
	r.AllocMBPerMin = float64(b.Go.TotalAlloc-a.Go.TotalAlloc) / (1 << 20) / mins
	r.GCPerMin = float64(b.Go.NumGC-a.Go.NumGC) / mins
	r.PauseMsPerMin = float64(b.Go.PauseTotalNs-a.Go.PauseTotalNs) / 1e6 / mins
	if a.Bridge != nil && b.Bridge != nil {
		r.CallsPerSec = float64(total(b.Bridge.Calls)-total(a.Bridge.Calls)) / r.Seconds
		r.StallsPerMin = float64(b.Bridge.Stalls.N-a.Bridge.Stalls.N) / mins
		r.StallMsPerMin = (b.Bridge.Stalls.Ms - a.Bridge.Stalls.Ms) / mins
	}
	return r
}

func total(cs []Count) (n int64) {
	for _, c := range cs {
		n += c.N
	}
	return n
}

// short is a count in few characters: 950, 12.3k, 4.1M.
func short(n int64) string {
	switch {
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e4:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// joinCounts is the first n counts as "name 12.3k, other 950".
func joinCounts(cs []Count, n int) string {
	var parts []string
	for i, c := range cs {
		if i == n {
			break
		}
		parts = append(parts, c.Name+" "+short(c.N))
	}
	return strings.Join(parts, ", ")
}
