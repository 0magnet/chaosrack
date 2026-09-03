//go:build js && wasm

package splitwasm

import (
	"strconv"
	"syscall/js"
)

// The control plane's standing heap.
//
// chaosrack's live set, measured with runtime.ReadMemStats on the TinyGo build:
// 18 MB across ~25,000 objects, oscillating between 17.9 and 19.5 MB, and
// pointer-dense — a UI made of strings, maps, closures and js.Value wrappers,
// not of flat buffers. That distinction matters more than the megabytes: the
// collector is conservative and marks word by word, so a word only costs when
// it looks like a heap pointer. Reproducing the density is what makes this a
// fair stand-in; reproducing only the size is not.
type cpObject struct {
	name  string
	kind  string
	ptrs  []*cpObject
	next  *cpObject
	extra map[string]string
}

var cpHeap []*cpObject

// buildControlHeap retains about mb megabytes over roughly objects
// allocations, pointer-dense.
func buildControlHeap(mb, objects int) {
	per := (mb << 20) / objects
	cpHeap = make([]*cpObject, 0, objects)
	var prev *cpObject
	for i := 0; i < objects; i++ {
		o := &cpObject{
			name: "control " + strconv.Itoa(i),
			kind: "knob",
			next: prev,
		}
		n := per / 4
		o.ptrs = make([]*cpObject, n)
		for j := 0; j < n && len(cpHeap) > 0; j++ {
			o.ptrs[j] = cpHeap[(i+j)%len(cpHeap)]
		}
		if i%64 == 0 {
			o.extra = map[string]string{"label": o.name, "group": "panel"}
		}
		cpHeap = append(cpHeap, o)
		prev = o
	}
}

type knobSpec struct {
	idx            int
	label          string
	min, max, step float64
	geom           bool // changing it means the renderer must rebuild geometry
}

var knobs = []knobSpec{
	{PRotY, "spin Y", -5, 5, 0.05, false},
	{PRotX, "spin X", -5, 5, 0.05, false},
	{PZoom, "zoom", -3, 3, 0.01, false},
	{PLat, "parallels", 2, 60, 1, true},
	{PLon, "meridians", 1, 90, 1, true},
	{PTwist, "twist", -3, 3, 0.01, true},
	{PR, "red", 0, 1, 0.01, false},
	{PG, "green", 0, 1, 0.01, false},
	{PB, "blue", 0, 1, 0.01, false},
}

// StartControl builds the panel and wires it to the shared parameter block.
//
// Everything here runs on events. Nothing in this module is called per frame,
// which is the entire point of the boundary: the renderer never asks the
// control plane for anything, so the control plane's heap is never walked on
// the renderer's account.
func StartControl(heapMB, heapObjects int) {
	if heapMB > 0 {
		buildControlHeap(heapMB, heapObjects)
	}

	shared := EnsureShared()
	doc := js.Global().Get("document")
	panel := doc.Call("createElement", "div")
	panel.Get("style").Set("cssText",
		"position:fixed;left:0;top:0;z-index:9;padding:10px 12px;"+
			"font:12px ui-monospace,monospace;color:#cfe;background:rgba(8,12,20,.72);"+
			"border-right:1px solid #1d2a3a;border-bottom:1px solid #1d2a3a;max-height:100vh;overflow:auto")

	for _, k := range knobs {
		row := doc.Call("createElement", "div")
		row.Get("style").Set("cssText", "margin:3px 0;display:flex;gap:8px;align-items:center")
		lab := doc.Call("createElement", "span")
		lab.Set("textContent", k.label)
		lab.Get("style").Set("cssText", "width:70px;display:inline-block;opacity:.8")
		inp := doc.Call("createElement", "input")
		inp.Set("type", "range")
		inp.Set("min", k.min)
		inp.Set("max", k.max)
		inp.Set("step", k.step)
		inp.Set("value", float64(Defaults[k.idx]))
		val := doc.Call("createElement", "span")
		val.Set("textContent", strconv.FormatFloat(float64(Defaults[k.idx]), 'f', 2, 64))
		val.Get("style").Set("cssText", "width:44px;text-align:right;opacity:.7")

		spec := k
		inp.Call("addEventListener", "input", js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
			f, err := strconv.ParseFloat(this.Get("value").String(), 64)
			if err != nil {
				return nil
			}
			// A knob WRITES. It does not call the renderer, and the renderer does
			// not call back to read it — the next frame picks the value up out of
			// the shared array on its own.
			shared.SetIndex(spec.idx, f)
			if spec.geom {
				shared.SetIndex(PGeomSeq, shared.Index(PGeomSeq).Float()+1)
			}
			val.Set("textContent", strconv.FormatFloat(f, 'f', 2, 64))
			return nil
		}))

		row.Call("appendChild", lab)
		row.Call("appendChild", inp)
		row.Call("appendChild", val)
		panel.Call("appendChild", row)
	}

	auto := doc.Call("createElement", "label")
	auto.Get("style").Set("cssText", "display:flex;gap:8px;align-items:center;margin-top:6px")
	box := doc.Call("createElement", "input")
	box.Set("type", "checkbox")
	box.Set("checked", Defaults[PAuto] >= 0.5)
	box.Call("addEventListener", "change", js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
		v := float64(0)
		if this.Get("checked").Bool() {
			v = 1
		}
		shared.SetIndex(PAuto, v)
		return nil
	}))
	txt := doc.Call("createElement", "span")
	txt.Set("textContent", "auto-rotate")
	auto.Call("appendChild", box)
	auto.Call("appendChild", txt)
	panel.Call("appendChild", auto)

	doc.Get("body").Call("appendChild", panel)
}
