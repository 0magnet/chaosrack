package stlview

import (
	"crypto/rand"
	m "math"
)

// NewColorInterpolation generates color interpolation
func NewColorInterpolation(a Color, b Color) ColorInterpolation {
	return ColorInterpolation{
		a,
		b,
		a.Subtract(b),
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

// GenerateGradient Generates Gradient
func GenerateGradient(numColors int, steps int) []Color {
	distribution := distributeColors(numColors, steps)
	colors := make([]Color, numColors)
	for i := 0; i < numColors; i++ {
		colors[i] = NewRandomColor()
	}
	outputBuffer := make([]Color, 0, steps)
	for index := 0; index < numColors; index++ {
		if index >= numColors-1 {
			size := steps - distribution[index]
			interpolation := NewColorInterpolation(colors[index-1], colors[index])
			buffer := generateSingleGradient(interpolation, size)
			outputBuffer = append(outputBuffer, buffer...)
			break
		}
		currentStep := distribution[index]
		nextStep := distribution[index+1]
		size := nextStep - currentStep
		interpolation := NewColorInterpolation(colors[index], colors[index+1])
		buffer := generateSingleGradient(interpolation, size)
		outputBuffer = append(outputBuffer, buffer...)
	}
	return outputBuffer
}

func distributeColors(numColors int, steps int) []int {
	diff := int(m.Ceil(float64(steps) / float64(numColors)))
	output := make([]int, numColors)
	for i := 0; i < numColors; i++ {
		output[i] = diff * i
	}
	return output
}

func generateSingleGradient(c ColorInterpolation, numSteps int) []Color {
	output := make([]Color, numSteps)
	for i := 0; i < numSteps; i++ {
		percent := float32(i) / float32(numSteps)
		output[i] = c.Interpolate(percent)
	}
	return output
}
