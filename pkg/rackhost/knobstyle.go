package rackhost

import "github.com/0magnet/chaosrack/pkg/attractor"

// The Style module's KNOB, natively: the knobs' bodies, borders and pointers
// in the setting's colors (attractor.KnobStyles), as the page restyles its
// knobs. The picture was captured in the std look, so a knob is found by the
// std colors it was captured in, once a picture of it is loaded.

// knobParts are the picture's knob bodies and the pointers on them, by item.
type knobParts struct {
	scanned     bool
	bodies, ptr []int
}

// findKnobs finds the knobs among the picture's items from lo on, where a
// new bank has just put its own, and forgets the last bank's. The first
// time, it finds the ones outside the bank too. Called with r.mu held.
func (r *Rack) findKnobs(lo int) {
	k := &r.knobParts
	if !k.scanned {
		k.scanned, lo = true, 0
	}
	keep := func(s []int) []int {
		out := s[:0]
		for _, i := range s {
			if i < lo {
				out = append(out, i)
			}
		}
		return out
	}
	k.bodies, k.ptr = keep(k.bodies), keep(k.ptr)
	std := attractor.KnobStyles["std"]
	for i := lo; i < len(r.pic.Items); i++ {
		it := r.pic.Items[i]
		switch {
		case it == nil:
		case it.Pointer():
			if it.Fill != nil && *it.Fill == std.Pointer {
				k.ptr = append(k.ptr, i)
			}
		case it.Round == 1 && it.Fill != nil && *it.Fill == std.Body && it.Border != nil && *it.Border == std.Border:
			k.bodies = append(k.bodies, i)
		}
	}
}

// lookKnobs colors every knob in the KNOB setting's look. Called with r.mu
// held.
func (r *Rack) lookKnobs() {
	ks, ok := attractor.KnobStyles[r.value("knob-style")]
	if !ok {
		ks = attractor.KnobStyles["std"]
	}
	for _, i := range r.knobParts.bodies {
		r.recolor(i, ks.Body, ks.Border)
	}
	for _, i := range r.knobParts.ptr {
		r.recolor(i, ks.Pointer, -1)
	}
}

// recolor fills item i with fill, and borders it with border unless that is
// negative, as the next patch's.
func (r *Rack) recolor(i, fill, border int) {
	it := *r.pic.Items[i]
	if it.Fill != nil && *it.Fill == fill && (border < 0 || it.Border != nil && *it.Border == border) {
		return
	}
	it.Fill = &fill
	if border >= 0 {
		it.Border = &border
	}
	r.pic.Gen++
	r.pic.Items[i] = &it
	r.changed[i] = r.pic.Gen
}
