// Package rasterview is a software twin of pkg/attractor's WebGL line
// renderer: the same pose composition (Rx·Ry·Rz), the same gradient
// fragment shader ported to Go, drawing indexed line lists into an
// image.RGBA instead of a canvas. It exists for hosts without a GPU — a
// terminal UI paints the pixel buffer as half-block cells, a test
// encodes it to PNG — so the models look the same everywhere they
// appear. Stdlib only, so it builds anywhere (TinyGo included).
package rasterview

import (
	"image"
	"image/color"
	"math"
)

// Gradient is the coloring model of pkg/attractor's fragment shader: a
// source axis the gradient parameter t follows, and a palette applied to
// it. The zero value is invisible — start from DefaultGradient.
type Gradient struct {
	Source  int // t follows: 0=X, 1=Y, 2=Z (model space)
	Colors  int // palette: 1=monochrome, 2=two-color, 3=three-color, 4=rainbow
	Base    [3]float32
	Mid     [3]float32
	Top     [3]float32
	Freq    float32 // rainbow cycles over the range
	Phase   float32 // rainbow hue offset
	Reverse bool
}

// DefaultGradient is the look the attractor starts with on the web: a
// two-color red→blue gradient along model Z.
func DefaultGradient() Gradient {
	return Gradient{
		Source: 2,
		Colors: 2,
		Base:   [3]float32{1, 0, 0},
		Top:    [3]float32{0, 0, 1},
		Freq:   1,
	}
}

// View is a pose and projection. Angles compose Rx·Ry·Rz like the WebGL
// renderer's rebuildModelMatrix; auto-rotate is advancing AngleY.
type View struct {
	AngleX, AngleY, AngleZ float64
	Dist                   float64 // camera distance in model units; 0 = orthographic
	Scale                  float64 // model fills Scale of the half-frame; 0 = 0.85
	BackDim                float64 // 0..1: dim the far hemisphere (WebGL draws it undimmed; a raster benefits from ~0.5)
}

// Render draws an indexed line list (xyz triples + endpoint pairs, as
// pkg/geom generates and gl.LINES consumes) over whatever dst already
// holds. The frame is not cleared: composite onto a backdrop by copying
// it in first.
func (v View) Render(dst *image.RGBA, vertices []float32, indices []uint16, g Gradient) {
	n := len(vertices) / 3
	if n == 0 || dst == nil {
		return
	}
	b := dst.Bounds()
	pw, ph := b.Dx(), b.Dy()
	if pw == 0 || ph == 0 {
		return
	}

	// Model bounds: the shader's uMin/uMax gradient normalization, and
	// the fit radius for projection.
	var min, max [3]float32
	for a := 0; a < 3; a++ {
		min[a], max[a] = vertices[a], vertices[a]
	}
	maxLen := 0.0
	for i := 0; i < n; i++ {
		x, y, z := vertices[i*3], vertices[i*3+1], vertices[i*3+2]
		for a, val := range [3]float32{x, y, z} {
			if val < min[a] {
				min[a] = val
			}
			if val > max[a] {
				max[a] = val
			}
		}
		if l := math.Sqrt(float64(x*x + y*y + z*z)); l > maxLen {
			maxLen = l
		}
	}
	if maxLen == 0 {
		maxLen = 1
	}

	scale := v.Scale
	if scale == 0 {
		scale = 0.85
	}
	cx, cy := float64(pw)/2, float64(ph)/2
	r := math.Min(cx, cy) * scale / maxLen

	sinX, cosX := math.Sin(v.AngleX), math.Cos(v.AngleX)
	sinY, cosY := math.Sin(v.AngleY), math.Cos(v.AngleY)
	sinZ, cosZ := math.Sin(v.AngleZ), math.Cos(v.AngleZ)

	// Per-vertex screen position, depth, and color.
	px := make([]float64, n)
	py := make([]float64, n)
	cr := make([][3]float32, n)
	for i := 0; i < n; i++ {
		x0 := float64(vertices[i*3])
		y0 := float64(vertices[i*3+1])
		z0 := float64(vertices[i*3+2])

		// Rz, then Ry, then Rx — Rx·Ry·Rz applied to a column vector.
		x1, y1 := x0*cosZ-y0*sinZ, x0*sinZ+y0*cosZ
		x2, z2 := x1*cosY+z0*sinY, -x1*sinY+z0*cosY
		y3, z3 := y1*cosX-z2*sinX, y1*sinX+z2*cosX

		persp := 1.0
		if v.Dist > 0 {
			persp = v.Dist / (v.Dist - z3)
		}
		px[i] = cx + x2*r*persp
		py[i] = cy - y3*r*persp

		c := g.colorAt(vertices[i*3], vertices[i*3+1], vertices[i*3+2], min, max)
		if z3 < 0 && v.BackDim > 0 {
			dim := float32(1 - v.BackDim)
			c[0] *= dim
			c[1] *= dim
			c[2] *= dim
		}
		cr[i] = c
	}

	set := func(x, y int, c [3]float32) {
		if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
			return
		}
		dst.SetRGBA(x, y, color.RGBA{
			uint8(clamp01(c[0]) * 255),
			uint8(clamp01(c[1]) * 255),
			uint8(clamp01(c[2]) * 255),
			255,
		})
	}

	// DDA over each segment, color interpolated between endpoints.
	for i := 0; i+1 < len(indices); i += 2 {
		a, bIdx := int(indices[i]), int(indices[i+1])
		if a >= n || bIdx >= n {
			continue
		}
		dx, dy := px[bIdx]-px[a], py[bIdx]-py[a]
		steps := int(math.Max(math.Abs(dx), math.Abs(dy))) + 1
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)
			var c [3]float32
			for k := 0; k < 3; k++ {
				c[k] = cr[a][k] + float32(t)*(cr[bIdx][k]-cr[a][k])
			}
			set(int(px[a]+dx*t), int(py[a]+dy*t), c)
		}
	}
}

// colorAt ports the fragment shader's coloring: t from the source axis
// normalized over the model bounds, then the palette.
func (g Gradient) colorAt(x, y, z float32, min, max [3]float32) [3]float32 {
	var val, lo, hi float32
	switch g.Source {
	case 0:
		val, lo, hi = x, min[0], max[0]
	case 1:
		val, lo, hi = y, min[1], max[1]
	default:
		val, lo, hi = z, min[2], max[2]
	}
	span := hi - lo
	if span < 0.001 {
		span = 0.001
	}
	t := clamp01((val - lo) / span)
	if g.Reverse {
		t = 1 - t
	}
	switch g.Colors {
	case 1:
		return g.Base
	case 3:
		if t < 0.5 {
			return mix(g.Base, g.Mid, t*2)
		}
		return mix(g.Mid, g.Top, (t-0.5)*2)
	case 4:
		return hsv2rgb(t*g.Freq+g.Phase, 1, 1)
	default:
		return mix(g.Base, g.Top, t)
	}
}

func mix(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{
		a[0] + t*(b[0]-a[0]),
		a[1] + t*(b[1]-a[1]),
		a[2] + t*(b[2]-a[2]),
	}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// hsv2rgb ports the shader's helper exactly:
//
//	K = (1, 2/3, 1/3, 3); p = abs(fract(h + K.xyz)*6 - K.www)
//	rgb = v * mix(K.xxx, clamp(p - K.xxx, 0, 1), s)
func hsv2rgb(h, s, v float32) [3]float32 {
	fract := func(x float32) float32 { return x - float32(math.Floor(float64(x))) }
	k := [3]float32{0, 2.0 / 3.0, 1.0 / 3.0}
	var out [3]float32
	for i := 0; i < 3; i++ {
		p := float32(math.Abs(float64(fract(h+k[i])*6 - 3)))
		out[i] = v * (1 + s*(clamp01(p-1)-1))
	}
	return out
}
