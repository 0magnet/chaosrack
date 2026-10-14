package rackhost

import (
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The Poincaré section, natively: the page's section (attractor.
// PoincareSection) of the last flow shown, as its own model, and as the
// LINE S overlay over a running flow.

// sectView is the native section: its crossings, when it last ran, and the
// frame the model was fitted to once it had enough.
type sectView struct {
	sect *attractor.PoincareSection
	last time.Time
	fit  [][3]float64
}

// runSection brings the section of flow up to now, while running. Called
// with r.mu held.
func (r *Rack) runSection(flow string, running bool) {
	sv := &r.analyzers.sect
	for _, id := range attractor.PoincareKnobs {
		sv.sect.Set(id, float32(r.number(id)))
	}
	now := r.sound.now()
	frames := 0.0
	if running && !sv.last.IsZero() {
		// The page integrates a frame's budget at 60 frames a second.
		frames = math.Min(now.Sub(sv.last).Seconds()*60, 10)
	}
	sv.last = now
	_, scale := speedSteps(r.number("speed-slider"))
	if frames > 0 || sv.sect.Crossings() == 0 {
		sv.sect.Run(flow, float32(scale), max(frames, 1))
	}
}

// sectionFigure is the Poincaré model: the section of the last flow in the
// view its knob picks. Framed once there is a section worth framing, as the
// page fits its camera once. Called with r.mu held.
func (r *Rack) sectionFigure(running bool) (attractor.Figure, [][3]float64) {
	sv := &r.analyzers.sect
	before := sv.sect.Crossings()
	r.runSection(r.analyzers.lastFlow, running)
	f := attractor.Figure{Kind: attractor.FigurePoints}
	sv.sect.Points(sv.sect.View(), math.MaxInt32, func(x, y, z, _ float32) {
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), float64(z)})
	})
	if sv.sect.Crossings() < before {
		sv.fit = nil // a new section: the plane or the system moved
	}
	if sv.fit == nil && sv.sect.Crossings() > 256 {
		lo, hi := f.Points[0], f.Points[0]
		for _, p := range f.Points {
			for a := range 3 {
				lo[a], hi[a] = math.Min(lo[a], p[a]), math.Max(hi[a], p[a])
			}
		}
		sv.fit = [][3]float64{lo, hi}
	}
	return f, sv.fit
}

// sectionOverlay is LINE S over a running flow: the crossings in place, to
// draw in gold over the trail, or none. Called with r.mu held.
func (r *Rack) sectionOverlay(key string, running bool) attractor.Figure {
	if r.value("sect-sw") != "1" || key == "poincare" {
		return attractor.Figure{}
	}
	r.runSection(key, running)
	f := attractor.Figure{Kind: attractor.FigurePoints}
	r.analyzers.sect.sect.Points(0, math.MaxInt32, func(x, y, z, _ float32) {
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), float64(z)})
	})
	return f
}

// sectGold is the overlay's color: the page's monochrome gold.
var sectGold = [3]float32{1.0, 0.8, 0.15}
