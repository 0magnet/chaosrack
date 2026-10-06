//go:build js && wasm

package audiosrc

import (
	"syscall/js"
	"testing"
)

func TestJSQueueKeepsTheFieldInOrderAndDropsTheOldest(t *testing.T) {
	q := newJSQueue("data", 3)
	push := q.push()
	for i := range 5 {
		ev := js.Global().Get("Object").New()
		ev.Set("data", i)
		push.Invoke(ev)
	}
	var got []int
	q.each(func(v js.Value) { got = append(got, v.Int()) })
	if len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 4 {
		t.Fatalf("took %v, want the newest three in order: [2 3 4]", got)
	}
	n := 0
	q.each(func(js.Value) { n++ })
	if n != 0 {
		t.Fatalf("took %d more after emptying the queue", n)
	}
}

func TestJSQueueWholeItemsAndTheZeroQueue(t *testing.T) {
	q := newJSQueue("", 8)
	q.push().Invoke("a")
	q.push().Invoke("b")
	var got string
	q.each(func(v js.Value) { got += v.String() })
	if got != "ab" {
		t.Fatalf("took %q, want \"ab\"", got)
	}
	var none jsQueue // a source whose stream never opened
	none.each(func(js.Value) { t.Fatal("a zero queue handed over an item") })
}
