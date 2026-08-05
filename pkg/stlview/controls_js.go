// DOM event handlers wired to the X/Y/Z/Zoom sliders + Stop button.
// Each handler reads its slider value, updates the renderer
// rotation/zoom state, and refreshes the textual readout next to the
// slider.
package stlview

import (
	"strconv"
	"syscall/js"
)

func stopApplication(_ js.Value, _ []js.Value) interface{} {
	running = false
	sZoomV.Set(ih, float32(0))
	currentZoom = float32(0)
	rr.SetZoom(float32(0))
	footer.Set(ih, originalHTML)

	if OnStop != nil {
		OnStop()
	}

	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, p []js.Value) interface{} {
		close(done)
		return nil
	}), 5000)
	return nil
}

func sCX(this js.Value, _ []js.Value) interface{} {
	sSpeed := this.Get("value").String()
	s, _ := strconv.ParseFloat(sSpeed, 64)
	rr.SetX(float32(s))
	if s > 0 {
		sXV.Set(ih, "+"+f64(s, 'f', 2, 32))
	}
	if s == 0 {
		sXV.Set(ih, "0"+f64(s, 'f', 2, 32))
	}
	if s < 0 {
		sXV.Set(ih, f64(s, 'f', 2, 32))
	}
	return nil
}

func sCY(this js.Value, _ []js.Value) interface{} {
	sS := this.Get("value").String()
	s, _ := strconv.ParseFloat(sS, 64)
	rr.SetY(float32(s))
	if s > 0 {
		sYV.Set(ih, "+"+f64(s, 'f', 2, 32))
	}
	if s == 0 {
		sYV.Set(ih, "0"+f64(s, 'f', 2, 32))
	}
	if s < 0 {
		sYV.Set(ih, f64(s, 'f', 2, 32))
	}
	return nil
}

func sCZ(this js.Value, _ []js.Value) interface{} {
	sS := this.Get("value").String()
	s, _ := strconv.ParseFloat(sS, 64)
	rr.SetZ(float32(s))
	if s > 0 {
		sZV.Set(ih, "+"+f64(s, 'f', 2, 32))
	}
	if s == 0 {
		sZV.Set(ih, "0"+f64(s, 'f', 2, 32))
	}
	if s < 0 {
		sZV.Set(ih, f64(s, 'f', 2, 32))
	}
	return nil
}

func sCZoom(this js.Value, _ []js.Value) interface{} {
	sS := this.Get("value").String()
	s, _ := strconv.ParseFloat(sS, 64)
	if s < 10 {
		sZoomV.Set(ih, "000"+f64(s, 'f', 2, 32))
	} else if s < 100 {
		sZoomV.Set(ih, "00"+f64(s, 'f', 2, 32))
	} else if s < 1000 {
		sZoomV.Set(ih, "0"+f64(s, 'f', 2, 32))
	} else {
		sZoomV.Set(ih, f64(s, 'f', 2, 32))
	}
	currentZoom = float32(s)
	rr.SetZoom(currentZoom)
	return nil
}
