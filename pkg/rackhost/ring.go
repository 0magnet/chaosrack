package rackhost

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// A knob's LED ring, lit as the page lights it: a bank position's encoder,
// a selector's dot and a generator's FREQ, whose laps are colors. The
// catalog marks each LED with its place round the ring and how the ring
// lights (attractor.RingSpec), and turning the knob relights them.

// ledRing is a control's ring: its LEDs' items, in order round it, and how
// it lights.
type ledRing struct {
	leds []int
	spec attractor.RingSpec
}

// findRing is the LED ring among items lo to hi in a control's box: of LEDs
// marked with the same place, the one nearest the box's middle, since a box
// can hold a neighbor's ring as well.
func (r *Rack) findRing(box [4]float64, lo, hi int) (ledRing, bool) {
	mx, my := box[0]+box[2]/2, box[1]+box[3]/2
	type led struct {
		item int
		dist float64
	}
	at := map[int]led{}
	var spec attractor.RingSpec
	for i := lo; i < hi; i++ {
		it := r.pic.Items[i]
		if it == nil || !strings.HasPrefix(it.Mark, "ring ") || !inside(it, box) {
			continue
		}
		f := strings.SplitN(it.Mark, " ", 3)
		if len(f) != 3 {
			continue
		}
		n, err := strconv.Atoi(f[1])
		s, ok := attractor.ParseRing(f[2])
		if err != nil || !ok {
			continue
		}
		d := math.Hypot(it.X+it.W/2-mx, it.Y+it.H/2-my)
		if old, has := at[n]; !has || d < old.dist {
			at[n], spec = led{i, d}, s
		}
	}
	if len(at) == 0 {
		return ledRing{}, false
	}
	places := make([]int, 0, len(at))
	for n := range at {
		places = append(places, n)
	}
	slices.Sort(places)
	rg := ledRing{spec: spec, leds: make([]int, places[len(places)-1]+1)}
	for i := range rg.leds {
		rg.leds[i] = -1
	}
	for n, l := range at {
		rg.leds[n] = l.item
	}
	return rg, true
}

// paintRing lights c's ring for its value, in the LED color chosen.
func (r *Rack) paintRing(rg ledRing, c *racktui.Control) {
	v, count := 0.0, 0
	if c.IsSelect {
		v, count = float64(max(slices.Index(c.Options, c.Value), 0)), len(c.Options)
	} else {
		f, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return
		}
		v = f
	}
	base := r.ledColorIndex()
	dark := hexColor(attractor.LEDColors[base].BG)
	for i, col := range rg.spec.Lights(v, count, len(rg.leds), base) {
		item := rg.leds[i]
		if item < 0 {
			continue
		}
		fill := dark
		if col >= 0 {
			fill = hexColor(attractor.LEDColors[col].Col)
		}
		if f := r.pic.Items[item].Fill; f != nil && *f == fill {
			continue
		}
		r.pic.Gen++
		it := *r.pic.Items[item]
		it.Fill = &fill
		r.pic.Items[item] = &it
		r.changed[item] = r.pic.Gen
	}
}

// paintRings relights every ring: when the LED color changes, which the
// laps count from, and once a bank is put up.
func (r *Rack) paintRings() {
	for id, rg := range r.rings {
		if i, ok := r.byID[id]; ok {
			r.paintRing(rg, &r.ctls[i])
		}
	}
}

// ledColorIndex is where the Style knob's LED color is in the table.
func (r *Rack) ledColorIndex() int {
	for i, c := range attractor.LEDColors {
		if c.Name == r.value("led-color") {
			return i
		}
	}
	return 0
}
