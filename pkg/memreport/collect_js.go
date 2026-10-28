//go:build js && wasm

package memreport

import (
	"encoding/json"
	"runtime"
	"syscall/js"
)

// pageSide reads what the page knows about itself in one crossing: walking
// twenty-five thousand elements from Go would cost more than the reading.
//
// inst is the page's top-level binding for the running instance (index.html,
// tinygo/index.html); the Go build exports its memory as mem, TinyGo as
// memory. A worker's fetches are not in the page's resource timing, so the
// meters worker's module is not listed. window.__crDiag is the page's counters
// (assets/diag.js), present when the page was served from this template.
const pageSide = `return (function(){
  var out = {modules: [], canvases: [], resources: []};
  try {
    var ex = (typeof inst !== 'undefined' && inst) ? inst.exports : null;
    var m = ex && (ex.mem || ex.memory);
    if (m) out.wasm_memory = m.buffer.byteLength;
  } catch (e) {}
  if (performance.memory) {
    out.js_heap_used = performance.memory.usedJSHeapSize;
    out.js_heap_total = performance.memory.totalJSHeapSize;
  }
  out.uptime_s = performance.now() / 1000;
  if (window.__crDiag) out.bridge = window.__crDiag.snapshot();
  out.elements = document.getElementsByTagName('*').length;
  document.querySelectorAll('.sect').forEach(function(s){
    var hdr = s.querySelector('.sect-hdr');
    if (!hdr) return;
    out.modules.push({name: hdr.textContent.trim().toLowerCase(),
                      elements: s.getElementsByTagName('*').length + 1,
                      hidden: s.offsetWidth === 0});
  });
  document.querySelectorAll('canvas').forEach(function(c){
    out.canvases.push({name: c.id || String(c.className) || (c.parentElement && c.parentElement.id) || 'canvas',
                       w: c.width, h: c.height});
  });
  performance.getEntriesByType('resource').forEach(function(r){
    var n = r.decodedBodySize || 0;
    if (n < (1 << 20)) return;
    var name = r.name.replace(/^[a-z]+:\/\/[^\/]+/, '');
    out.resources.push({name: name, bytes: n});
    if (/\.wasm$/.test(name) && n > (out.wasm_module || 0)) out.wasm_module = n;
  });
  return JSON.stringify(out);
})();`

var pageFn js.Value

// Collect reads the page's memory now.
func Collect() Report {
	var r Report
	if s := pageJSON(); s != "" {
		_ = json.Unmarshal([]byte(s), &r) //nolint:errcheck // a page that could not say leaves the fields zero
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	r.Compiler = runtime.Compiler
	r.Go = GoHeap{
		HeapAlloc:    ms.HeapAlloc,
		HeapInuse:    ms.HeapInuse,
		HeapIdle:     ms.HeapIdle,
		HeapReleased: ms.HeapReleased,
		HeapSys:      ms.HeapSys,
		HeapObjects:  ms.HeapObjects,
		Sys:          ms.Sys,
		TotalAlloc:   ms.TotalAlloc,
		NumGC:        ms.NumGC,
		PauseTotalNs: ms.PauseTotalNs,
	}
	return r
}

// JSON is Collect as JSON, for rackctl.mem().
func JSON() string {
	b, err := json.Marshal(Collect())
	if err != nil {
		return "{}"
	}
	return string(b)
}

// pageJSON runs pageSide, making it the first time. A page whose CSP
// forbids making functions from text gets the Go side alone.
func pageJSON() (s string) {
	defer func() {
		if recover() != nil {
			s = ""
		}
	}()
	if pageFn.IsUndefined() {
		pageFn = js.Global().Get("Function").New(pageSide)
	}
	return pageFn.Invoke().String()
}
