//go:build js && wasm

package audiosrc

import (
	"syscall/js"
	"testing"
	"time"
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
	var none *jsQueue // a source whose stream never opened
	none.each(func(js.Value) { t.Fatal("a zero queue handed over an item") })
}

func TestJSQueueBytesOfEveryKindInOrder(t *testing.T) {
	q := newJSQueue("", 16)
	u8 := func(b ...byte) js.Value {
		a := js.Global().Get("Uint8Array").New(len(b))
		js.CopyBytesToJS(a, b)
		return a
	}
	q.push().Invoke(u8(1, 2, 3).Get("buffer"))                                   // an ArrayBuffer, as a WebSocket gives
	q.push().Invoke(u8(9, 4, 5, 9).Call("subarray", 1, 3))                       // a view at an offset, as a datagram may be
	q.push().Invoke("BgcI")                                                      // base64 text, the old WebSocket frames: 6 7 8
	q.push().Invoke(js.Global().Get("Object").New())                             // not bytes: dropped
	q.push().Invoke(js.Global().Get("Float32Array").New(js.ValueOf([]any{1.5}))) // a typed array of another kind
	var got [][]byte
	q.eachBytes(func(b []byte) { got = append(got, append([]byte(nil), b...)) })
	want := [][]byte{{1, 2, 3}, {4, 5}, {6, 7, 8}, {0, 0, 0xc0, 0x3f}}
	if len(got) != len(want) {
		t.Fatalf("took %v, want %v", got, want)
	}
	for i := range want {
		if string(got[i]) != string(want[i]) {
			t.Fatalf("item %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestJSQueueBytesBeyondOneStage(t *testing.T) {
	const items, size = 300, 1000 // 300 kB: more than one stage holds
	q := newJSQueue("", items)
	for i := range items {
		a := js.Global().Get("Uint8Array").New(size)
		a.Call("fill", i%251)
		q.push().Invoke(a)
	}
	n := 0
	q.eachBytes(func(b []byte) {
		if len(b) != size || int(b[0]) != n%251 || int(b[size-1]) != n%251 {
			t.Fatalf("item %d: %d bytes starting %d", n, len(b), b[0])
		}
		n++
	})
	if n != items {
		t.Fatalf("took %d items, want %d", n, items)
	}
}

func TestJSQueueAsksAgainAMillisecondAfterFindingNothing(t *testing.T) {
	q := newJSQueue("", 8)
	q.eachBytes(func([]byte) { t.Fatal("an empty queue handed over bytes") })
	q.push().Invoke(js.Global().Get("Uint8Array").New(2))
	n := 0
	q.eachBytes(func([]byte) { n++ }) // within the millisecond: not asked
	time.Sleep(2 * time.Millisecond)
	q.eachBytes(func([]byte) { n++ })
	if n != 1 {
		t.Fatalf("took %d items, want the one pushed, once the millisecond was over", n)
	}
	var none *jsQueue
	none.eachBytes(func([]byte) { t.Fatal("a zero queue handed over bytes") })
}
