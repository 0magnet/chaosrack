//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// The page offers its rack to the server it came from (pkg/racklink), so a
// terminal can drive it with no browser debugging port. Started with
// window.rackctl, which is when the rack can first be driven; a page served
// by anything but a chaosrack server finds no link to make and does nothing.
// rackLinkStarted keeps it to one link a page.
var rackLinkStarted bool

func startRackLink() {
	if rackLinkStarted {
		return
	}
	rackLinkStarted = true
	// The picture script first: the link answers the picture's calls with it.
	js.Global().Call("eval", rackpic.Script)
	js.Global().Call("eval", racklink.PageScript)
	link := js.Global().Get("__racklink")
	if !link.Truthy() {
		return
	}
	// js.FuncOf, not dom.FuncOf: the link lives as long as the page, and a
	// panel rebuild must not release it.
	answer := js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 {
			return ""
		}
		return string(racklink.Answer(inPageRack{}, []byte(a[0].String())))
	})
	link.Call("start", answer)
}

// The picture as the script writes it, for the link to pass on as it is.

func (inPageRack) PictureJSON() string        { return picCall(rackpic.CallPicture) }
func (inPageRack) ChangesJSON(gen int) string { return picCall(rackpic.ChangesCall(gen)) }
func (inPageRack) ActJSON(gen int, x, y float64, kind string, delta float64) string {
	return picCall(rackpic.ActCall(gen, x, y, kind, delta))
}

func (inPageRack) SceneJSON(w, h int, shape float64) string {
	return picCall(rackpic.SceneCall(w, h, shape))
}

func (inPageRack) SceneActJSON(w, h int, shape, x, y float64, kind string, delta float64) string {
	return picCall(rackpic.SceneActCall(w, h, shape, x, y, kind, delta))
}

func (inPageRack) CanvasesJSON(gen int, want []rackpic.CanvasWant) string {
	return picCall(rackpic.CanvasesCall(gen, want))
}
