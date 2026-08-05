// Public entry points + renderer setup. Run builds the X/Y/Z/Zoom
// slider HTML, wires DOM event handlers, creates the WebGL Renderer
// with the default sphere geometry, and starts the
// requestAnimationFrame loop; LoadSTL swaps the sphere for a parsed
// model from an async callback.
package stlview

import (
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// Package-level state shared across files. wasm runs in a single
// goroutine so these are accessed sequentially in practice (the only
// concurrency is the renderFrame callback + the stopApplication
// setTimeout, both safe by construction).
var (
	running                                                      = true
	done                                                         chan struct{}
	stlFileName, originalHTML                                    string
	rr                                                           Renderer
	existingFooter, body, footer, sXV, sYV, sZV, sZoomV, cEl, gl js.Value
	currentZoom                                                  float32 = 3
)

// OnStop, if set, is invoked when the user clicks the Stop Rendering
// button, after the slider controls are removed from the footer and
// before the render loop shuts down. Use it to restore host-page UI.
var OnStop func()

// Short aliases for the most-called DOM method names. Saves keystrokes
// and slightly shrinks the wasm.
const (
	gebi = "getElementById"
	ael  = "addEventListener"
	ih   = "innerHTML"
)

// Run starts the viewer on the canvas element with id "gocanvas" and
// blocks until the Stop button shuts the render loop down. Pass the
// model's file name (it tunes the zoom range and shaders for a real
// mesh) or "" to render the bare wireframe sphere. Call from the main
// goroutine; load the model afterwards from an async callback with
// LoadSTL.
func Run(stlName string) {
	stlFileName = stlName
	niam()
}

// LoadSTL parses STL data (binary or ASCII) and displays it, replacing
// whatever the running viewer shows. Safe to call from a js callback
// after Run has set the renderer up.
func LoadSTL(data []byte) error {
	solid, err := NewSTL(data)
	if err != nil {
		return err
	}
	vert, colors, indices := solid.GetModel()
	rr.SetZoom(getMaxScalar(vert) * 3)
	rr.SetModel(colors, vert, indices)
	return nil
}

func niam() {
	mgl32.DisableMemoryPooling()

	tdata := struct {
		XRange, ZMin, ZMax string
		XStep, ZoomStep    string
	}{
		XRange: "1", ZMin: "0", ZMax: "50",
		XStep: "0.01", ZoomStep: "0.1",
	}
	if stlFileName != ".stl" && stlFileName != "" {
		tdata.ZMin = "10"
		tdata.ZMax = "1000"
	}

	var controlsHTML = `
	<datalist id="speeds">
	<option>-` + tdata.XRange + `</option><option>0</option><option>` + tdata.XRange + `</option></datalist>
	<table class="🌐">
	<tr><td><p><button type="button" id="stop">Stop Rendering</button></p></td><td><p>X<input id="X" type="range" min="-` + tdata.XRange + `" max="` + tdata.XRange + `" step="` + tdata.XStep + `" list="speeds"><text id="XV">00.00</text></p></td>
	<td><p>Y<input id="Y" type="range" min="-` + tdata.XRange + `" max="` + tdata.XRange + `" step="` + tdata.XStep + `" list="speeds"><text id="YV">00.00</text></p></td>
	<td><p>Z<input id="Z" type="range" min="-` + tdata.XRange + `" max="` + tdata.XRange + `" step="` + tdata.XStep + `" list="speeds"><text id="ZV">00.00</text></p></td>
	<td><p>Zoom<input id="Zoom" type="range" min="` + tdata.ZMin + `" max="` + tdata.ZMax + `" step="` + tdata.ZoomStep + `" list="speeds"><text id="ZoomV">0000.00</text></p></td>
	</table>
	`
	doc := js.Global().Get("document")
	body = doc.Get("body")
	existingFooter = doc.Call("getElementsByTagName", "footer").Index(0)
	if existingFooter.Truthy() {
		originalHTML = existingFooter.Get(ih).String()
		footer = doc.Call("createElement", "footer")
		footer.Set(ih, originalHTML+controlsHTML)
		body.Call("replaceChild", footer, existingFooter)
	} else {
		footer = doc.Call("createElement", "footer")
		footer.Set(ih, controlsHTML)
		body.Call("appendChild", footer)
	}

	cEl = doc.Call(gebi, "gocanvas")
	width := doc.Get("body").Get("clientWidth").Int()
	height := doc.Get("body").Get("clientHeight").Int()

	cEl.Set("width", width)
	cEl.Set("height", height)
	sXc := js.FuncOf(sCX)
	sX := doc.Call(gebi, "X")
	sX.Call(ael, "input", sXc)
	sXV = doc.Call(gebi, "XV")

	sYc := js.FuncOf(sCY)
	sY := doc.Call(gebi, "Y")
	sY.Call(ael, "input", sYc)
	sYV = doc.Call(gebi, "YV")

	sZc := js.FuncOf(sCZ)
	sZ := doc.Call(gebi, "Z")
	sZ.Call(ael, "input", sZc)
	sZV = doc.Call(gebi, "ZV")

	sZoomc := js.FuncOf(sCZoom)
	sZoom := doc.Call(gebi, "Zoom")
	sZoom.Call(ael, "input", sZoomc)
	sZoomV = doc.Call(gebi, "ZoomV")

	sBc := js.FuncOf(stopApplication)
	sB := doc.Call(gebi, "stop")
	sB.Call(ael, "click", sBc)
	defer sBc.Release()

	gl = cEl.Call("getContext", "webgl")
	if gl.IsUndefined() {
		gl = cEl.Call("getContext", "experimental-webgl")
	}
	if gl.IsUndefined() {
		js.Global().Call("alert", "WASM:  browser might not support webgl")
		return
	}

	config := InitialConfig{
		W:        width,
		H:        height,
		X:        0,
		Y:        0,
		Z:        0,
		Vertices: verticesNative,
		Indices:  indicesNative,
		Colors:   colorsNative,
		FSC:      fragShaderCode,
		VSC:      vertShaderCode,
	}

	config.X = cryptoRandFloat32() / 20
	config.Y = cryptoRandFloat32() / 20
	config.Z = cryptoRandFloat32() / 20
	config.Vertices, config.Indices = generateSphereVertices(float32(1.0), 30, 30)
	if stlFileName == ".stl" || stlFileName == "" {
		config.FSC, config.VSC = fragShaderCode1, vertShaderCode1
	}
	var jsErr js.Value
	rr, jsErr = NewRenderer(gl, config)
	if !jsErr.IsNull() {
		js.Global().Call("alert", "WASM: Cannot load webgl ")
		return
	}
	rr.SetZoom(currentZoom)
	defer rr.Release()

	x, y, z := rr.GetSpeed()
	sX.Set("value", f32(x, 'f', -1, 64))
	if x > 0 {
		sXV.Set(ih, "+"+f32(x, 'f', 2, 64))
	}
	if x == 0 {
		sXV.Set(ih, " "+f32(x, 'f', 2, 64))
	}
	if x < 0 {
		sXV.Set(ih, f32(x, 'f', 2, 64))
	}
	sY.Set("value", f32(y, 'f', -1, 64))
	if y > 0 {
		sYV.Set(ih, "+"+f32(y, 'f', 2, 64))
	}
	if y == 0 {
		sYV.Set(ih, "0"+f32(y, 'f', 2, 64))
	}
	if y < 0 {
		sYV.Set(ih, f32(y, 'f', 2, 64))
	}
	sZ.Set("value", f32(z, 'f', -1, 64))
	if z > 0 {
		sZV.Set(ih, "+"+f32(z, 'f', 2, 64))
	}
	if z == 0 {
		sZV.Set(ih, "0"+f32(z, 'f', 2, 64))
	}
	if z < 0 {
		sZV.Set(ih, f32(z, 'f', 2, 64))
	}

	var renderFrame js.Func
	renderFrame = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		rr.Render(this, args)
		js.Global().Call("requestAnimationFrame", renderFrame)
		return nil
	})
	js.Global().Call("requestAnimationFrame", renderFrame)

	done = make(chan struct{})

	<-done
}
