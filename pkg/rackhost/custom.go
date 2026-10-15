package rackhost

import (
	"errors"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// Custom, natively: the page's typed system (attractor.CustomSystem),
// compiled and registered as "custom", so the flow and map flavors draw as
// every flow and map does, and the surface flavor by its contours.

// customKnobs writes the knobs into the system: its constants, custom-NAME,
// and its step, custom-dt. A knob the panel has no value for keeps the
// system's. Called with r.mu held; the constants are written under the
// sound's lock, as Model Out reads them on the speaker's goroutine.
func (r *Rack) customKnobs() {
	c := r.custom
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range c.Params() {
		if v, ok := r.knobValue("custom-" + name); ok {
			c.SetParam(name, float32(v))
		}
	}
	if v, ok := r.knobValue("custom-dt"); ok {
		c.SetDt(float32(v))
	}
	if v, ok := r.knobValue("custom-extent"); ok {
		c.SetExtent(float32(v))
	}
}

// customFigure is Custom's figure at its flavor. Called with r.mu held.
func (r *Rack) customFigure() (attractor.Figure, error) {
	c := r.custom
	r.customKnobs()
	if err := c.Err(); err != "" {
		return attractor.Figure{}, errors.New("custom: " + err)
	}
	if _, surface, _ := c.Flavor(); surface {
		_, pv := c.SurfaceKey()
		v := c.SurfaceLines(pv, surfaceSlices, surfaceRes)
		f := attractor.Figure{Kind: attractor.FigureLines}
		for i := 0; i+2 < len(v); i += 3 {
			f.Points = append(f.Points, [3]float64{float64(v[i]), float64(v[i+1]), float64(v[i+2])})
			f.Edges = append(f.Edges, uint16(len(f.Points)-1)) //nolint:gosec // implicit.Contours stops short of 65535 vertices
		}
		e := float64(c.Extent())
		// The region drawn, framed as the page frames it: known by
		// construction, not by what the surface happens to reach.
		f.Points = append(f.Points, [3]float64{-e, -e, -e}, [3]float64{e, e, e})
		return f, nil
	}
	return figureFor("custom", nil, 0)
}

// The surface flavor's detail, the page's settled drawing.
const surfaceSlices, surfaceRes = 18, 56

// customSwitches are the Visual head's switches for Custom: iter reads the
// equations as a map, surf as a surface, and 4d w adds the fourth state to
// a flow. Called with r.mu and the sound's lock held.
func (r *Rack) customSwitches() []modelSwitch {
	c := r.custom
	flavor := func(set func(iter, surf, w *bool)) func(bool) {
		return func(bool) {
			i, s, w := c.Flavor()
			set(&i, &s, &w)
			c.SetFlavor(i, s, w)
			r.refigure.Store(true)
		}
	}
	return []modelSwitch{
		{legend: "iter",
			on: func() bool { i, _, _ := c.Flavor(); return i },
			set: func(v bool) {
				flavor(func(i, s, _ *bool) { *i = v; *s = *s && !v })(v)
			}},
		{legend: "surf",
			on: func() bool { _, s, _ := c.Flavor(); return s },
			set: func(v bool) {
				flavor(func(i, s, _ *bool) { *s = v; *i = *i && !v })(v)
			}},
		{legend: "4d w",
			on:  func() bool { _, _, w := c.Flavor(); return w },
			set: func(v bool) { flavor(func(_, _, w *bool) { *w = v })(v) }},
	}
}

// knobValue is a control's value as a number, if the rack has the control
// and the control has one. Called with r.mu held.
func (r *Rack) knobValue(id string) (float64, bool) {
	i, ok := r.byID[id]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(r.ctls[i].Value, 64)
	return v, err == nil
}

// seedCustomKnobs gives Custom's knobs the template's values. The page
// builds them for whatever equations are typed, so the catalog has them
// with no value at all, and a dial turned from nothing would start the
// constant from zero. Called with r.mu held, once.
func (r *Rack) seedCustomKnobs() {
	put := func(id string, v float32) {
		if i, ok := r.byID[id]; ok && r.ctls[i].Value == "" {
			r.ctls[i].Value = strconv.FormatFloat(float64(v), 'g', -1, 32)
		}
	}
	for _, name := range r.custom.Params() {
		if v, ok := r.custom.Param(name); ok {
			put("custom-"+name, v)
		}
	}
	put("custom-dt", r.custom.Dt())
	// The Turtle's Physics knobs too, which the page builds only with its
	// switch up.
	for _, k := range attractor.TurtlePhysicsKnobs {
		put(k.ID, k.Def)
	}
	// The equations too, and the fourth, which the page builds only while 4d
	// w is on, so the catalog may not have it.
	for i := range 4 {
		id := "custom-eq-" + strconv.Itoa(i)
		r.addControl(racktui.Control{ControlInfo: controlspec.ControlInfo{
			ID: id, Label: [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}[i], IsText: true, Module: "equation",
		}})
		if j := r.byID[id]; r.ctls[j].Value == "" {
			r.ctls[j].Value = r.custom.Equation(i)
		}
	}
}
