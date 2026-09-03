package attractor

import (
	"math"
	"testing"
)

// The harmonic character generator's math is pure Go, so the reconstruction
// contract is tested natively: with many harmonics each glyph curve must
// hug its stroke path; with one harmonic it must collapse to an ellipse.

func TestScopeTextStrokesCoverFont(t *testing.T) {
	for _, s := range []string{"A", "CHAOSRACK", "0123456789", "V-2"} {
		glyphs := scopeTextGlyphStrokes(s)
		if len(glyphs) != len([]rune(s)) {
			t.Fatalf("strokes(%q): %d glyphs, want %d", s, len(glyphs), len(s))
		}
		for gi, pts := range glyphs {
			if len(pts)%4 != 0 {
				t.Fatalf("strokes(%q)[%d]: %d coords, want multiple of 4", s, gi, len(pts))
			}
			for i := 0; i < len(pts); i += 2 {
				if math.Abs(pts[i]) > 2.0 || math.Abs(pts[i+1]) > 1.0 {
					t.Fatalf("strokes(%q)[%d]: point (%.2f, %.2f) outside the scope face", s, gi, pts[i], pts[i+1])
				}
			}
		}
	}
	sp := scopeTextGlyphStrokes(" ")
	if len(sp) != 1 || sp[0] != nil {
		t.Fatal("space should be one empty glyph")
	}
}

func TestScopeTextSynthConverges(t *testing.T) {
	for _, g := range scopeTextGlyphStrokes("HI") {
		curve := scopeTextSynth(g, 300, 1024)
		if len(curve) != 2048 {
			t.Fatalf("synth: got %d coords, want 2048", len(curve))
		}
		// Every reconstructed point must lie near the glyph's tour (piecewise
		// linear including the closing wrap).
		worst := 0.0
		for k := 0; k < len(curve); k += 2 {
			if d := distToStrokes(curve[k], curve[k+1], g); d > worst {
				worst = d
			}
		}
		if worst > 0.05 {
			t.Fatalf("300-harmonic glyph strays %.3f from its strokes (want ≤ 0.05)", worst)
		}
	}
}

func TestScopeTextSynthOneHarmonicIsEllipse(t *testing.T) {
	curve := scopeTextSynth(scopeTextGlyphStrokes("R")[0], 1, 512)
	// Harmonics −1..1 give an ellipse about the centroid — no sharp corner
	// survives, so the curve's second differences stay tiny.
	maxKink := 0.0
	n := len(curve) / 2
	for k := 0; k < n; k++ {
		a, b, c := (k-1+n)%n, k, (k+1)%n
		ddx := curve[a*2] - 2*curve[b*2] + curve[c*2]
		ddy := curve[a*2+1] - 2*curve[b*2+1] + curve[c*2+1]
		if d := math.Hypot(ddx, ddy); d > maxKink {
			maxKink = d
		}
	}
	if maxKink > 0.01 {
		t.Fatalf("1-harmonic curve has a kink of %.4f — not smooth (want ≤ 0.01)", maxKink)
	}
}

// distToStrokes: distance from (x,y) to the nearest segment of the glyph
// tour (pairs of endpoints), including the closing wrap segments between
// consecutive tour points — the reconstruction follows the full circuit.
func distToStrokes(x, y float64, strokes []float64) float64 {
	best := math.Inf(1)
	n := len(strokes) / 2
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		d := distToSeg(x, y, strokes[i*2], strokes[i*2+1], strokes[j*2], strokes[j*2+1])
		if d < best {
			best = d
		}
	}
	return best
}

func distToSeg(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := ((px-x1)*dx + (py-y1)*dy) / l2
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}
