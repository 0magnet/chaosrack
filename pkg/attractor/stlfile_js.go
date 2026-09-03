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
	"bytes"
	"errors"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/meshstl"
	"gitlab.com/russoj88/stl/stl"
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
		uploadBuffersIndexed(verticesCube[:72], indicesCube[:36], glTypes.Line)
		return
	}
	uploadBuffersIndexed(stlFileVerts, stlFileIdx, glTypes.Line)
}

// setSTLFileModel parses STL bytes into the mode's wireframe buffers:
// bounding-box centered, normalized to half-extent 1.5 (the scale the
// built-in geometry models live at, so camera fitting behaves the same),
// decimated to the 16-bit index budget when oversized.
func setSTLFileModel(data []byte) error {
	solid, err := stl.From(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return setSTLFileTris(len(solid.Triangles), func(i int) [3]meshstl.V3 {
		v := solid.Triangles[i].Vertices
		return [3]meshstl.V3{
			{float64(v[0].X), float64(v[0].Y), float64(v[0].Z)},
			{float64(v[1].X), float64(v[1].Y), float64(v[1].Z)},
			{float64(v[2].X), float64(v[2].Y), float64(v[2].Z)},
		}
	})
}

// setSTLFileMesh takes a mesh straight from a generator, with no STL in
// between. The built-in models used to be encoded to bytes and parsed back by
// the same program that had just built them — a megabyte of round trip per
// selection to arrive at the triangles it already had.
func setSTLFileMesh(m meshstl.Mesh) error {
	return setSTLFileTris(len(m.Tris), func(i int) [3]meshstl.V3 {
		t := m.Tris[i]
		return [3]meshstl.V3{t.A, t.B, t.C}
	})
}

// setSTLFileTris is the shared half: center on the bounding box, normalize to
// the extent the built-in geometry lives at, decimate to the 16-bit index
// budget, and emit triangle edges. at(i) returns one triangle.
func setSTLFileTris(n int, at func(int) [3]meshstl.V3) error {
	if n == 0 {
		return errors.New("no triangles in file")
	}
	stride := (n + stlFileMaxTris - 1) / stlFileMaxTris

	first := true
	var minX, minY, minZ, maxX, maxY, maxZ float32
	for i := 0; i < n; i += stride {
		for _, v := range at(i) {
			x, y, z := float32(v[0]), float32(v[1]), float32(v[2])
			if first {
				minX, maxX, minY, maxY, minZ, maxZ = x, x, y, y, z, z
				first = false
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}
	}
	cx, cy, cz := (minX+maxX)/2, (minY+maxY)/2, (minZ+maxZ)/2
	half := (maxX - minX) / 2
	if h := (maxY - minY) / 2; h > half {
		half = h
	}
	if h := (maxZ - minZ) / 2; h > half {
		half = h
	}
	scale := float32(1)
	if half > 0 {
		scale = 1.5 / half
	}

	verts := make([]float32, 0, (n/stride+1)*9)
	idx := make([]uint16, 0, (n/stride+1)*6)
	var vi uint16
	for i := 0; i < n; i += stride {
		for _, v := range at(i) {
			verts = append(verts,
				(float32(v[0])-cx)*scale,
				(float32(v[1])-cy)*scale,
				(float32(v[2])-cz)*scale)
		}
		idx = append(idx, vi, vi+1, vi+1, vi+2, vi+2, vi)
		vi += 3
	}
	stlFileVerts, stlFileIdx = verts, idx
	stlFileTris = len(idx) / 6
	return nil
}

// stlFileSetLED writes the Loader readout (and its hover detail).
func stlFileSetLED(text, title string) {
	if led := doc.Call("getElementById", "stlfile-led"); led.Truthy() {
		led.Set("textContent", text)
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
	btn := doc.Call("getElementById", "stlfile-load")
	if !btn.Truthy() {
		return
	}
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".stl")
	input.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", input)

	onParsed := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
		// LED readout: base name only, uppercased (DSEG has no lowercase
		// worth reading), capped to what the cell fits — the tooltip keeps
		// the full name.
		short := strings.ToUpper(strings.TrimSuffix(strings.ToLower(name), ".stl"))
		if r := []rune(short); len(r) > 7 {
			short = string(r[:7])
		}
		detail := name + " — " + strconv.Itoa(stlFileTris) + " triangles"
		if stlFileTris >= stlFileMaxTris {
			detail += " (decimated to fit the 16-bit index pipeline)"
		}
		stlFileSetLED(short, "Loaded STL — "+detail)
		if selectedMode == "stlfile" {
			staticGeomDirty = true
			generateForMode("stlfile")
			autoFitCamera()
		}
		input.Set("value", "")
		return nil
	})
	input.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		files := input.Get("files")
		if !files.Truthy() || files.Get("length").Int() == 0 {
			return nil
		}
		files.Index(0).Call("arrayBuffer").Call("then", onParsed)
		return nil
	}))
	btn.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		input.Call("click")
		return nil
	}))
}

// syncSTLFileExtras shows the Loader module while STL File is the active
// model (the panel-rebuild hook, like the other mode-scoped modules).
func syncSTLFileExtras(mode string) {
	if sect := doc.Call("getElementById", "stlfile-module"); sect.Truthy() {
		if mode == "stlfile" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
}

// buildSTLBuiltInPicker fills the Loader module's built-in list and wires it.
//
// The models are generated here, in the browser, from pkg/attractor's own
// STLModels — the same list cmd/stlgen writes to disk. Nothing is fetched and
// nothing is embedded: a hundred megabytes of STL would have to ship with the
// app otherwise, and the generators are a few kilobytes of code.
func buildSTLBuiltInPicker() {
	sel := doc.Call("getElementById", "stlfile-builtin")
	if !sel.Truthy() {
		return
	}
	group := js.Value{}
	lastGroup := ""
	for _, m := range STLModels() {
		if m.Group != lastGroup {
			group = doc.Call("createElement", "optgroup")
			group.Set("label", m.Group)
			sel.Call("appendChild", group)
			lastGroup = m.Group
		}
		opt := doc.Call("createElement", "option")
		opt.Set("value", m.Name)
		opt.Set("textContent", m.Label)
		opt.Set("title", m.Description)
		group.Call("appendChild", opt)
	}

	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
		// Straight from the generator: see STLViewerSeg for the detail, and
		// setSTLFileMesh for why there is no STL in between.
		if err := setSTLFileMesh(m.Build(STLViewerSeg)); err != nil {
			stlFileSetLED("ERR", "Could not read "+m.Label+": "+err.Error())
			return nil
		}
		short := strings.ToUpper(name)
		if r := []rune(short); len(r) > 7 {
			short = string(r[:7])
		}
		detail := m.Label + " — " + strconv.Itoa(stlFileTris) + " triangles"
		if stlFileTris >= stlFileMaxTris {
			detail += " (decimated to fit the 16-bit index pipeline)"
		}
		stlFileSetLED(short, "Built-in — "+detail+". "+m.Description)
		if selectedMode == "stlfile" {
			staticGeomDirty = true
			generateForMode("stlfile")
			autoFitCamera()
		}
		return nil
	}))
}
