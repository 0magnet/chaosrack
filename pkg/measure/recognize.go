package measure

import (
	"math"
	"strconv"
)

// Naming a measured number.
//
// A length measured against a chosen unit is a ratio, and the ratios this
// geometry is made of have names: √3 in the vesica, φ in the pentagon and
// the golden rectangle, 1/φ² where the pentagram's points end. Recognize
// gives a number the simplest such name it is within tolerance of — the
// thing a person with a ruler is trying to find out — or nothing, rather
// than a far-fetched one.

// Tolerance is how near a measured ratio must be to a name, relatively. A
// figure's circles are polylines, so a crossing read off their segments is a
// little off its exact place; this allows for that and little more.
const Tolerance = 1.5e-3

// name is a candidate: its value and how it is written.
type name struct {
	v    float64
	text string
	cost int // simpler is cheaper; the cheapest match wins
}

var names = func() []name {
	var out []name
	add := func(v float64, text string, cost int) { out = append(out, name{v, text, cost}) }
	frac := func(p, q int) string {
		if q == 1 {
			return strconv.Itoa(p)
		}
		return strconv.Itoa(p) + "/" + strconv.Itoa(q)
	}
	gcd := func(a, b int) int {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	// Whole numbers and simple fractions.
	for q := 1; q <= 8; q++ {
		for p := 1; p <= 16*q; p++ {
			if gcd(p, q) == 1 {
				add(float64(p)/float64(q), frac(p, q), 2*q+p/q)
			}
		}
	}
	// Square roots, and simple multiples of them.
	for _, n := range []int{2, 3, 5, 6, 7, 10, 11, 13, 15} {
		r := math.Sqrt(float64(n))
		s := "√" + strconv.Itoa(n)
		for q := 1; q <= 6; q++ {
			for p := 1; p <= 6; p++ {
				if gcd(p, q) != 1 {
					continue
				}
				text := s
				switch {
				case p == 1 && q == 1:
				case q == 1:
					text = strconv.Itoa(p) + s
				case p == 1:
					text = s + "/" + strconv.Itoa(q)
				default:
					text = strconv.Itoa(p) + s + "/" + strconv.Itoa(q)
				}
				add(float64(p)*r/float64(q), text, 4+2*q+p)
			}
		}
	}
	// The golden ratio's powers.
	phi := (1 + math.Sqrt(5)) / 2
	for k := 1; k <= 5; k++ {
		e := ""
		if k > 1 {
			e = []string{"", "", "²", "³", "⁴", "⁵"}[k]
		}
		add(math.Pow(phi, float64(k)), "φ"+e, 3+k)
		add(math.Pow(phi, -float64(k)), "1/φ"+e, 4+k)
	}
	add(math.Sqrt(phi), "√φ", 6)
	add(math.Pi, "π", 4)
	add(math.Pi/2, "π/2", 5)
	add(2*math.Pi, "2π", 5)
	return out
}()

// Recognize is the simplest name for x, and whether it has one.
func Recognize(x float64) (string, bool) {
	if x <= 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return "", false
	}
	best, found := name{cost: math.MaxInt}, false
	for _, n := range names {
		if math.Abs(x-n.v) <= Tolerance*n.v && n.cost < best.cost {
			best, found = n, true
		}
	}
	return best.text, found
}

// RecognizeAngle names an angle in degrees as a whole degree count that is a
// simple fraction of a half turn, with that fraction of π: "72° = 2π/5".
func RecognizeAngle(deg float64) (string, bool) {
	for q := 1; q <= 12; q++ {
		p := math.Round(deg * float64(q) / 180)
		if p <= 0 {
			continue
		}
		exact := 180 * p / float64(q)
		if math.Abs(deg-exact) > 0.15 {
			continue
		}
		g := gcdInt(int(p), q)
		pi, qi := int(p)/g, q/g
		text := ""
		switch {
		case pi == 1 && qi == 1:
			text = "π"
		case qi == 1:
			text = strconv.Itoa(pi) + "π"
		case pi == 1:
			text = "π/" + strconv.Itoa(qi)
		default:
			text = strconv.Itoa(pi) + "π/" + strconv.Itoa(qi)
		}
		return strconv.FormatFloat(exact, 'f', -1, 64) + "° = " + text, true
	}
	return "", false
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
