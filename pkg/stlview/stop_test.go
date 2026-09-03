//go:build js && wasm

package stlview

import (
	"syscall/js"
	"testing"
)

// Pressing stop a second time used to take the page down.
//
// Stopping takes five seconds by design, so the button appears to do nothing
// and the natural response is to press it again. That scheduled a second close
// of the same channel, and closing a closed channel panics — in wasm, a blank
// tab rather than a button that misbehaved. Later presses were worse: once Run
// has returned, the callback behind the listener has been released, and calling
// a released js.Func panics too.
//
// What is asserted here is the guard alone: once latched, the handler returns
// before it touches anything. The rest of it cannot run in a test — it drives
// the renderer, which needs a live WebGL context — and the guard is the whole
// of the fix.
func TestStopIsIgnoredOnceItHasStarted(t *testing.T) {
	old, oldDone := stopping, done
	t.Cleanup(func() { stopping, done = old, oldDone })

	stopping = true
	done = make(chan struct{})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a press after stopping had begun panicked: %v", r)
		}
	}()

	// Three more presses, the way an impatient person produces them. Each must
	// fall out at the guard: reaching the body would drive the renderer, and
	// reaching the end would close `done` a second time.
	for i := 0; i < 3; i++ {
		if got := stopApplication(js.Undefined(), nil); got != nil {
			t.Fatalf("press %d returned %v, so it did not stop at the guard", i+1, got)
		}
	}

	select {
	case <-done:
		t.Error("the channel was closed by a press that should have been ignored")
	default:
	}
}
