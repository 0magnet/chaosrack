package rackhost

import (
	"math"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// A Mixer pin is a lamp: a square lit as bright as its gain. The page lights
// it by opacity, 0.45 + 0.55·|gain| over the module, and dark it is the
// module's own color; both as the catalog captured them.
const (
	pinDark = 0x141A21
	pinLit  = 0xFFD13C
)

// isPin reports whether a control is a Mixer pin.
func isPin(id string) bool { return strings.HasPrefix(id, "mix-") }

// findLamp is a pin's square: the item whose box is the pin's.
func (r *Rack) findLamp(box [4]float64, lo, hi int) (int, bool) {
	for i := lo; i < hi; i++ {
		it := r.pic.Items[i]
		if it != nil && it.Fill != nil && math.Abs(it.X-box[0]) < 0.5 && math.Abs(it.Y-box[1]) < 0.5 &&
			math.Abs(it.W-box[2]) < 0.5 && math.Abs(it.H-box[3]) < 0.5 {
			return i, true
		}
	}
	return 0, false
}

// light colors a pin's lamp for c's value.
func (r *Rack) light(item int, c *racktui.Control) {
	g, err := strconv.ParseFloat(c.Value, 64)
	if err != nil {
		return
	}
	col := pinDark
	if g != 0 {
		col = mixRGB(pinDark, pinLit, min(1, 0.45+0.55*math.Abs(g)))
	}
	if f := r.pic.Items[item].Fill; f != nil && *f == col {
		return
	}
	r.pic.Gen++
	it := *r.pic.Items[item]
	it.Fill = &col
	r.pic.Items[item] = &it
	r.changed[item] = r.pic.Gen
}

// mixRGB is a + t of the way to b, channel by channel.
func mixRGB(a, b int, t float64) int {
	out := 0
	for sh := 16; sh >= 0; sh -= 8 {
		x, y := float64(a>>sh&0xff), float64(b>>sh&0xff)
		out |= int(math.Round(x+(y-x)*t)) << sh
	}
	return out
}
