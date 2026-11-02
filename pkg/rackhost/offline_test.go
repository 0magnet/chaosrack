package rackhost

import (
	"math"
	"testing"
	"time"
)

// litPixels counts the pixels of px that are not the scene's background.
func litPixels(px []byte) int {
	n := 0
	for i := 0; i+2 < len(px); i += 3 {
		if px[i] != sceneBack.R || px[i+1] != sceneBack.G || px[i+2] != sceneBack.B {
			n++
		}
	}
	return n
}

// tone is n samples of a sine at f hertz, starting at sample from, with the
// right channel a quarter turn behind.
func tone(f float64, from, n int) (l, r []float32) {
	l, r = make([]float32, n), make([]float32, n)
	for i := range n {
		t := float64(from+i) / sampleRate
		l[i] = float32(0.5 * math.Sin(2*math.Pi*f*t))
		r[i] = float32(0.5 * math.Cos(2*math.Pi*f*t))
	}
	return l, r
}

// TestOfflineFramesAddUpToTheSecond: whatever the rate, a second of frames
// stands for a second of samples, so a video's picture keeps time with its
// sound however long it runs.
func TestOfflineFramesAddUpToTheSecond(t *testing.T) {
	for _, fps := range []int{24, 30, 60, 144} {
		o := &Offline{fps: fps}
		n := 0
		for i := range fps {
			n += o.Samples(i)
		}
		if n != sampleRate {
			t.Errorf("%d fps: a second of frames is %d samples, not %d", fps, n, sampleRate)
		}
	}
}

// TestOfflineDrawsAnEmbeddingOfWhatItIsFed: an embedding of the samples
// handed to the frame, not of this machine's audio; silence draws nothing
// and a tone draws its figure.
func TestOfflineDrawsAnEmbeddingOfWhatItIsFed(t *testing.T) {
	o, err := NewOffline(60)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.SelectModel("stereo"); err != nil {
		t.Fatal(err)
	}
	const w, h = 160, 160
	var quiet, loud int
	at := 0
	for i := range 20 {
		n := o.Samples(i)
		var l, r []float32
		if i >= 10 {
			l, r = tone(220, at, n)
		}
		at += n
		img, _, _, err := o.Frame(w, h, l, r)
		if err != nil {
			t.Fatal(err)
		}
		if i == 9 {
			quiet = litPixels(img.Px)
		}
		if i == 19 {
			loud = litPixels(img.Px)
		}
	}
	if loud <= quiet {
		t.Errorf("a tone lit %d pixels, silence %d: the frame did not draw what it was fed", loud, quiet)
	}
}

// TestOfflineClockIsTheVideos: a frame moves the rack's clock on by one
// frame's share of a second exactly, however long it took to draw.
func TestOfflineClockIsTheVideos(t *testing.T) {
	o, err := NewOffline(50)
	if err != nil {
		t.Fatal(err)
	}
	for range 25 {
		if _, _, _, err := o.Frame(32, 32, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := o.r.sound.now().Sub(o.epoch); got != 500*time.Millisecond {
		t.Errorf("25 frames at 50 fps moved the clock %v, not 500ms", got)
	}
}

// TestOfflinePersistKeepsOldTrails: Display's Persist, a GUI control and not
// a flag of render's, reaches the frames: with it on, what was drawn stays.
func TestOfflinePersistKeepsOldTrails(t *testing.T) {
	run := func(persist string) int {
		o, err := NewOffline(30)
		if err != nil {
			t.Fatal(err)
		}
		if err := o.SelectModel("lorenz"); err != nil {
			t.Fatal(err)
		}
		if err := o.Set("persist-trail", persist); err != nil {
			t.Fatal(err)
		}
		var last []byte
		for range 30 {
			img, _, _, err := o.Frame(120, 120, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			last = img.Px
		}
		return litPixels(last)
	}
	off, on := run("0"), run("1")
	if on <= off {
		t.Errorf("Persist on lit %d pixels, off %d: it did not keep the old trails", on, off)
	}
}

// TestOfflineSelectsAFamilyMember: Sprott B is not on the selector; its
// family's position is, with the lettered dial on B.
func TestOfflineSelectsAFamilyMember(t *testing.T) {
	o, err := NewOffline(30)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.SelectModel("sprottb"); err != nil {
		t.Fatal(err)
	}
	o.r.mu.Lock()
	m := o.r.model()
	o.r.mu.Unlock()
	if m != "sprottb" {
		t.Errorf("the rack plays %q, not sprottb", m)
	}
	if err := o.SelectModel("nosuchmodel"); err == nil {
		t.Error("an unknown model was selected without complaint")
	}
}
