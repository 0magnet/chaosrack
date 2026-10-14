package rackhost

import (
	"testing"
)

// legends is what the head's switch legends say now.
func legends(t *testing.T, r *Rack) [3]string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out [3]string
	for i, id := range []string{"model-sw-0", "model-sw-1", "model-sw-2"} {
		j, ok := r.legendItem[id]
		if !ok {
			t.Fatalf("no legend found for %s", id)
		}
		out[i] = r.pic.Items[j].Text
	}
	return out
}

// The head's switches say what they do for the model on show.
func TestTheSwitchLegendsFollowTheModel(t *testing.T) {
	r, _ := idleRack(t, "fvf")
	if got := legends(t, r); got != [3]string{"fx", "lstn", "rout"} {
		t.Errorf("FVF's switches read %q", got)
	}
	if r.value("model-sw-0") != "1" {
		t.Error("FVF's fx switch is not up, as its wobbulator is in")
	}
	set(t, r, r.selector, "pong")
	if got := legends(t, r); got != [3]string{"rset", "", ""} {
		t.Errorf("Pong's switches read %q", got)
	}
	set(t, r, r.selector, "globe")
	if got := legends(t, r); got != [3]string{} {
		t.Errorf("a globe's switches read %q", got)
	}
}

// FVF's switches: fx takes the wobbulator out, lstn plays it into FX, and
// rout, which only a server can do, will not go up.
func TestFVFsSwitches(t *testing.T) {
	r, _ := idleRack(t, "fvf")
	set(t, r, "model-sw-0", "0", "model-sw-1", "1", "model-sw-2", "1")
	s := r.sound
	s.mu.Lock()
	bypassed, listening, heard := s.wob.Bypassed(), s.listening(), s.heard()
	s.mu.Unlock()
	if !bypassed || !listening {
		t.Errorf("fx down left the wobbulator in (%v), or lstn up is not listening (%v)", !bypassed, listening)
	}
	if heard != s.pinned(srcFX) {
		t.Errorf("listening is heard %v with FX pinned %v", heard, s.pinned(srcFX))
	}
	if r.value("model-sw-2") != "0" {
		t.Error("rout stayed up with no server to route")
	}
	set(t, r, r.selector, "globe")
	s.mu.Lock()
	listening = s.listening()
	s.mu.Unlock()
	if listening {
		t.Error("Listen plays on with FVF gone")
	}
}

// Pong's rset is momentary: it restarts the game and springs back.
func TestPongsResetSpringsBack(t *testing.T) {
	r, c := idleRack(t, "pong")
	looks(t, r, c, 0.2)
	set(t, r, "model-sw-0", "1")
	if r.value("model-sw-0") != "0" {
		t.Error("rset stayed down")
	}
}

// FVF shows the input through the wobbulator: a tone gains sidebands the
// spectrogram of it does not have, and fx down shows the tone as it is.
func TestFVFShowsTheWobbulatedInput(t *testing.T) {
	bright := func(model string, fx string) int {
		r, c := toneRack(t, model)
		if model == "fvf" {
			set(t, r, "model-sw-0", fx)
		}
		watch(t, r, c, 4)
		sp := &r.analyzers.spect
		col := (sp.col - 1 + spectroTexW) % spectroTexW
		n := 0
		for y := range sp.pic.Rect.Dy() {
			p := sp.pic.RGBAAt(col, y)
			if int(p.R)+int(p.G)+int(p.B) > 200 {
				n++
			}
		}
		return n
	}
	plain, wobbled, bypassed := bright("spectrogram", ""), bright("fvf", "1"), bright("fvf", "0")
	if wobbled <= plain+5 {
		t.Errorf("FVF lights %d rows of a tone, the spectrogram %d: no sidebands", wobbled, plain)
	}
	if bypassed != plain {
		t.Errorf("fx down lights %d rows, the spectrogram %d", bypassed, plain)
	}
}
