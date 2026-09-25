package meters

// SlidingWindow is a window over the newest samples, costing what ARRIVES
// rather than what it holds.
//
// The analyzers each keep the most recent N samples and measure them on a
// timer — THD over 16384 samples every 400ms, wow and flutter over ten
// SECONDS of them every 500ms. Both kept that window by sliding it:
//
//	copy(buf, buf[n:])                 // shift the whole window down
//	copy(buf[len(buf)-n:], scratch)    // and put the new samples on the end
//
// which moves the entire window on every frame, for perhaps 800 new samples.
// Wow and flutter's window at 48 kHz is 480,000 float32 — 1.9 MB — so at
// sixty frames a second that is about 115 MB/s of memmove, to produce a
// reading twice a second. THD's is 64 KB, 3.9 MB/s.
//
// A ring costs what arrives. The samples are written at a cursor that wraps,
// and the window is put in order only when something is actually going to
// read it — which is on the analyzer's own timer, not on the frame.
type SlidingWindow struct {
	buf  []float32
	head int // where the next sample goes
	fill int // how many of buf are real, up to len(buf)
}

// Resize sets the window length, discarding what it held.
//
// Called when the source's sample rate changes, which is the one thing that
// changes how many samples ten seconds is. A window of nothing is legal and
// simply never fills.
func (w *SlidingWindow) Resize(n int) {
	if n < 0 {
		n = 0
	}
	if len(w.buf) == n {
		return
	}
	w.buf = make([]float32, n)
	w.head, w.fill = 0, 0
}

// Reset forgets what the window holds without giving up its memory.
//
// For when the SIGNAL changes rather than its length — switching the
// distortion analyzer from mix to left is a different waveform, and
// measuring across the join would report a transient nobody played.
func (w *SlidingWindow) Reset() { w.head, w.fill = 0, 0 }

// Len is the window length.
func (w *SlidingWindow) Len() int { return len(w.buf) }

// Full reports whether the window has as many real samples as it holds.
func (w *SlidingWindow) Full() bool { return len(w.buf) > 0 && w.fill >= len(w.buf) }

// Fill is how many real samples it has.
func (w *SlidingWindow) Fill() int { return w.fill }

// Push adds samples, keeping the newest.
//
// A push longer than the window keeps only its tail, which is the same thing
// the sliding version did and the only sensible reading of "the newest N".
func (w *SlidingWindow) Push(s []float32) {
	if len(w.buf) == 0 || len(s) == 0 {
		return
	}
	if len(s) >= len(w.buf) {
		copy(w.buf, s[len(s)-len(w.buf):])
		w.head, w.fill = 0, len(w.buf)
		return
	}
	n := copy(w.buf[w.head:], s)
	if n < len(s) {
		copy(w.buf, s[n:])
	}
	w.head = (w.head + len(s)) % len(w.buf)
	if w.fill += len(s); w.fill > len(w.buf) {
		w.fill = len(w.buf)
	}
}

// Linear writes the window into dst oldest-first and returns how many.
//
// This is the only O(window) operation, and it runs on the analyzer's timer
// rather than on the frame — which is the whole point. dst must be at least
// Fill() long; a shorter one takes the newest samples that fit, because a
// truncated window should lose its oldest end, not its newest.
func (w *SlidingWindow) Linear(dst []float32) int {
	n := min(w.fill, len(dst))
	if n == 0 {
		return 0
	}
	// The newest n samples end at head and run backwards, wrapping.
	start := ((w.head-n)%len(w.buf) + len(w.buf)) % len(w.buf)
	k := copy(dst[:n], w.buf[start:])
	if k < n {
		copy(dst[k:n], w.buf[:n-k])
	}
	return n
}
