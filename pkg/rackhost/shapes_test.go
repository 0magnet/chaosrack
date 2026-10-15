package rackhost

import (
	"bytes"
	"testing"
)

// A shape is drawn as its knobs make it, as the page draws it: the torus's
// radii, the solid {p,q}, the globe's twist, the Lissajous ratios. They were
// drawn at their defaults whatever the knobs said.
func TestShapesFollowTheirKnobs(t *testing.T) {
	for _, tc := range []struct{ model, id, v string }{
		{"torus", "torus-r", "1.5"},
		{"polyhedron", "polyhedron-p", "4"},
		{"globe", "globe-twist", "3"},
		{"lissajou", "lissajou-a", "7"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			r := newRack(t)
			set(t, r, "auto-rotate", "0", "pause-sw", "1", r.selector, tc.model)
			shot := func() []byte {
				m, err := r.Scene(120, 90, 1)
				if err != nil {
					t.Fatal(err)
				}
				return m.Px
			}
			shot()
			before := shot()
			set(t, r, tc.id, tc.v)
			if bytes.Equal(shot(), before) {
				t.Errorf("%s=%s drew the same %s", tc.id, tc.v, tc.model)
			}
		})
	}
}

// ROLL turns the torus's tube while the torus stays where it is.
func TestTorusRollTurnsTheTube(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", r.selector, "torus", "torus-roll", "4")
	a, err := r.Scene(120, 90, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Scene(120, 90, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Px, b.Px) {
		t.Error("a rolling torus drew the same frame twice")
	}
}
