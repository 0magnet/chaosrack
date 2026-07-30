// Package spectrogram implements functions for generating spectrograms from audio data.
package spectrogram

import (
	"image/color"
	"math"
	"math/cmplx"

	"github.com/0magnet/go-dsp/fft"
	"github.com/0magnet/go-dsp/window"
)

// Default constants (kept for backward compatibility)
const FFTSize = 1024
const SampleRate = 24000
const Overlap = 0.50
const StepSize = int(FFTSize * (1.0 - Overlap))

// SetSingleThreaded avoids the use of goroutines in go-dsp/fft library
func SetSingleThreaded() {
	fft.SetWorkerPoolSize(-1)
}

// Normalize normalizes the value between the given range.
func Normalize(value, minVal, maxVal float64) float64 {
	return (math.Max(math.Min(value, maxVal), minVal) - minVal) / (maxVal - minVal)
}

// ValueToPixelHeat converts a normalized value to a color based on a heatmap.
func ValueToPixelHeat(value float64) color.Color {
	var r, g, b uint8

	if value < 1.0/5.0 {
		b = uint8(255.0 * Normalize(value, 0.0, 1.0/5.0))
	} else if value < 2.0/5.0 {
		c := uint8(255.0 * Normalize(value, 1.0/5.0, 2.0/5.0))
		r = 0
		g = c
		b = 255 - c
	} else if value < 3.0/5.0 {
		r = uint8(255.0 * Normalize(value, 2.0/5.0, 3.0/5.0))
		g = 255
		b = 0
	} else if value < 4.0/5.0 {
		r = 255
		g = uint8(255 - 255.0*Normalize(value, 3.0/5.0, 4.0/5.0))
		b = 0
	} else {
		c := uint8(255.0 * Normalize(value, 4.0/5.0, 1.0))
		r = 255
		g = c
		b = c
	}

	return color.RGBA{r, g, b, 255}
}

// ValueToPixelBlue converts a normalized value to a color based on a blue gradient.
func ValueToPixelBlue(value float64) color.Color {
	var r, g, b uint8

	if value < 0.5 {
		b = uint8(255.0 * Normalize(value, 0.0, 0.5))
	} else {
		c := uint8(255.0 * Normalize(value, 0.5, 1.0))
		r = c
		g = c
		b = 255
	}

	return color.RGBA{r, g, b, 255}
}

// ValueToPixelGrayscale converts a normalized value to a grayscale color.
func ValueToPixelGrayscale(value float64) color.Color {
	c := uint8(255.0 * value)
	return color.RGBA{c, c, c, 255}
}

// MagnitudeToPixel converts a magnitude value to a pixel color using default settings.
func MagnitudeToPixel(value float64) color.Color {
	return MagnitudeToPixelWith(value, S)
}

// MagnitudeToPixelWith converts a magnitude value to a pixel color using the given settings.
func MagnitudeToPixelWith(value float64, s *Settings) color.Color {
	s.mu.RLock()
	scale := s.Mag
	magMin := s.MagMin
	magMax := s.MagMax
	cs := s.Color
	s.mu.RUnlock()

	if scale == ScaleLog {
		value = 20 * math.Log10(value+1e-10)
	}

	normalized := Normalize(value, magMin, magMax)

	switch cs {
	case ColorBlue:
		return ValueToPixelBlue(normalized)
	case ColorGrayscale:
		return ValueToPixelGrayscale(normalized)
	default:
		return ValueToPixelHeat(normalized)
	}
}

// ComputeFFT computes the FFT of the input and returns the magnitudes using default settings.
func ComputeFFT(input []float32) []float64 {
	return ComputeFFTWith(input, S)
}

// ComputeFFTWith computes the FFT using the window function from the given settings.
func ComputeFFTWith(input []float32, s *Settings) []float64 {
	s.mu.RLock()
	wf := s.Window
	s.mu.RUnlock()

	var win []float64
	switch wf {
	case WindowHamming:
		win = window.Hamming(len(input))
	case WindowBartlett:
		win = window.Bartlett(len(input))
	case WindowRectangular:
		win = window.Rectangular(len(input))
	default:
		win = window.Hann(len(input))
	}

	windowedBuffer := make([]float64, len(input))
	for i := 0; i < len(input); i++ {
		windowedBuffer[i] = float64(input[i]) * win[i]
	}

	spectrum := fft.FFTReal(windowedBuffer)
	magnitudes := make([]float64, len(spectrum)/2)
	for i := 0; i < len(magnitudes); i++ {
		magnitudes[i] = cmplx.Abs(spectrum[i])
	}
	return magnitudes
}
