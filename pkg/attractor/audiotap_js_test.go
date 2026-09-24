//go:build js && wasm

package attractor

import "testing"

// resetTap puts the tap back to its zero state so each test starts clean.
func resetTap(t *testing.T) {
	t.Helper()
	tap.ringL = make([]float32, tapRingSize)
	tap.ringR = make([]float32, tapRingSize)
	tap.w = 0
	tap.scratch = make([]float32, 4096)
	tap.scratchR = make([]float32, 4096)
	tap.src = nil
}

// tapWrite pushes n samples carrying their own index as a value, so a reader
// can assert not just how many samples it got but WHICH ones. Both channels get
// the same value, so a fold of them is that value too and the existing
// assertions read unchanged.
func tapWrite(n int) {
	for i := 0; i < n; i++ {
		j := tap.w % len(tap.ringL)
		tap.ringL[j] = float32(tap.w)
		tap.ringR[j] = float32(tap.w)
		tap.w++
	}
}

// The defect this tap exists to fix: two consumers of one stream must each
// receive every sample. Draining the source directly gave the first caller
// everything and the second nothing, which is what froze the Takens embedding
// whenever the spectrogram backdrop was on.
func TestTapFansOutToEveryConsumer(t *testing.T) {
	resetTap(t)

	a, b := tapUnjoined, tapUnjoined
	dst := make([]float32, 512)

	// Both consumers join before any audio arrives.
	if got := tapRead(&a, dst); got != 0 {
		t.Fatalf("new cursor read %d samples from an empty tap, want 0", got)
	}
	if got := tapRead(&b, dst); got != 0 {
		t.Fatalf("new cursor read %d samples from an empty tap, want 0", got)
	}

	tapWrite(300)

	gotA := tapRead(&a, dst)
	if gotA != 300 {
		t.Fatalf("first consumer read %d, want 300", gotA)
	}
	firstA := dst[0]

	gotB := tapRead(&b, dst)
	if gotB != 300 {
		t.Fatalf("second consumer read %d, want 300 — the first must not consume the stream", gotB)
	}
	if dst[0] != firstA {
		t.Fatalf("second consumer saw %v as its first sample, first consumer saw %v", dst[0], firstA)
	}

	// A second round: each is caught up, so each sees only what is new.
	tapWrite(100)
	if got := tapRead(&a, dst); got != 100 {
		t.Fatalf("first consumer read %d on round two, want 100", got)
	}
	if got := tapRead(&b, dst); got != 100 {
		t.Fatalf("second consumer read %d on round two, want 100", got)
	}
	// Caught up: nothing new until more is written.
	if got := tapRead(&a, dst); got != 0 {
		t.Fatalf("caught-up consumer read %d, want 0", got)
	}
}

// The samples a consumer receives must be the ones actually written, in order.
func TestTapPreservesOrderAndValues(t *testing.T) {
	resetTap(t)

	c := tapUnjoined
	dst := make([]float32, 64)
	tapRead(&c, dst) // join

	tapWrite(64)
	got := tapRead(&c, dst)
	if got != 64 {
		t.Fatalf("read %d, want 64", got)
	}
	for i := 0; i < got; i++ {
		if want := float32(i); dst[i] != want {
			t.Fatalf("sample %d = %v, want %v", i, dst[i], want)
		}
	}
}

// A consumer that stops reading (its model was switched off) and falls further
// behind than the ring holds must be fast-forwarded rather than replaying stale
// audio or indexing behind the ring.
func TestTapFastForwardsAStaleCursor(t *testing.T) {
	resetTap(t)

	c := tapUnjoined
	dst := make([]float32, 1024)
	tapRead(&c, dst) // join at 0

	tapWrite(tapRingSize + 5000) // overrun the ring while the consumer sleeps

	got := tapRead(&c, dst)
	if got != len(dst) {
		t.Fatalf("stale cursor read %d, want %d", got, len(dst))
	}
	// It must resume at the oldest sample still present, not at 0.
	oldest := float32(tap.w - tapRingSize)
	if dst[0] != oldest {
		t.Fatalf("stale cursor resumed at %v, want the oldest retained sample %v", dst[0], oldest)
	}
}

// Switching source resets the write counter; cursors pointing past it are
// stale and must not read garbage or negative counts.
func TestTapSurvivesASourceSwitch(t *testing.T) {
	resetTap(t)

	c := tapUnjoined
	dst := make([]float32, 128)
	tapRead(&c, dst)
	tapWrite(500)
	if got := tapRead(&c, dst); got != 128 {
		t.Fatalf("read %d, want 128", got)
	}

	// Source switch: tapPump zeroes tapW while the cursor still points high.
	tap.w = 0
	if got := tapRead(&c, dst); got != 0 {
		t.Fatalf("cursor past the write head read %d, want 0", got)
	}
	if c != 0 {
		t.Fatalf("cursor = %d after a source switch, want it rebased to 0", c)
	}

	tapWrite(200)
	if got := tapRead(&c, dst); got != 128 {
		t.Fatalf("after the switch read %d, want 128", got)
	}
}

// Under FVF the tap must pull from the engine's processed output, and the
// switch has to invalidate the cursors — they index a different stream.
func TestTapSwitchesUpstreamForFVF(t *testing.T) {
	resetTap(t)
	tap.upstream = tapFromSource

	savedMode, savedActive := run.selectedMode, fvf.audioActive
	defer func() { run.selectedMode, fvf.audioActive = savedMode, savedActive }()

	run.selectedMode, fvf.audioActive = "lorenz", false
	if got := tapPumpUpstream(); got != tapFromSource {
		t.Fatalf("upstream = %v with FVF off, want tapFromSource", got)
	}

	// FVF selected but its audio engine not started: it is not draining the
	// source yet, so the tap must stay on the source.
	run.selectedMode, fvf.audioActive = "fvf", false
	if got := tapPumpUpstream(); got != tapFromSource {
		t.Fatalf("upstream = %v with the FVF engine stopped, want tapFromSource", got)
	}

	run.selectedMode, fvf.audioActive = "fvf", true
	if got := tapPumpUpstream(); got != tapFromFVF {
		t.Fatalf("upstream = %v with the FVF engine running, want tapFromFVF", got)
	}
}
