package rackhost

import (
	"math/rand/v2"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// Jam, natively: with the Console's Jam switch up the rack performs itself,
// hopping to a random flow every 12–20 seconds with a fresh spin, as the
// page's does (attractor's jam.go).

// jamTick hops, if Jam is up and a hop is due at nowMs: at once when the
// switch has just gone up. Called with r.mu held, as the panel looks.
func (r *Rack) jamTick(nowMs float64) {
	if r.value("jam-sw") != "1" {
		r.jamNext = 0
		return
	}
	if r.jamNext != 0 && nowMs < r.jamNext {
		return
	}
	r.jamNext = nowMs + attractor.JamInterval(rand.Float64)              //nolint:gosec // a show, not a secret
	next, persist, y, x, ok := attractor.JamHop(rand.Float64, r.model()) //nolint:gosec // a show, not a secret
	if !ok || r.playModel(next) != nil {
		// A hop that did not happen is not one: try again at the next look,
		// rather than a whole interval on the model it was leaving.
		r.jamNext = 0
		return
	}
	_ = r.set("persist-trail", map[bool]string{true: "1", false: "0"}[persist]) //nolint:errcheck // a switch's own values
	_ = r.set("rotation-controls-y", strconv.FormatFloat(y, 'f', 1, 64))        //nolint:errcheck // within the knob's travel
	_ = r.set("rotation-controls-x", strconv.FormatFloat(x, 'f', 1, 64))        //nolint:errcheck // within the knob's travel
}

// playModel turns the MODEL knob to model key: to its own position, or for a
// member of a family (Sprott A–S) to the family's and its lettered dial to
// the member. Called with r.mu held.
func (r *Rack) playModel(key string) error {
	for _, c := range r.ctls {
		if !isFamily(c.ID) || len(c.Options) == 0 {
			continue
		}
		for _, o := range c.Options {
			if o == key {
				if err := r.set(r.selector, c.Options[0]); err != nil {
					return err
				}
				return r.set(c.ID, key)
			}
		}
	}
	return r.set(r.selector, key)
}
