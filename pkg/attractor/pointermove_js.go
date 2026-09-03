package attractor

import "syscall/js"

// Pointer movement is the busiest event a browser delivers — a drag produces
// them faster than frames arrive — and under TinyGo every listener that
// receives one costs a scheduler task whether or not it does anything with it.
//
// Four separate document-level pointermove listeners used to sit in this
// package, one per draggable thing, and each opened by testing its own gesture
// flag and returning immediately if that gesture was not running. So moving
// the pointer anywhere on the page crossed into Go four times to decide four
// times that there was nothing to do. Profiling the TinyGo build put most of
// what it still allocated in the scheduler rather than in application code,
// and the browser reported a 234ms pointermove handler during a drag.
//
// They share one listener now, and it is on the document only while a gesture
// could be running: it goes on at pointerdown and comes off at pointerup. All
// four gestures begin with a pointerdown, so the listener is always present
// while one is in progress, and idle pointer movement crosses into Go not at
// all.
//
// If a pointerup is missed — released over the browser's own chrome, say — the
// listener stays attached and the handlers go on testing their flags, which is
// exactly the behavior this replaced. That is the right way for it to fail.
var (
	pointerMoveFns  []func(e js.Value)
	pointerMoveFunc js.Func
	pointerMoveOn   bool
)

// onPointerMove registers a document-level pointermove handler. The handler
// runs only while a gesture is in progress, so it still has to check whether
// the gesture is its own.
func onPointerMove(fn func(e js.Value)) {
	pointerMoveFns = append(pointerMoveFns, fn)
}

// initPointerMove installs the one shared listener and the pointerdown and
// pointerup that switch it on and off. Called once from Run.
func initPointerMove() {
	if !pointerMoveFunc.IsUndefined() {
		return
	}
	pointerMoveFunc = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
		if len(a) == 0 {
			return nil
		}
		e := a[0]
		for _, fn := range pointerMoveFns {
			fn(e)
		}
		return nil
	})
	// Capture phase, so the move listener is on the document before any
	// pointerdown handler that starts a gesture has run.
	doc.Call("addEventListener", "pointerdown", js.FuncOf(func(js.Value, []js.Value) interface{} {
		if !pointerMoveOn {
			doc.Call("addEventListener", "pointermove", pointerMoveFunc)
			pointerMoveOn = true
		}
		return nil
	}), true)
	off := js.FuncOf(func(js.Value, []js.Value) interface{} {
		if pointerMoveOn {
			doc.Call("removeEventListener", "pointermove", pointerMoveFunc)
			pointerMoveOn = false
		}
		return nil
	})
	doc.Call("addEventListener", "pointerup", off, true)
	doc.Call("addEventListener", "pointercancel", off, true)
}
