package sacred

import (
	"math"
	"strconv"
	"strings"
)

// EquationLines is the Equation module's four lines for form at morph and
// detail in dim dimensions, as label and text: what the form is, the number
// it is about, what it holds, and how far it is drawn.
func EquationLines(form int, morph, detail float64, dim int) [4][2]string {
	form = max(0, min(NumForms-1, form))
	d := int(math.Round(max(0, min(MaxDetail, detail))))
	f := func(x float64, prec int) string { return strconv.FormatFloat(x, 'f', prec, 64) }
	var l [4][2]string
	if dim == 3 {
		l = spaceLines(form, d, morph, f)
	} else {
		l = planeLines(form, d, morph, f)
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

func planeLines(form, d int, morph float64, f func(float64, int) string) [4][2]string {
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
	case Merkaba:
		l[0] = [2]string{"form", "the hexagram: two triangles, the star tetrahedron seen down its axis"}
		l[1] = [2]string{"inner", "the hexagon they share, 1/√3 = " + f(1/math.Sqrt(3), 4) + " of the star's radius"}
		l[2] = [2]string{"dim 3", "turns it into the merkaba, the two tetrahedra in a cube"}
	case Kepler:
		l[0] = [2]string{"form", "Kepler 1595: triangle, square, pentagon, hexagon, each between circles"}
		l[1] = [2]string{"ratio", "a triangle's inner circle is ½ its outer; Jupiter's orbit is 0.54 of Saturn's"}
		l[2] = [2]string{"then", "the rest do not fit, and the next year he went to the solids: dim 3"}
	default:
		l[0] = [2]string{"form", "sunflower (Vogel 1979): seed n at r = √n, turned n·α"}
		l[1] = [2]string{"α", f(SeedAngle(morph), 4) + "°; 360°/φ² = " + f(GoldenAngle, 4) + "° is the golden angle"}
		l[2] = [2]string{"spirals", "the spirals you see number Fibonacci: 13, 21, 34, 55, 89"}
	}
	return l
}

func spaceLines(form, d int, morph float64, f func(float64, int) string) [4][2]string {
	var l [4][2]string
	switch form {
	case Vesica:
		l[0] = [2]string{"form", "two spheres, each through the other's center"}
		l[1] = [2]string{"meet", "in a circle of radius √3/2 = " + f(math.Sqrt(3)/2, 4)}
		l[2] = [2]string{"seen", "side on, the vesica's two circles and its lens"}
	case Seed:
		l[0] = [2]string{"form", "Egg of Life: eight spheres on the corners of a cube"}
		l[1] = [2]string{"down", "the cube's diagonal, two corners meet at the center and six ring them"}
		l[2] = [2]string{"so", "seen down that axis (z) it is the Seed of Life"}
	case Flower:
		n := len(fccPoints(float64(PackingShells(d))))
		l[0] = [2]string{"form", "the Flower of Life in space: " + strconv.Itoa(n) + " spheres of the face-centered cubic packing"}
		l[1] = [2]string{"shells", "12 round one is the vector equilibrium; two shells are 55"}
		l[2] = [2]string{"layers", "each layer across z is the Flower's lattice, stacked three ways"}
	case Fruit:
		l[0] = [2]string{"form", "the vector equilibrium: thirteen spheres that touch, twelve round one"}
		l[1] = [2]string{"Fuller", "the cuboctahedron: its radius equals its edge, every vector in balance"}
		l[2] = [2]string{"holds", "4 hexagons through the center, 8 triangles and 6 squares outside"}
	case Metatron:
		l[0] = [2]string{"form", "Metatron's Cube in space: the vector equilibrium's 13 centers, each joined to every other"}
		l[1] = [2]string{"count", "78 lines = 13·12/2"}
		l[2] = [2]string{"holds", "the cuboctahedron, the octahedron, the star tetrahedron and the cube, in solid"}
	case Golden:
		l[0] = [2]string{"form", "three golden rectangles at right angles"}
		l[1] = [2]string{"corners", "(0, ±1, ±φ) and its turns: the twelve corners of the icosahedron"}
		l[2] = [2]string{"φ", "(1 + √5)/2 = " + f(Phi, 10)}
	case Merkaba:
		l[0] = [2]string{"form", "the merkaba, the star tetrahedron: two tetrahedra through each other"}
		l[1] = [2]string{"in", "a cube, their corners its corners; they share an octahedron"}
		l[2] = [2]string{"down", "the cube's diagonal it is the hexagram"}
	case Kepler:
		var rs []string
		for i, r := range keplerRadii() {
			rs = append(rs, Planets[i]+" "+f(r, 3))
		}
		l[0] = [2]string{"form", "Mysterium Cosmographicum (1596): cube, tetrahedron, dodecahedron, icosahedron, octahedron"}
		l[1] = [2]string{"spheres", strings.Join(rs, " · ")}
		l[2] = [2]string{"orbits", "Jupiter 0.54, Mars 0.16, Earth 0.10, Venus 0.08, Mercury 0.04 of Saturn's"}
	default:
		l[0] = [2]string{"form", "the Fibonacci sphere: seed n at height 1 − (2n+1)/N, turned n·α"}
		l[1] = [2]string{"α", f(SeedAngle(morph), 4) + "°; the golden angle spaces them most evenly"}
		l[2] = [2]string{"used", "for the most even points on a sphere: sampling, meshes, antennas"}
	}
	return l
}
