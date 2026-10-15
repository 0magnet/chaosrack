package rackhost

import (
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The Style module's LED color, natively: every readout drawn in an LED
// color — its lit segments, its ground and its border — redrawn in the one
// chosen, from the table the page colors its readouts from.

// hexColor is a CSS "#rrggbb" as the picture holds a color.
func hexColor(s string) int {
	v, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return -1
	}
	return int(v)
}

// colorLEDs redraws the readouts in the LED color chosen. A readout is
// known by its colors being one of the table's, which it was drawn in.
// Called with r.mu held, when the color is turned and when a bank is put up.
func (r *Rack) colorLEDs() {
	want := attractor.LEDColors[0]
	for _, c := range attractor.LEDColors {
		if c.Name == r.value("led-color") {
			want = c
		}
	}
	to := map[int]int{}
	for _, c := range attractor.LEDColors {
		to[hexColor(c.Col)] = hexColor(want.Col)
		to[hexColor(c.BG)] = hexColor(want.BG)
		to[hexColor(c.BD)] = hexColor(want.BD)
	}
	recolor := func(p **int) bool {
		if *p == nil {
			return false
		}
		n, ok := to[**p]
		if !ok || n == **p {
			return false
		}
		*p = &n
		return true
	}
	for i, it := range r.pic.Items {
		if it == nil {
			continue
		}
		c := *it
		if a, b, d := recolor(&c.Ink), recolor(&c.Fill), recolor(&c.Border); a || b || d {
			r.setItem(i, &c)
		}
	}
}
