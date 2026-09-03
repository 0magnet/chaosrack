//go:build js && wasm

package rack

// Reordering: take hold of a module's header and it moves, settling into
// whatever gap the cursor is over.
//
// Which controls sit next to each other is most of what makes a panel workable,
// and it is not something the program that built the panel can know. A rack
// reflows, so there is nothing to place — a module dropped between two others
// simply becomes the module between them.
//
// The listener is on the container rather than on each header, because modules
// come and go: a host rebuilds them when its state changes, and a handler bound
// to an element that gets replaced stops working the moment it is.

import "syscall/js"

// Pointer capture is taken on the CONTAINER so that moves and the release keep
// arriving while the pointer is over something else — an iframe, a plugin,
// past the edge of the document. It is an improvement, not a dependency: the
// move and end listeners are on the window, which sees the events whether
// capture holds or not.
//
// It is not a dependency because it does not hold. Dragging a module means
// moving it in the DOM, a move is a remove and an insert, and disconnecting the
// pointer's original target releases the capture — on the container, even
// though the container itself never moves. So capture is re-taken after every
// reorder, and losing it is not treated as the end of the drag. Ending on
// lostpointercapture ends the drag on its first step.
//
// What is deliberately NOT here is the obvious alternative: end the drag when a
// move reports no buttons held, on the grounds that the release must have
// happened somewhere unseen. That is true of a real mouse and false of a
// synthesized one — a browser driven over the devtools protocol reports no
// buttons on nearly every move, so the guard makes the whole feature untestable
// while looking correct.
func (r *Rack) wireDrag() {
	r.root.Call("addEventListener", "pointerdown", r.track(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		e := args[0]
		target := e.Get("target")
		if !target.Truthy() || target.Get("closest").Type() != js.TypeFunction {
			return nil
		}
		hdr := target.Call("closest", "."+r.opts.HeaderClass)
		if !hdr.Truthy() {
			return nil
		}
		mod := hdr.Call("closest", "."+r.opts.ModuleClass)
		if !mod.Truthy() || !mod.Get("parentNode").Equal(r.root) {
			return nil
		}
		r.dragHeld, r.dragMoving = mod, true
		mod.Get("classList").Call("add", "rack-dragging")
		if id := e.Get("pointerId"); id.Truthy() || id.Type() == js.TypeNumber {
			r.dragPointer = id
			if r.root.Get("setPointerCapture").Type() == js.TypeFunction {
				r.root.Call("setPointerCapture", id)
			}
		}
		e.Call("preventDefault") // or the header's text gets selected instead
		return nil
	}))

	window.Call("addEventListener", "pointermove", r.track(func(_ js.Value, args []js.Value) interface{} {
		if !r.dragMoving || len(args) == 0 {
			return nil
		}
		e := args[0]
		x, y := e.Get("clientX").Float(), e.Get("clientY").Float()
		for _, other := range r.Modules() {
			if other.Equal(r.dragHeld) || isHidden(other) {
				continue
			}
			rect := other.Call("getBoundingClientRect")
			if x < rect.Get("left").Float() || x > rect.Get("right").Float() ||
				y < rect.Get("top").Float() || y > rect.Get("bottom").Float() {
				continue
			}
			// Past the middle of the module under the cursor and it goes after
			// it, before the middle and it goes in front — so a module can be
			// dropped into any gap, and two swap by being dragged over each
			// other.
			//
			// Which middle depends on how the two are arranged. A rack that
			// wraps into rows puts them side by side and the answer is the
			// horizontal midpoint; a rack one module wide — a column, or any
			// rack whose modules are wider than it is — stacks them, and there
			// the horizontal midpoint is the same for every module and decides
			// nothing. Worse than nothing: a drag straight up a column lands on
			// each target's exact horizontal center, which reads as "after",
			// which is where the dragged module already was, so nothing moves
			// and the rack looks like it does not reorder at all.
			//
			// The axis is taken from the two modules rather than from the
			// container's flex-direction, because a wrapping row that has run
			// out of width IS a column and does not say so.
			held := r.dragHeld.Call("getBoundingClientRect")
			dx := centerOf(rect, "left", "right") - centerOf(held, "left", "right")
			dy := centerOf(rect, "top", "bottom") - centerOf(held, "top", "bottom")
			var before bool
			if abs(dy) > abs(dx) {
				before = y < centerOf(rect, "top", "bottom")
			} else {
				before = x < centerOf(rect, "left", "right")
			}
			if before {
				r.root.Call("insertBefore", r.dragHeld, other)
			} else {
				r.root.Call("insertBefore", r.dragHeld, other.Get("nextSibling"))
			}
			r.recapture()
			break
		}
		return nil
	}))

	for _, ev := range []string{"pointerup", "pointercancel"} {
		window.Call("addEventListener", ev, r.track(func(_ js.Value, _ []js.Value) interface{} {
			r.endDrag()
			return nil
		}))
	}
}

func (r *Rack) endDrag() {
	if !r.dragMoving {
		return
	}
	r.dragMoving = false
	if r.dragPointer.Truthy() || r.dragPointer.Type() == js.TypeNumber {
		if r.root.Get("releasePointerCapture").Type() == js.TypeFunction &&
			r.root.Call("hasPointerCapture", r.dragPointer).Bool() {
			r.root.Call("releasePointerCapture", r.dragPointer)
		}
		r.dragPointer = js.Undefined()
	}
	if r.dragHeld.Truthy() {
		r.dragHeld.Get("classList").Call("remove", "rack-dragging")
	}
	r.dragHeld = js.Undefined()
	r.Quantize()
	if r.opts.OnReorder != nil {
		r.opts.OnReorder(r.Order())
	}
}

// Order is the module keys as they currently appear, left to right.
func (r *Rack) Order() []string {
	mods := r.Modules()
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		if k := r.Key(m); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// SetOrder rearranges the modules to match, for restoring a saved rack. Keys
// that name no module are skipped, and modules the order does not mention keep
// their relative position at the end — so an order saved before a module
// existed still works, and puts the newcomer last rather than dropping it.
func (r *Rack) SetOrder(order []string) *Rack {
	for _, key := range order {
		m := r.Module(key)
		if !m.Truthy() {
			continue
		}
		r.root.Call("appendChild", m)
	}
	// Everything not named has now been left in front of the named ones, in
	// its original relative order, which is the only arrangement that does not
	// invent a position for it.
	r.Quantize()
	return r
}

// recapture re-takes pointer capture after a reorder. Moving the dragged
// module disconnects the pointer's original target, which releases the capture
// the container was holding; without this every step of a drag runs uncaptured
// and a pointer that wanders off the rack stops steering it.
func (r *Rack) recapture() {
	if !r.dragMoving || r.root.Get("setPointerCapture").Type() != js.TypeFunction {
		return
	}
	if r.dragPointer.Type() != js.TypeNumber {
		return
	}
	if r.root.Call("hasPointerCapture", r.dragPointer).Bool() {
		return
	}
	r.root.Call("setPointerCapture", r.dragPointer)
}

// centerOf is the midpoint of a DOMRect along one axis.
func centerOf(rect js.Value, lo, hi string) float64 {
	return (rect.Get(lo).Float() + rect.Get(hi).Float()) / 2
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
