package stlview

import (
	"math"
	"testing"
)

func closeTo(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func sameColor(a, b Color) bool {
	return closeTo(a.Red, b.Red) && closeTo(a.Green, b.Green) && closeTo(a.Blue, b.Blue)
}

func TestColorArithmetic(t *testing.T) {
	a := Color{10, 20, 30}
	b := Color{1, 2, 3}

	if got, want := a.Add(b), (Color{11, 22, 33}); !sameColor(got, want) {
		t.Errorf("Add = %v, want %v", got, want)
	}
	if got, want := a.Subtract(b), (Color{9, 18, 27}); !sameColor(got, want) {
		t.Errorf("Subtract = %v, want %v", got, want)
	}
	if got, want := a.MultiplyFloat(2), (Color{20, 40, 60}); !sameColor(got, want) {
		t.Errorf("MultiplyFloat = %v, want %v", got, want)
	}
	if got, want := a.MultiplyFloat(0), (Color{0, 0, 0}); !sameColor(got, want) {
		t.Errorf("MultiplyFloat(0) = %v, want black", got)
	}
}

// Interpolation runs from the start color at 0 to the end color at 1. Getting
// the ends the wrong way round is invisible until a gradient runs backwards.
func TestInterpolateHitsBothEnds(t *testing.T) {
	start, end := Color{0, 0, 0}, Color{100, 200, 255}
	c := NewColorInterpolation(start, end)

	if got := c.Interpolate(0); !sameColor(got, start) {
		t.Errorf("Interpolate(0) = %v, want the start color %v", got, start)
	}
	if got := c.Interpolate(1); !sameColor(got, end) {
		t.Errorf("Interpolate(1) = %v, want the end color %v", got, end)
	}
}

func TestInterpolateIsLinearInTheMiddle(t *testing.T) {
	c := NewColorInterpolation(Color{0, 0, 0}, Color{100, 100, 100})
	if got, want := c.Interpolate(0.5), (Color{50, 50, 50}); !sameColor(got, want) {
		t.Errorf("Interpolate(0.5) = %v, want %v", got, want)
	}
	if got, want := c.Interpolate(0.25), (Color{25, 25, 25}); !sameColor(got, want) {
		t.Errorf("Interpolate(0.25) = %v, want %v", got, want)
	}
}

func TestNewRandomColorStaysInRange(t *testing.T) {
	for i := 0; i < 200; i++ {
		c := NewRandomColor()
		for name, v := range map[string]float32{"red": c.Red, "green": c.Green, "blue": c.Blue} {
			if v < 0 || v > 255 {
				t.Fatalf("%s = %v, outside 0..255", name, v)
			}
		}
	}
}

// Two calls returning the same color would mean the source was not random.
func TestNewRandomColorVaries(t *testing.T) {
	first := NewRandomColor()
	for i := 0; i < 50; i++ {
		if !sameColor(NewRandomColor(), first) {
			return
		}
	}
	t.Error("fifty random colors were all identical")
}

func TestGenerateSingleGradientLength(t *testing.T) {
	c := NewColorInterpolation(Color{0, 0, 0}, Color{255, 255, 255})
	for _, n := range []int{0, 1, 2, 10} {
		if got := generateSingleGradient(c, n); len(got) != n {
			t.Errorf("asked for %d steps, got %d", n, len(got))
		}
	}
}

// The gradient is used to color a mesh, so its length has to be the number of
// steps asked for: short and the far end of the model is unpainted.
func TestGenerateGradientReturnsTheStepsAskedFor(t *testing.T) {
	for _, tc := range []struct{ colors, steps int }{
		{2, 10}, {3, 30}, {5, 100}, {2, 2}, {6, 7},
	} {
		got := GenerateGradient(tc.colors, tc.steps)
		if len(got) != tc.steps {
			t.Errorf("GenerateGradient(%d, %d) gave %d colors, want %d",
				tc.colors, tc.steps, len(got), tc.steps)
		}
	}
}

// NewSTL picks between two and six colors, so one is outside what it asks for
// today — but a gradient of one color is a reasonable thing to ask for, and
// the answer should not be a crash.
func TestGenerateGradientWithOneColor(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GenerateGradient(1, 10) panicked: %v", r)
		}
	}()
	if got := GenerateGradient(1, 10); len(got) != 10 {
		t.Errorf("got %d colors, want 10", len(got))
	}
}

func TestGenerateGradientWithNoColors(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GenerateGradient(0, 10) panicked: %v", r)
		}
	}()
	if got := GenerateGradient(0, 10); len(got) != 0 {
		t.Errorf("got %d colors from no colors", len(got))
	}
}

func TestGenerateGradientWithNoSteps(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GenerateGradient(3, 0) panicked: %v", r)
		}
	}()
	if got := GenerateGradient(3, 0); len(got) != 0 {
		t.Errorf("got %d colors from no steps", len(got))
	}
}
