//go:build js && wasm

package dom

import (
	"syscall/js"
	"testing"
)

// The second arena has to actually recycle. A widget rebuilt on every mode
// change leaks one closure per listener per rebuild if it does not, which
// is the exact failure the panel arena was written for — silent, and only
// visible as a wasm heap that never comes back down.
func TestRebuildIntoRecyclesItsArena(t *testing.T) {
	savedAlt := altArena
	defer func() { altArena = savedAlt }()

	var arena []js.Func
	build := func() {
		for i := 0; i < 3; i++ {
			FuncOf(func(js.Value, []js.Value) interface{} { return nil })
		}
	}
	for pass := 1; pass <= 4; pass++ {
		RebuildInto(&arena, build)
		if len(arena) != 3 {
			t.Fatalf("pass %d left %d funcs in the arena, want 3", pass, len(arena))
		}
	}
	// Clean up the last pass's funcs, which nothing else will.
	RebuildInto(&arena, func() {})
	if len(arena) != 0 {
		t.Errorf("an empty rebuild left %d funcs", len(arena))
	}
}

// And it must not take the panel's funcs while it is doing it, or a dial
// rebuild would quietly adopt — and later free — the panel's listeners.
func TestRebuildIntoLeavesThePanelArenaAlone(t *testing.T) {
	savedAlt, savedCollect := altArena, panelCollect
	savedLen := len(panelFuncs)
	defer func() {
		altArena, panelCollect = savedAlt, savedCollect
		panelFuncs = panelFuncs[:savedLen]
	}()

	panelCollect = true
	var arena []js.Func
	RebuildInto(&arena, func() {
		FuncOf(func(js.Value, []js.Value) interface{} { return nil })
	})
	if len(panelFuncs) != savedLen {
		t.Errorf("the panel arena grew by %d during a dial rebuild",
			len(panelFuncs)-savedLen)
	}
	if len(arena) != 1 {
		t.Errorf("the dial arena took %d funcs, want 1", len(arena))
	}
	// And collection goes back to the panel afterwards.
	FuncOf(func(js.Value, []js.Value) interface{} { return nil })
	if len(panelFuncs) != savedLen+1 {
		t.Error("the panel arena did not resume collecting after the rebuild")
	}
	RebuildInto(&arena, func() {}) // free the one above
}
