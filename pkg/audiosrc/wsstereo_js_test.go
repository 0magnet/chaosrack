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

// The channel request is a query parameter on whichever endpoint is being
// opened, so it has to survive a URL that already carries one — ?wsurl= and
// ?wturl= are user-supplied and there is no reason they would not.
func TestWithChannelsAppendsToEitherShapeOfURL(t *testing.T) {
	for _, c := range []struct {
		url      string
		channels int
		want     string
	}{
		{"ws://h/ws", 2, "ws://h/ws?ch=2"},
		{"ws://h/ws?rate=48000", 2, "ws://h/ws?rate=48000&ch=2"},
		{"https://h:8080/wt", 2, "https://h:8080/wt?ch=2"},
		{"ws://h/ws", 1, "ws://h/ws"}, // one channel asks for nothing
		{"ws://h/ws", 0, "ws://h/ws"},
	} {
		if got := withChannels(c.url, c.channels); got != c.want {
			t.Errorf("withChannels(%q, %d) = %q, want %q", c.url, c.channels, got, c.want)
		}
	}
}

// THE TWO TRANSPORTS MUST AGREE. The WebSocket and WebTransport sources carry
// identical bytes and now share this retention, so what follows is what both of
// them do with a chunk — and a difference between them would show as the Stereo
// Embedding working on one transport and drawing the mono diagonal on the
// other, with nothing anywhere to say which one you are on.
func TestStereoRingsKeepBothChannelsAndTheFold(t *testing.T) {
	s := newStereoRings(64, 2)
	if !s.write([]float32{0, 9, 1, 8, 2, 7, 3, 6}) {
		t.Fatal("a full interleaved chunk was rejected")
	}
	if got := s.channels(); got != 2 {
		t.Errorf("channels() = %d, want 2", got)
	}
	l, r := make([]float32, 4), make([]float32, 4)
	s.latestStereo(l, r)
	for i, want := range []float32{0, 1, 2, 3} {
		if l[i] != want {
			t.Errorf("l[%d] = %v, want %v", i, l[i], want)
		}
	}
	for i, want := range []float32{9, 8, 7, 6} {
		if r[i] != want {
			t.Errorf("r[%d] = %v, want %v", i, r[i], want)
		}
	}
	// The single-signal readers get the fold, not the interleaving. Handing
	// them the raw stream would make every one of them read the two channels
	// as one signal at twice the rate — an octave up, and a spectrogram of a
	// sound nobody played.
	mono := make([]float32, 4)
	s.latest(mono)
	for i, want := range []float32{4.5, 4.5, 4.5, 4.5} {
		if mono[i] != want {
			t.Errorf("fold[%d] = %v, want the mix %v", i, mono[i], want)
		}
	}
	// And Drain delivers the fold once, not the interleaved stream.
	drained := make([]float32, 16)
	if n := s.drain(drained); n != 4 {
		t.Errorf("drain returned %d samples for a 4-pair chunk, want 4", n)
	}
	if n := s.drain(drained); n != 0 {
		t.Errorf("a second drain returned %d samples; each is delivered exactly once", n)
	}
}

// A one-channel stream must still answer TimeDomainStereo, because the Source
// contract says r gets a copy of l rather than zeros — and the Stereo
// Embedding relies on that to report a mono source instead of drawing nothing.
func TestStereoRingsMirrorAMonoStream(t *testing.T) {
	s := newStereoRings(64, 1)
	if !s.write([]float32{1, 2, 3, 4}) {
		t.Fatal("a mono chunk was rejected")
	}
	if got := s.channels(); got != 1 {
		t.Errorf("channels() = %d, want 1", got)
	}
	l, r := make([]float32, 4), make([]float32, 4)
	s.latestStereo(l, r)
	for i := range l {
		if l[i] != float32(i+1) || r[i] != l[i] {
			t.Errorf("sample %d: l=%v r=%v, want %v in both", i, l[i], r[i], i+1)
		}
	}
}

// A chunk with nothing in it must not mark the source ready. On the WebSocket
// that is a frame that decoded to nothing; on WebTransport it is a reassembled
// message too short to hold a pair. Either way the caller flips its ready flag
// on this answer, and saying yes to no samples is a source that reports itself
// live and hands back zeros.
func TestStereoRingsRejectChunksWithNothingInThem(t *testing.T) {
	for _, c := range []struct {
		name     string
		channels int
		in       []float32
	}{
		{"mono, empty", 1, nil},
		{"stereo, empty", 2, nil},
		{"stereo, half a pair", 2, []float32{1}},
	} {
		s := newStereoRings(64, c.channels)
		if s.write(c.in) {
			t.Errorf("%s: reported that samples landed", c.name)
		}
	}
}

// The fold follows the knob. It is applied as chunks arrive rather than on
// read, so what is already retained keeps the fold it was written with — which
// is the documented behavior and worth pinning, because the alternative
// reading (that the change is retroactive) is impossible once the channels
// have been summed and would be a silent lie if anyone assumed it.
func TestStereoRingsFoldFollowsTheModeFromTheNextChunk(t *testing.T) {
	s := newStereoRings(64, 2)
	s.write([]float32{1, 5})
	s.setMono(MonoRight)
	s.write([]float32{2, 6})

	got := make([]float32, 2)
	s.latest(got)
	if got[0] != 3 { // written under the mix: (1+5)/2
		t.Errorf("the first sample refolded to %v; it was written as the mix, 3", got[0])
	}
	if got[1] != 6 { // written under "right"
		t.Errorf("the second sample folded to %v, want the right channel, 6", got[1])
	}
	if s.mono != MonoRight {
		t.Errorf("mono = %v, want right", s.mono)
	}
}
