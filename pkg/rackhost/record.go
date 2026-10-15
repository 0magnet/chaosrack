package rackhost

import (
	"bytes"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// The Record module, natively: RECORD starts a take of the scene and pressed
// again, or STOP, ends it, written as a GIF in the working directory — the
// page downloads its takes, and the host has no video encoder, so a take is
// a GIF whatever FORMAT says. The counter runs while it records and the log
// line says what the take came out as, as the page's do (recordtext.go).

// recFPS is how many frames a second a take keeps, and recMaxSecs how long
// one runs before it stops itself: a GIF holds every frame in memory until
// it is written.
const (
	recFPS     = 15
	recMaxSecs = 60
)

// recWidth is how wide a take is, whatever the panel's scene: a picture worth
// keeping, not a terminal's cells.
const recWidth = 480

// recorder is a take: its frames, and when it started and last kept one.
type recorder struct {
	on     bool
	start  time.Time
	last   time.Time
	frames []*image.Paletted
	delays []int
}

// recSize is a take's frame size for a w by h scene of pixels shape times as
// tall as wide: recWidth across, and tall enough to keep its shape.
func recSize(w, h int, shape float64) (int, int) {
	return recWidth, max(1, int(float64(recWidth)*float64(h)*shape/float64(w)+0.5))
}

// keep adds frame m to the take, if a frame is due at now.
func (rc *recorder) keep(m *rackpic.Image, now time.Time) {
	if !rc.last.IsZero() && now.Sub(rc.last) < time.Second/recFPS {
		return
	}
	delay := 100 / recFPS
	if !rc.last.IsZero() {
		delay = max(2, int(now.Sub(rc.last).Milliseconds()/10)) // in GIF's hundredths
		rc.delays[len(rc.delays)-1] = delay
	}
	rc.last = now
	src := image.NewRGBA(image.Rect(0, 0, m.W, m.H))
	for i := range m.W * m.H {
		copy(src.Pix[4*i:], m.Px[3*i:3*i+3])
		src.Pix[4*i+3] = 255
	}
	p := image.NewPaletted(src.Rect, palette.Plan9)
	draw.FloydSteinberg.Draw(p, p.Rect, src, image.Point{})
	rc.frames = append(rc.frames, p)
	rc.delays = append(rc.delays, delay)
}

// recordPress is RECORD or STOP pressed. Called with r.mu held.
func (r *Rack) recordPress(id string) bool {
	rc := &r.rec
	switch {
	case id == "rec-btn" && !rc.on:
		*rc = recorder{on: true, start: r.sound.now()}
		r.showMeter([]attractor.Readout{{ID: "rec-status", Text: attractor.RecTimecode(0)}})
	case rc.on:
		r.endTake()
	}
	return true
}

// endTake writes the take out, off the panel's turn, and says what it came
// out as. Called with r.mu held.
func (r *Rack) endTake() {
	rc := r.rec
	r.rec = recorder{}
	if len(rc.frames) == 0 {
		return
	}
	secs := int(rc.last.Sub(rc.start).Seconds())
	r.showMeter([]attractor.Readout{{ID: "rec-log", Text: "saving…"}})
	go func() {
		var b bytes.Buffer
		line, saved := "", ""
		err := gif.EncodeAll(&b, &gif.GIF{Image: rc.frames, Delay: rc.delays})
		if err == nil {
			name := "chaosrack-" + rc.start.Format("20060102-150405") + ".gif"
			err = os.WriteFile(name, b.Bytes(), 0o644) //nolint:gosec // a take, for the user to open
			if abs, aerr := filepath.Abs(name); aerr == nil {
				saved = abs
			}
			line = attractor.RecLogLine("gif", secs, b.Len())
		}
		if err != nil {
			line = "not saved: " + err.Error()
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if err == nil {
			r.recSaved = saved
		}
		r.showMeter([]attractor.Readout{{ID: "rec-log", Text: line}})
	}()
}

// recordFrame keeps scene frame m for the take, runs its counter, and ends a
// take that has run its length. Called with r.mu held.
func (r *Rack) recordFrame(m *rackpic.Image) {
	rc := &r.rec
	if !rc.on {
		return
	}
	now := r.sound.now()
	rc.keep(m, now)
	secs := int(now.Sub(rc.start).Seconds())
	r.showMeter([]attractor.Readout{{ID: "rec-status", Text: attractor.RecTimecode(secs)}})
	if secs >= recMaxSecs {
		r.endTake()
	}
}

// shrink is m resampled to w by h, nearest pixel.
func shrink(m *rackpic.Image, w, h int) *rackpic.Image {
	out := &rackpic.Image{W: w, H: h, Px: make([]byte, 3*w*h)}
	for y := range h {
		sy := min(y*m.H/h, m.H-1)
		for x := range w {
			sx := min(x*m.W/w, m.W-1)
			copy(out.Px[3*(y*w+x):3*(y*w+x)+3], m.Px[3*(sy*m.W+sx):3*(sy*m.W+sx)+3])
		}
	}
	return out
}
