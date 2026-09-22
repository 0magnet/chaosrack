package meters

import "testing"

// The window holds the newest samples in order, which is the whole contract:
// the analyzers measure a waveform, and a waveform out of order is a
// different signal.
func TestTheWindowIsTheNewestSamplesInOrder(t *testing.T) {
	var w SlidingWindow
	w.Resize(5)
	for i := 1; i <= 8; i++ {
		w.Push([]float32{float32(i)})
	}
	got := make([]float32, 5)
	if n := w.Linear(got); n != 5 {
		t.Fatalf("got %d samples, want 5", n)
	}
	want := []float32{4, 5, 6, 7, 8}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("window is %v, want %v", got, want)
		}
	}
}

// Pushes that straddle the wrap must come back in order too — this is the
// case the ring exists for and the one a sliding copy could not get wrong.
func TestAWrappedWindowStillReadsInOrder(t *testing.T) {
	var w SlidingWindow
	w.Resize(6)
	w.Push([]float32{1, 2, 3, 4})
	w.Push([]float32{5, 6, 7}) // wraps
	got := make([]float32, 6)
	w.Linear(got)
	want := []float32{2, 3, 4, 5, 6, 7}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after a wrapping push the window is %v, want %v", got, want)
		}
	}
}

// A push longer than the window keeps its tail. The old sliding code did the
// same, and it is the only reading of "the newest N" that makes sense.
func TestAnOversizedPushKeepsItsTail(t *testing.T) {
	var w SlidingWindow
	w.Resize(3)
	w.Push([]float32{1, 2, 3, 4, 5, 6, 7})
	got := make([]float32, 3)
	w.Linear(got)
	want := []float32{5, 6, 7}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("window is %v, want %v", got, want)
		}
	}
	if !w.Full() {
		t.Error("a window filled by one big push does not report Full")
	}
}

// Before it fills, it reports only what it really has — an analyzer that
// measured the zeroes past the end would report silence it never heard.
func TestAPartialWindowReportsOnlyWhatArrived(t *testing.T) {
	var w SlidingWindow
	w.Resize(8)
	w.Push([]float32{1, 2, 3})
	if w.Full() {
		t.Error("three of eight samples reports Full")
	}
	if w.Fill() != 3 {
		t.Errorf("Fill is %d, want 3", w.Fill())
	}
	got := make([]float32, 8)
	if n := w.Linear(got); n != 3 {
		t.Fatalf("Linear returned %d, want 3", n)
	}
	for i, v := range []float32{1, 2, 3} {
		if got[i] != v {
			t.Fatalf("got %v, want the first three to be 1,2,3", got[:3])
		}
	}
}

// A short destination loses the OLDEST end, not the newest. An analyzer
// given less room than the window still wants the most recent audio.
func TestAShortDestinationKeepsTheNewest(t *testing.T) {
	var w SlidingWindow
	w.Resize(6)
	w.Push([]float32{1, 2, 3, 4, 5, 6})
	got := make([]float32, 2)
	if n := w.Linear(got); n != 2 {
		t.Fatalf("Linear returned %d, want 2", n)
	}
	if got[0] != 5 || got[1] != 6 {
		t.Errorf("got %v, want the newest two, 5 and 6", got)
	}
}

// Resizing drops what it held, because a window of a different length over
// the same ring is samples from two different sample rates in one buffer.
// Resizing to the SAME length must not drop anything, or a per-frame call
// that recomputes the length would empty it every frame.
func TestResizeDropsOnlyWhenTheLengthChanges(t *testing.T) {
	var w SlidingWindow
	w.Resize(4)
	w.Push([]float32{1, 2, 3, 4})
	w.Resize(4)
	if w.Fill() != 4 {
		t.Errorf("resizing to the same length emptied the window (fill %d)", w.Fill())
	}
	w.Resize(9)
	if w.Fill() != 0 {
		t.Errorf("a real resize kept %d samples", w.Fill())
	}
}

// Degenerate sizes must not panic: a zero-length window simply never fills,
// which is what a source with no sample rate yet gives.
func TestAZeroWindowIsHarmless(t *testing.T) {
	var w SlidingWindow
	w.Resize(0)
	w.Push([]float32{1, 2, 3})
	if w.Full() || w.Fill() != 0 {
		t.Error("a zero-length window claims to hold something")
	}
	if n := w.Linear(make([]float32, 4)); n != 0 {
		t.Errorf("Linear on a zero window returned %d", n)
	}
	w.Resize(-1)
	w.Push(nil)
}

// The point of the type: pushing costs what ARRIVED, not what the window
// holds. A run of small pushes across a large window must touch far fewer
// samples than the sliding copy it replaces would have.
func TestPushingCostsWhatArrivesNotWhatItHolds(t *testing.T) {
	const size, chunk = 480000, 800
	const pushes = size/chunk + 50 // enough to fill it and keep going
	var w SlidingWindow
	w.Resize(size)
	s := make([]float32, chunk)
	for i := 0; i < pushes; i++ {
		w.Push(s)
	}
	// The sliding version moved the whole window per push; the ring moves
	// one chunk. This is the invariant that makes the readout affordable,
	// stated as the ratio it has to beat.
	slid := size * pushes
	ring := chunk * pushes
	if ring*10 > slid {
		t.Errorf("the ring moves %d samples against the sliding %d — not the saving claimed", ring, slid)
	}
	if !w.Full() {
		t.Error("the window never filled")
	}
}

// Reset forgets the samples and keeps the memory. A channel change is a
// different signal, and measuring across the join reports a transient
// nobody played.
func TestResetForgetsTheSamplesAndKeepsTheRoom(t *testing.T) {
	var w SlidingWindow
	w.Resize(4)
	w.Push([]float32{1, 2, 3, 4})
	w.Reset()
	if w.Fill() != 0 || w.Full() {
		t.Errorf("after Reset the window still holds %d", w.Fill())
	}
	if w.Len() != 4 {
		t.Errorf("Reset gave up the memory: length is now %d", w.Len())
	}
	// And it fills again from scratch, in order.
	w.Push([]float32{7, 8})
	got := make([]float32, 4)
	if n := w.Linear(got); n != 2 || got[0] != 7 || got[1] != 8 {
		t.Errorf("after Reset the window refilled as %v (n=%d)", got, n)
	}
}
