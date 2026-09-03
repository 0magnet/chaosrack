// DOM event handlers wired to the X/Y/Z/Zoom sliders + Stop button.
// Each handler reads its slider value, updates the renderer
// rotation/zoom state, and refreshes the textual readout next to the
// slider.
package stlview

import (
	"strconv"
	"syscall/js"
)

// stopping guards the stop button against being pressed twice.
//
// Stopping takes five seconds by design, so nothing visible happens when the
// button is clicked and the natural thing to do is click it again. That used to
// schedule a second close of the same channel, and closing a closed channel
// panics -- which in wasm is the whole page rather than a button that did
// nothing. Later clicks are worse still: once Run has returned, the callback
// behind this listener has been released, and calling a released js.Func panics
// as well. One flag closes both.
var stopping bool

func stopApplication(_ js.Value, _ []js.Value) interface{} {
	if stopping {
		return nil
	}
	stopping = true
	running = false
	sZoomV.Set(ih, float32(0))
	currentZoom = float32(0)
	rr.SetZoom(float32(0))
	footer.Set(ih, originalHTML)

	if OnStop != nil {
		OnStop()
	}

	var fin js.Func
	fin = js.FuncOf(func(js.Value, []js.Value) interface{} {
		close(done)
		fin.Release()
		return nil
	})
	js.Global().Call("setTimeout", fin, 5000)
	return nil
}

func sCX(this js.Value, _ []js.Value) interface{} {
	sSpeed := this.Get("value").String()
	s, _ := strconv.ParseFloat(sSpeed, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
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
	s, _ := strconv.ParseFloat(sS, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
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
	s, _ := strconv.ParseFloat(sS, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
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
	s, _ := strconv.ParseFloat(sS, 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
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
