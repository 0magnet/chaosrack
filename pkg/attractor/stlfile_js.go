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

	"gitlab.com/russoj88/stl/stl"
)

var (
	stlFileVerts []float32 // packed xyz per triangle corner, centered + normalized
	stlFileIdx   []uint16  // triangle edge pairs (LINES)
	stlFileLabel string    // LED readout: file name
	stlFileTris  int       // triangles drawn (after any decimation)
)

// 3 vertices per triangle; uint16 indices cap the pipeline at 65535
// vertices. Larger files are decimated by triangle stride to fit.
const stlFileMaxTris = 21845

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
	n := len(solid.Triangles)
	if n == 0 {
		return errors.New("no triangles in file")
	}
	stride := (n + stlFileMaxTris - 1) / stlFileMaxTris

	first := true
	var minX, minY, minZ, maxX, maxY, maxZ float32
	for i := 0; i < n; i += stride {
		for _, v := range solid.Triangles[i].Vertices {
			x, y, z := float32(v.X), float32(v.Y), float32(v.Z)
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
		for _, v := range solid.Triangles[i].Vertices {
			verts = append(verts,
				(float32(v.X)-cx)*scale,
				(float32(v.Y)-cy)*scale,
				(float32(v.Z)-cz)*scale)
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
		stlFileLabel = name
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
