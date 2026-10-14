//go:build js && wasm

package attractor

// STL File mode ("stlfile"): render a user-supplied stereolithograph as
// a wireframe through the shared static-geometry pipeline. The Loader
// module's Load button opens a file picker; the chosen .stl (binary or
// ASCII) is parsed, centered, normalized to the pipeline's usual extent,
// and drawn as triangle edges — so gradient, spin, camera, and Model Out
// all apply like any other Geometry model. Before a file is loaded the
// mode shows the plain cube as a stand-in.
//
// pkg/stlview is the standalone single-model viewer this mode grew out
// of; here the mesh joins the full instrument instead.

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/stlmodels"
)

var (
	stlFileVerts []float32 // packed xyz per triangle corner, centered + normalized
	stlFileIdx   []uint16  // triangle edge pairs (LINES)
	stlFileTris  int       // triangles drawn (after any decimation)
)

func generateSTLFile() {
	if len(stlFileVerts) == 0 {
		// Nothing loaded yet — show the plain cube as a stand-in so the
		// screen isn't blank while the Load button waits.
		gpu.uploadBuffersIndexed(verticesCube[:72], indicesCube[:36], glctx.Types.Line)
		return
	}
	gpu.uploadBuffersIndexed(stlFileVerts, stlFileIdx, glctx.Types.Line)
}

// setSTLFileModel parses STL bytes into the mode's wireframe buffers
// (STLFileWire).
func setSTLFileModel(data []byte) error {
	w, err := STLFileWire(data)
	if err != nil {
		return err
	}
	stlFileVerts, stlFileIdx, stlFileTris = w.Verts, w.Idx, w.Tris
	return nil
}

// setSTLFileMesh takes a mesh straight from a generator (STLMeshWire).
func setSTLFileMesh(m meshstl.Mesh) error {
	w, err := STLMeshWire(m)
	if err != nil {
		return err
	}
	stlFileVerts, stlFileIdx, stlFileTris = w.Verts, w.Idx, w.Tris
	return nil
}

// stlFileSetLED writes the Loader readout (and its hover detail).
func stlFileSetLED(text, title string) {
	if led := dom.Doc.Call("getElementById", "stlfile-led"); led.Truthy() {
		setDotText(led, text)
		if title != "" {
			led.Set("title", title)
		}
	}
}

// buildSTLFileModule wires the Loader module: the Load button clicks a
// hidden <input type=file>; a chosen file is read, parsed, and swapped
// into the running mode with a camera refit. Called once from Run (via
// buildDemoModules), so the handlers live for the app's lifetime.
func buildSTLFileModule() {
	buildSTLBuiltInPicker()
	btn := dom.Doc.Call("getElementById", "stlfile-load")
	if !btn.Truthy() {
		return
	}
	input := dom.Doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".stl")
	input.Get("style").Set("display", "none")
	dom.Doc.Get("body").Call("appendChild", input)

	onParsed := dom.FuncOf(func(this js.Value, a []js.Value) any {
		u8 := js.Global().Get("Uint8Array").New(a[0])
		data := make([]byte, u8.Get("length").Int())
		js.CopyBytesToGo(data, u8)
		name := input.Get("value").String()
		if files := input.Get("files"); files.Truthy() && files.Get("length").Int() > 0 {
			name = files.Index(0).Get("name").String()
		}
		if err := setSTLFileModel(data); err != nil {
			stlFileSetLED("ERR", "Could not read "+name+": "+err.Error())
			return nil
		}
		// The readout: the base name, cut to what the display holds — the
		// tooltip keeps the full name.
		short := strings.TrimSuffix(strings.TrimSuffix(name, ".stl"), ".STL")
		if r := []rune(short); len(r) > dispFullChars {
			short = string(r[:dispFullChars])
		}
		detail := name + " — " + strconv.Itoa(stlFileTris) + " triangles"
		if stlFileTris >= stlmodels.MaxTris {
			detail += " (decimated to fit the 16-bit index pipeline)"
		}
		stlFileSetLED(short, "Loaded STL — "+detail)
		if run.selectedMode == "stlfile" {
			gpu.staticDirty = true
			generateForMode("stlfile")
			view.autoFitCamera()
		}
		input.Set("value", "")
		return nil
	})
	dom.On(input, "change", func(this js.Value, a []js.Value) any {
		files := input.Get("files")
		if !files.Truthy() || files.Get("length").Int() == 0 {
			return nil
		}
		files.Index(0).Call("arrayBuffer").Call("then", onParsed)
		return nil
	})
	dom.On(btn, "click", func(this js.Value, a []js.Value) any {
		input.Call("click")
		return nil
	})
}

// buildSTLBuiltInPicker fills the Loader module's built-in list and wires it.
//
// The models are generated here, in the browser, from pkg/attractor's own
// STLModels — the same list chaosrack stlgen writes to disk. Nothing is fetched and
// nothing is embedded: a hundred megabytes of STL would have to ship with the
// app otherwise, and the generators are a few kilobytes of code.
func buildSTLBuiltInPicker() {
	sel := dom.Doc.Call("getElementById", "stlfile-builtin")
	if !sel.Truthy() {
		return
	}
	group := js.Value{}
	lastGroup := ""
	for _, m := range STLModels() {
		if m.Group != lastGroup {
			group = dom.Doc.Call("createElement", "optgroup")
			group.Set("label", m.Group)
			sel.Call("appendChild", group)
			lastGroup = m.Group
		}
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", m.Name)
		opt.Set("textContent", m.Label)
		opt.Set("title", m.Description)
		group.Call("appendChild", opt)
	}

	// A control of the rack, so a rack without the page can pick a solid:
	// "" is the file loaded, the rest the built-ins by name.
	registerControl(ControlDesc{ID: "stlfile-builtin", Label: "solid", IsSelect: true, SelectDef: ""}, "visual · model")

	dom.On(sel, "change", func(this js.Value, a []js.Value) any {
		name := sel.Get("value").String()
		if name == "" {
			return nil
		}
		m, ok := STLModelByName(name)
		if !ok {
			stlFileSetLED("ERR", "No built-in named "+name)
			return nil
		}
		// Building an attractor tube is tens of thousands of triangles of
		// work; say what is happening before starting rather than freezing
		// the readout on the previous model's name.
		stlFileSetLED("BUILD", "Generating "+m.Label+"…")
		// Straight from the generator: see stlmodels.ViewerSeg for the detail, and
		// setSTLFileMesh for why there is no STL in between.
		if err := setSTLFileMesh(m.Build(stlmodels.ViewerSeg)); err != nil {
			stlFileSetLED("ERR", "Could not read "+m.Label+": "+err.Error())
			return nil
		}
		short := name
		if r := []rune(short); len(r) > dispFullChars {
			short = string(r[:dispFullChars])
		}
		detail := m.Label + " — " + strconv.Itoa(stlFileTris) + " triangles"
		if stlFileTris >= stlmodels.MaxTris {
			detail += " (decimated to fit the 16-bit index pipeline)"
		}
		stlFileSetLED(short, "Built-in — "+detail+". "+m.Description)
		if run.selectedMode == "stlfile" {
			gpu.staticDirty = true
			generateForMode("stlfile")
			view.autoFitCamera()
		}
		return nil
	})
}

// setSTLFileTris is stlWire into the mode's buffers.
func setSTLFileTris(n int, at func(int) [3]meshstl.V3) error {
	w, err := stlWire(n, at)
	if err != nil {
		return err
	}
	stlFileVerts, stlFileIdx, stlFileTris = w.Verts, w.Idx, w.Tris
	return nil
}
