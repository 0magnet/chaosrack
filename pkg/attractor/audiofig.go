package attractor

import "math"

// The audio embeddings, built from a recorded signal instead of a live tap.
//
// takens_js.go, polar_js.go, stereo_js.go and xy_js.go read the page's audio
// tap every frame and draw the newest window. The window, the delay, the
// Catmull-Rom beam and the polar map are the arithmetic in takenswin.go and
// polarmap.go; what is here is the same walk over a plain slice, so that
// `chaosrack render` can draw an embedding of audio it recorded itself. Each
// mode is drawn at the page's default settings.

// AudioModels are the catalog models that render draws from a signal.
var AudioModels = []string{"takens", "stereo", "polar", "xy"}

// IsAudioModel reports whether a model is drawn from a signal.
func IsAudioModel(key string) bool {
	for _, k := range AudioModels {
		if k == key {
			return true
		}
	}
	return false
}

// AudioOptions are the page's knobs for an embedding.
type AudioOptions struct {
	SampleRate int
	// Tau is the delay in samples at 48 kHz, as the τ knob counts it
	// (see tauSamples); 0 is the knob's default.
	Tau float32
	// WindowMS is the display window; 0 is the mode's default.
	WindowMS float32
	// Budget is the point budget, the page's steps; 0 is 20000.
	Budget int
}

// xyWindowDef is the xy scope's default window, in milliseconds.
const xyWindowDef = 43

// AudioWindow is how many samples of signal one frame of a mode reads. A
// window ending before that many samples have been recorded draws nothing.
func AudioWindow(key string, o AudioOptions) int {
	o = o.withDefaults(key)
	tau := tauSamples(o.Tau, o.SampleRate)
	n, stride := takensWindow(o.WindowMS, o.SampleRate, o.Budget)
	switch key {
	case "stereo":
		return (n-1)*stride + tau + 1
	case "xy":
		return (n-1)*stride + 1
	}
	return (n-1)*stride + 2*tau + 1
}

func (o AudioOptions) withDefaults(key string) AudioOptions {
	if o.SampleRate <= 0 {
		o.SampleRate = 48000
	}
	if o.Tau <= 0 {
		o.Tau = takensTauDef
	}
	if o.WindowMS <= 0 {
		o.WindowMS = 85
		if key == "xy" {
			o.WindowMS = xyWindowDef
		}
	}
	if o.Budget <= 0 {
		o.Budget = 20000
	}
	return o
}

// AudioFigure builds a mode's figure from the window of l and r that ends
// just before sample end, as the page draws its newest window. It reports
// false for a model that is not an audio model, or a window that does not
// fit before end.
func AudioFigure(key string, l, r []float32, end int, o AudioOptions) (Figure, bool) {
	if !IsAudioModel(key) || len(l) != len(r) || end > len(l) {
		return Figure{}, false
	}
	o = o.withDefaults(key)
	need := AudioWindow(key, o)
	if end < need {
		return Figure{}, false
	}
	tau := tauSamples(o.Tau, o.SampleRate)
	n, stride := takensWindow(o.WindowMS, o.SampleRate, o.Budget)
	base := end - need

	// sample returns axis c of window point k, clamped at the ends as the
	// page's at() is so the spline's outer control points stay in range.
	var sample func(c, k int) float32
	switch key {
	case "takens", "polar":
		// The tap's default channel is the mix: both channels summed.
		sample = func(c, k int) float32 {
			i := base + 2*tau + k*stride - c*tau
			return (l[i] + r[i]) * 0.5
		}
	case "stereo":
		// Axis position 0: (L, R, L(t−τ)).
		sample = func(c, k int) float32 {
			i := base + tau + k*stride
			switch c {
			case 0:
				return l[i]
			case 1:
				return r[i]
			}
			return l[i-tau]
		}
	case "xy":
		// The scope's two deflection axes, flat in z.
		sample = func(c, k int) float32 {
			i := base + k*stride
			switch c {
			case 0:
				return l[i]
			case 1:
				return r[i]
			}
			return 0
		}
	}
	at := func(c, k int) float32 {
		if k < 0 {
			k = 0
		} else if k > n-1 {
			k = n - 1
		}
		return sample(c, k)
	}

	sm := takensSmooth()
	nv := takensVerts(n)
	pts := make([][3]float64, nv)
	for m := range pts {
		i := m / sm
		f := float32(m%sm) / float32(sm)
		var v [3]float32
		for c := 0; c < 3; c++ {
			p0, p1, p2, p3 := at(c, i-1), at(c, i), at(c, i+1), at(c, i+2)
			v[c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f)
		}
		if key == "polar" {
			// The default map, tanh at drive 2, bends the vector's length
			// and keeps its direction.
			s := polarScale(polarMapTanh, float32(math.Sqrt(float64(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))), 2)
			v[0], v[1], v[2] = v[0]*s, v[1]*s, v[2]*s
		}
		pts[m] = [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
	}
	return Figure{Kind: FigurePath, Points: pts}, true
}

// MeasureTau is the Takens mode's automatic τ: the first minimum of the
// average mutual information over the first 4096 samples of the mix, which is
// the window the page measures once it has that much audio. It returns τ in
// the knob's units (samples at 48 kHz), and false for a signal with no first
// minimum, such as white noise, where the knob's default stands.
func MeasureTau(l, r []float32, sr int) (float32, bool) {
	n := min(len(l), len(r), 4096)
	if n < 512 {
		return 0, false
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = float64(l[i]+r[i]) * 0.5
	}
	e := EstimateEmbedding(x, tauSamples(takensTauMax, sr), 8)
	if e.Tau < 1 {
		return 0, false
	}
	ref := float32(e.Tau)
	if sr > 0 && sr != tauRefRate {
		ref = float32(e.Tau) * float32(tauRefRate) / float32(sr)
	}
	return float32(int(ref + 0.5)), true
}
