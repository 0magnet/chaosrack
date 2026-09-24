package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/segfont"
)

// Fourier Text — an homage to the glensstuff.com Fourier Synthesis
// Character Generator, which built alphanumerics on a scope from summed
// harmonics. The text is laid out in a 16-segment stroke font, the whole
// beam tour (segments AND the retrace jumps between them) is treated as
// one complex periodic signal p(t) = x(t) + i·y(t), and the drawn curve
// is rebuilt from only its first N Fourier harmonics — genuine harmonic
// synthesis, not a stylistic filter. At low N every character collapses
// toward an ellipse (one harmonic IS an ellipse); raising the harmonics
// knob adds overtones until the text sharpens into legibility. The
// retrace swoops between glyphs are what a truncated series really does
// with a jump, and Model Out (CAM) literally plays the harmonic stack.
//
// This file is untagged: the font, layout, and Fourier machinery are pure
// math (tested natively); the mode wiring lives in scopetext_js.go.

// scopeTextGlyphStrokes lays the string out on the segment font and returns
// one beam tour PER GLYPH (x,y waypoint pairs), glyphs positioned along the
// baseline and scaled to fit a ~3-wide, ~1.6-tall scope face. Each glyph is
// its own closed circuit — the hardware character generator synthesized
// characters individually, and per-glyph synthesis keeps every retrace
// swoop inside its own letterform. Spaces (and empty masks) yield nil
// entries. Unknown runes draw as '?'.
func scopeTextGlyphStrokes(text string) [][]float64 {
	const adv = 2.8 // cell width 2 + gap
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	width := float64(len(runes)-1)*adv + 2
	scale := 3.0 / width
	if s := 1.6 / 3.0; scale > s { // short text: cap by glyph height
		scale = s
	}
	x0 := -width / 2
	out := make([][]float64, 0, len(runes))
	for ci, r := range runes {
		glyph, ok := segfont.Segments(r)
		if !ok {
			glyph, _ = segfont.Segments('?')
		}
		off := x0 + float64(ci)*adv
		var segs [][4]float64
		for _, e := range glyph {
			segs = append(segs, [4]float64{
				(e[0] + off) * scale, (e[1] - 1.5) * scale,
				(e[2] + off) * scale, (e[3] - 1.5) * scale})
		}
		if len(segs) == 0 {
			out = append(out, nil)
			continue
		}
		// Greedy segment tour (nearest unused endpoint, either direction) so
		// the intra-glyph retrace mostly rides along the strokes.
		var pts []float64
		cx, cy := segs[0][0], segs[0][1]
		for len(segs) > 0 {
			best, bestD, flip := 0, math.Inf(1), false
			for i, sg := range segs {
				if d := math.Hypot(sg[0]-cx, sg[1]-cy); d < bestD {
					best, bestD, flip = i, d, false
				}
				if d := math.Hypot(sg[2]-cx, sg[3]-cy); d < bestD {
					best, bestD, flip = i, d, true
				}
			}
			sg := segs[best]
			segs = append(segs[:best], segs[best+1:]...)
			if flip {
				sg = [4]float64{sg[2], sg[3], sg[0], sg[1]}
			}
			pts = append(pts, sg[0], sg[1], sg[2], sg[3])
			cx, cy = sg[2], sg[3]
		}
		out = append(out, pts)
	}
	return out
}

// scopeTextJumpFractions returns the glyph tour's retrace spans — the
// jumps between strokes plus the closing wrap — as arc-length fractions of
// the closed circuit. The tour alternates stroke-start/stroke-end points,
// so every segment leaving an odd-indexed point is a jump. These are the
// spans a hardware character generator would key the z-axis off for; the
// renderer blanks the reconstructed curve across them.
func scopeTextJumpFractions(strokes []float64) [][2]float64 { //nolint:unused // built but not wired up yet; kept deliberately
	n := len(strokes) / 2
	if n < 2 {
		return nil
	}
	total := 0.0
	lens := make([]float64, n)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		lens[i] = math.Hypot(strokes[j*2]-strokes[i*2], strokes[j*2+1]-strokes[i*2+1])
		total += lens[i]
	}
	if total <= 0 {
		return nil
	}
	var spans [][2]float64
	arc := 0.0
	for i := 0; i < n; i++ {
		if i%2 == 1 { // leaving a stroke END → retrace (incl. the wrap)
			spans = append(spans, [2]float64{arc / total, (arc + lens[i]) / total})
		}
		arc += lens[i]
	}
	return spans
}

// scopeTextSplitCurve cuts a reconstructed closed curve (x,y pairs, arc
// parameter uniform in [0,1)) into drawable sub-strokes, dropping the
// samples that fall inside the tour's retrace spans — beam blanking.
func scopeTextSplitCurve(curve []float64, spans [][2]float64) [][]float64 { //nolint:unused // built but not wired up yet; kept deliberately
	res := len(curve) / 2
	if res == 0 {
		return nil
	}
	inJump := func(f float64) bool {
		for _, sp := range spans {
			if f >= sp[0] && f < sp[1] {
				return true
			}
		}
		return false
	}
	var out [][]float64
	var run []float64
	for k := 0; k < res; k++ {
		if inJump(float64(k) / float64(res)) {
			if len(run) >= 4 {
				out = append(out, run)
			}
			run = nil
			continue
		}
		run = append(run, curve[k*2], curve[k*2+1])
	}
	if len(run) >= 4 {
		out = append(out, run)
	}
	return out
}

// scopeTextSynth resamples the beam tour into a uniform-arc-length closed
// loop, takes its complex Fourier coefficients, and reconstructs the curve
// from only harmonics −N..N (n=0 is the centroid). Returns curve samples
// (x,y pairs, res points). The retrace jumps are part of the signal, so a
// truncated series turns them into the characteristic inter-glyph swoops.
func scopeTextSynth(strokes []float64, harmonics, res int) []float64 {
	if len(strokes) < 4 || harmonics < 1 || res < 8 {
		return nil
	}
	// Uniform resample (closed: the wrap from last point back to first is a
	// segment too, so the period has no discontinuity in sample spacing).
	const m = 2048
	n := len(strokes) / 2
	total := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		total += math.Hypot(strokes[j*2]-strokes[i*2], strokes[j*2+1]-strokes[i*2+1])
	}
	if total <= 0 {
		return nil
	}
	px := make([]float64, m)
	py := make([]float64, m)
	seg, segStart := 0, 0.0
	segLen := math.Hypot(strokes[2]-strokes[0], strokes[3]-strokes[1])
	for k := 0; k < m; k++ {
		s := total * float64(k) / float64(m)
		for s > segStart+segLen && seg < n-1 {
			segStart += segLen
			seg++
			j := (seg + 1) % n
			segLen = math.Hypot(strokes[j*2]-strokes[seg*2], strokes[j*2+1]-strokes[seg*2+1])
		}
		f := 0.0
		if segLen > 0 {
			f = (s - segStart) / segLen
		}
		j := (seg + 1) % n
		px[k] = strokes[seg*2] + (strokes[j*2]-strokes[seg*2])*f
		py[k] = strokes[seg*2+1] + (strokes[j*2+1]-strokes[seg*2+1])*f
	}
	// Complex DFT, harmonics −N..N only: c_n = (1/m) Σ p_k e^{−2πink/m}.
	// Both this and the reconstruction below run trig-free — the twiddle
	// rotates by complex-multiply recurrence — so a several-hundred-harmonic
	// rebuild stays interactive under a dragging knob.
	if harmonics > m/2-1 {
		harmonics = m/2 - 1
	}
	nh := 2*harmonics + 1
	cre := make([]float64, nh)
	cim := make([]float64, nh)
	for h := 0; h < nh; h++ {
		fn := float64(h - harmonics)
		wc := math.Cos(-2 * math.Pi * fn / m)
		ws := math.Sin(-2 * math.Pi * fn / m)
		rc, rs := 1.0, 0.0 // e^{−2πi·fn·k/m}, stepped per k
		var sre, sim float64
		for k := 0; k < m; k++ {
			// (px+ipy)·r: re = px·rc − py·rs, im = px·rs + py·rc
			sre += px[k]*rc - py[k]*rs
			sim += px[k]*rs + py[k]*rc
			rc, rs = rc*wc-rs*ws, rc*ws+rs*wc
		}
		cre[h], cim[h] = sre/m, sim/m
	}
	// Reconstruct res samples: p(t) = Σ c_n e^{int}, n stepped by e^{it}.
	out := make([]float64, res*2)
	for k := 0; k < res; k++ {
		t := 2 * math.Pi * float64(k) / float64(res)
		wc, ws := math.Cos(t), math.Sin(t)
		fn := -float64(harmonics)
		rc, rs := math.Cos(fn*t), math.Sin(fn*t)
		var x, y float64
		for h := 0; h < nh; h++ {
			x += cre[h]*rc - cim[h]*rs
			y += cre[h]*rs + cim[h]*rc
			rc, rs = rc*wc-rs*ws, rc*ws+rs*wc
		}
		out[k*2] = x
		out[k*2+1] = y
	}
	return out
}
