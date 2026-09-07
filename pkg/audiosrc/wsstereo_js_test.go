//go:build js && wasm

package audiosrc

import "testing"

// The channels must come back as they were sent, not summed. This is the whole
// point of capturing two: a display that wants the difference between them
// cannot recover it from a mix.
func TestDeinterleaveSplitsChannels(t *testing.T) {
	// L ramps up, R ramps down, so a swap or a fold is visible in one look.
	src := []float32{0, 9, 1, 8, 2, 7, 3, 6}
	l, r, mono := deinterleave(src, MonoMix)

	wantL := []float32{0, 1, 2, 3}
	wantR := []float32{9, 8, 7, 6}
	for i := range wantL {
		if l[i] != wantL[i] {
			t.Errorf("l[%d] = %v, want %v", i, l[i], wantL[i])
		}
		if r[i] != wantR[i] {
			t.Errorf("r[%d] = %v, want %v", i, r[i], wantR[i])
		}
		if want := (wantL[i] + wantR[i]) * 0.5; mono[i] != want {
			t.Errorf("mono[%d] = %v, want the mix %v", i, mono[i], want)
		}
	}
}

func TestMonoModeSelectsAChannel(t *testing.T) {
	src := []float32{1, 5, 2, 6}
	for _, tc := range []struct {
		mode MonoMode
		want []float32
		name string
	}{
		{MonoMix, []float32{3, 4}, "mix"},
		{MonoLeft, []float32{1, 2}, "left"},
		{MonoRight, []float32{5, 6}, "right"},
	} {
		_, _, mono := deinterleave(src, tc.mode)
		for i := range tc.want {
			if mono[i] != tc.want[i] {
				t.Errorf("%s: mono[%d] = %v, want %v", tc.name, i, mono[i], tc.want[i])
			}
		}
		if got := tc.mode.String(); got != tc.name {
			t.Errorf("String() = %q, want %q", got, tc.name)
		}
	}
}

// A frame that ends mid-pair means the stream is not what it claims to be.
// Dropping the tail puts a click in nothing; inventing the missing sample would
// put one in every reader.
func TestDeinterleaveDropsAnOddTail(t *testing.T) {
	l, r, mono := deinterleave([]float32{1, 2, 3}, MonoMix)
	if len(l) != 1 || len(r) != 1 || len(mono) != 1 {
		t.Fatalf("lengths %d/%d/%d, want 1 each — the odd sample should be dropped", len(l), len(r), len(mono))
	}
	if l[0] != 1 || r[0] != 2 {
		t.Errorf("got (%v,%v), want (1,2)", l[0], r[0])
	}
}

func TestDeinterleaveEmpty(t *testing.T) {
	if l, r, m := deinterleave(nil, MonoMix); l != nil || r != nil || m != nil {
		t.Error("empty input should produce nothing rather than empty slices to append to")
	}
}
