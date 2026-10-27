// Package memreport says how much memory a running rack's page uses, and on
// what: the wasm heap the Go runtime manages, the module the browser keeps
// compiled, the page's JavaScript heap, its elements and its canvases.
//
// The page collects it (collect_js.go, behind rackctl.mem()) and anything
// holding a cable to the page prints it (`chaosrack mem`). What it cannot see
// is the browser's own side: the tab's process footprint, the compositor's
// tiles and the GPU's copies of the canvases. Brave's task manager (Shift+Esc)
// or chrome://tracing's memory-infra dump show those.
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
