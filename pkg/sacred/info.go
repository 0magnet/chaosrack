package sacred

import (
	"math"
	"strconv"
)

// EquationLines is the Equation module's four lines for form at morph and detail, as
// label and text: what the form is, the number it is about, what it holds,
// and how far it is drawn.
func EquationLines(form int, morph, detail float64) [4][2]string {
	form = max(0, min(NumForms-1, form))
	d := int(math.Round(max(0, min(MaxDetail, detail))))
	f := func(x float64, prec int) string { return strconv.FormatFloat(x, 'f', prec, 64) }
	var l [4][2]string
	switch form {
	case Vesica:
		l[0] = [2]string{"form", "vesica piscis: two circles of one radius, each through the other's center"}
		l[1] = [2]string{"h : w", "√3 : 1 = " + f(math.Sqrt(3), 4) + ", the lens's height to its width"}
		l[2] = [2]string{"Euclid", "I.1: its corners and centers make the equilateral triangle on a line"}
	case Seed:
		l[0] = [2]string{"form", "Seed of Life: seven circles, six round one"}
		l[1] = [2]string{"step", "each center 60° on, where the last circle crossed the first"}
		l[2] = [2]string{"holds", "six petals; the centers make a hexagon and, joined alternately, a hexagram"}
	case Flower:
		whole, arcs := FlowerCounts(FlowerRings(d))
		l[0] = [2]string{"form", "Flower of Life: " + strconv.Itoa(FlowerRings(d)) + " rings of the hexagonal lattice of circles"}
		l[1] = [2]string{"count", strconv.Itoa(whole) + " circles whole, " + strconv.Itoa(arcs) + " arcs inside the bound"}
		l[2] = [2]string{"lattice", "centers a·u + b·w, u and w 60° apart: each circle through six others' centers"}
	case Fruit:
		l[0] = [2]string{"form", "Fruit of Life: thirteen circles that touch and never cross"}
		l[1] = [2]string{"centers", "0, then 2r and 4r along six directions 60° apart"}
		l[2] = [2]string{"from", "the Flower's lattice, every other ring along its six axes"}
	case Metatron:
		l[0] = [2]string{"form", "Metatron's Cube: the Fruit of Life's 13 centers, each joined to every other"}
		l[1] = [2]string{"count", "78 lines = 13·12/2"}
		l[2] = [2]string{"holds", "outlines of the cube, the octahedron and the star tetrahedron, seen along an axis"}
	case Golden:
		l[0] = [2]string{"form", "the golden rectangle and its " + strconv.Itoa(4+2*d) + " whirling squares"}
		l[1] = [2]string{"φ", "(1 + √5)/2 = " + f(Phi, 10) + ": a square off a φ:1 rectangle leaves another"}
		l[2] = [2]string{"spiral", "quarter circles, each 1/φ the last: φ⁴ = " + f(math.Pow(Phi, 4), 4) + " a full turn"}
	default:
		angle := GoldenAngle
		if morph > 1 {
			angle *= 1 + 0.02*(min(morph, 2)-1)
		}
		l[0] = [2]string{"form", "sunflower (Vogel 1979): seed n at r = √n, turned n·α"}
		l[1] = [2]string{"α", f(angle, 4) + "°; 360°/φ² = " + f(GoldenAngle, 4) + "° is the golden angle"}
		l[2] = [2]string{"spirals", "the spirals you see number Fibonacci: 13, 21, 34, 55, 89"}
	}
	l[3] = [2]string{"drawn", f(min(max(morph, 0), 1)*100, 0) + "% of the construction"}
	if morph > 1 {
		if form == Phyllotaxis {
			l[3][1] = "all of it, the angle off golden by " + f(2*(min(morph, 2)-1), 2) + "%"
		} else {
			l[3][1] = "all of it, twisted " + f(min(morph-1, 1)*180, 0) + "° at the rim"
		}
	}
	return l
}
