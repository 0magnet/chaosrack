package stlview

import (
	"crypto/rand"
)

// NewColorInterpolation generates color interpolation from a to b.
//
// The delta is b-a, not a-b. Interpolate adds delta*percent to the start, so
// with a-b it walked away from the end color instead of towards it: at percent
// 1 it reached 2a-b, and for any b brighter than a that is a negative channel.
func NewColorInterpolation(a Color, b Color) ColorInterpolation {
	return ColorInterpolation{
		a,
		b,
		b.Subtract(a),
	}
}

// ColorInterpolation is interpolated color
type ColorInterpolation struct {
	startColor Color
	endColor   Color
	deltaColor Color
}

// Interpolate interpolates
func (c ColorInterpolation) Interpolate(percent float32) Color {
	scaled := c.deltaColor.MultiplyFloat(percent)
	return c.startColor.Add(scaled)
}

// Color represents a color
type Color struct {
	Red   float32
	Green float32
	Blue  float32
}

// NewRandomColor returns a New RandomColor
func NewRandomColor() Color {
	const maxRGB = 255
	var r, g, b float64
	buf := make([]byte, 3)
	rand.Read(buf)
	r = float64(buf[0]) / 256
	g = float64(buf[1]) / 256
	b = float64(buf[2]) / 256
	r = r * maxRGB
	g = g * maxRGB
	b = b * maxRGB
	return Color{float32(r), float32(g), float32(b)}
}

// Subtract Subtracts color
func (c Color) Subtract(d Color) Color {
	return Color{
		c.Red - d.Red,
		c.Green - d.Green,
		c.Blue - d.Blue,
	}
}

// Add Adds color
func (c Color) Add(d Color) Color {
	return Color{
		c.Red + d.Red,
		c.Green + d.Green,
		c.Blue + d.Blue,
	}
}

// MultiplyFloat Multiplies Float
func (c Color) MultiplyFloat(x float32) Color {
	return Color{
		c.Red * x,
		c.Green * x,
		c.Blue * x,
	}
}

// GenerateGradient returns exactly steps colors, running through numColors
// random ones.
//
// The segment sizes are computed by scaling the boundaries rather than by
// spacing them a fixed distance apart. The fixed spacing rounded up, so once
// steps was not comfortably larger than numColors the last segment came out
// negative and make panicked with "len out of range" — GenerateGradient(6, 7)
// was enough. Scaled boundaries telescope: the sizes always sum to steps, and
// none of them can be below zero.
func GenerateGradient(numColors int, steps int) []Color {
	if numColors <= 0 || steps <= 0 {
		return nil
	}

	colors := make([]Color, numColors)
	for i := range colors {
		colors[i] = NewRandomColor()
	}

	// One color is not a gradient, and asking for the segment before the first
	// one used to read colors[-1].
	if numColors == 1 {
		out := make([]Color, steps)
		for i := range out {
			out[i] = colors[0]
		}
		return out
	}

	segments := numColors - 1
	out := make([]Color, 0, steps)
	for i := 0; i < segments; i++ {
		size := steps*(i+1)/segments - steps*i/segments
		interpolation := NewColorInterpolation(colors[i], colors[i+1])
		out = append(out, generateSingleGradient(interpolation, size)...)
	}
	return out
}

func generateSingleGradient(c ColorInterpolation, numSteps int) []Color {
	output := make([]Color, numSteps)
	for i := 0; i < numSteps; i++ {
		percent := float32(i) / float32(numSteps)
		output[i] = c.Interpolate(percent)
	}
	return output
}
