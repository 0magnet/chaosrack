package attractor

import (
	"math"
	"slices"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/takens"
)

// The audio embeddings, built from a recorded signal instead of a live tap.
//
// takens_js.go, polar_js.go, stereo_js.go and xy_js.go read the page's audio
// tap every frame and draw the newest window. The window, the delay, the
// Catmull-Rom beam and the polar map are the arithmetic in pkg/takens; what
// is here is the same walk over a plain slice, so that
// `chaosrack render` can draw an embedding of audio it recorded itself, and
// the native rack its own signal. The Stereo Embedding is its StereoScope,
// the page's own.

// AudioModels are the catalog models that render draws from a signal.
var AudioModels = []string{"takens", "stereo", "polar", "xy"}

// IsAudioModel reports whether a model is drawn from a signal.
func IsAudioModel(key string) bool {
	return slices.Contains(AudioModels, key)
}

// AudioOptions are the page's knobs for an embedding. A zero is the knob's
// default.
type AudioOptions struct {
	SampleRate int
	// Tau is the delay in samples at 48 kHz, as the τ knob counts it
	// (see takens.TauSamples).
	Tau float32
	// WindowMS is the display window.
	WindowMS float32
	// Budget is the point budget, the page's steps; 0 is 20000.
	Budget int
	// Smooth is the spline steps a sample, as the smoothing knobs count
	// them; 0 is 4.
	Smooth float32
	// Chan is the channel Takens and Polar embed, as their SRC knob: the
	// mix, left, right, mid or side.
	Chan float32
	// PolarMap and PolarDrive are the Polar Embedding's radius map and its
	// drive; a zero drive is 2.
	PolarMap, PolarDrive float32
	// MidSide puts the XY on mid and side rather than left and right.
	MidSide bool
}

// xyWindowDef is the xy scope's default window, in milliseconds.
const xyWindowDef = 43

// AudioWindow is how many samples of signal one frame of a mode reads. A
// window ending before that many samples have been recorded draws nothing.
func AudioWindow(key string, o AudioOptions) int {
	o = o.withDefaults(key)
	switch key {
	case "stereo":
		snap, _ := o.stereo().Window(o.SampleRate, o.Budget, o.smooth())
		return snap
	case "xy":
		return xyWindowSamples(o.WindowMS, o.SampleRate)
	}
	tau := takens.TauSamples(o.Tau, o.SampleRate)
	n, stride := takens.Window(o.WindowMS, o.SampleRate, o.Budget, o.smooth())
	return (n-1)*stride + 2*tau + 1
}

func (o AudioOptions) withDefaults(key string) AudioOptions {
	if o.SampleRate <= 0 {
		o.SampleRate = 48000
	}
	if o.Tau <= 0 {
		o.Tau = takens.TauDef
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
	if o.Smooth <= 0 {
		o.Smooth = 4
	}
	if o.PolarDrive <= 0 {
		o.PolarDrive = 2
	}
	return o
}

// smooth is the smoothing as a step count, clamped as the knobs are.
func (o AudioOptions) smooth() int { return takens.Smooth(o.Smooth) }

// stereo is a Stereo Embedding at o's delay and window, untriggered and at
// unit gain.
func (o AudioOptions) stereo() *StereoScope {
	s := NewStereoScope()
	s.tau, s.win, s.gain = o.Tau, o.WindowMS, 1
	return s
}

// xyWindowSamples converts the WIN knob into a sample count, clamped to what
// one snapshot may ask the source for. The clamp is on the DURATION, so the
// only casualty is milliseconds that were asked for and cannot be had.
func xyWindowSamples(winMS float32, sr int) int {
	if sr <= 0 {
		sr = 48000
	}
	if maxMS := float32(xySpanMax) / float32(sr) * 1000; winMS > maxMS {
		winMS = maxMS
	}
	n := max(int(winMS/1000*float32(sr)), 64)
	return n
}

// xySpanMax is the most samples one snapshot may ask for, named against the
// source's own constant rather than repeating the number so the two cannot
// drift apart.
const xySpanMax = audiosrc.DefaultRingSize

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
	base := end - need
	sm := o.smooth()
	if key == "stereo" {
		s := o.stereo()
		_, nv := s.Window(o.SampleRate, o.Budget, sm)
		v := make([]float32, 4*nv)
		s.Frame(l[base:end], r[base:end], o.SampleRate, o.Budget, sm, v)
		return StereoFigure(v), true
	}
	tau := takens.TauSamples(o.Tau, o.SampleRate)
	n, stride := takens.Window(o.WindowMS, o.SampleRate, o.Budget, sm)
	if key == "xy" {
		// The scope draws every sample of its window.
		n, stride = need, 1
	}

	// sample returns axis c of window point k, clamped at the ends as the
	// page's at() is so the spline's outer control points stay in range.
	var sample func(c, k int) float32
	switch key {
	case "takens", "polar":
		ch := tapChanSel(o.Chan)
		sample = func(c, k int) float32 {
			i := base + 2*tau + k*stride - c*tau
			return tapFold(ch, l[i], r[i])
		}
	case "xy":
		// The scope's two deflection axes, flat in z, on left and right or
		// on mid and side.
		sample = func(c, k int) float32 {
			i := base + k*stride
			a, b := l[i], r[i]
			if o.MidSide {
				a, b = (a+b)*0.5, (a-b)*0.5
			}
			switch c {
			case 0:
				return a
			case 1:
				return b
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

	nv := takens.Verts(n, sm)
	if key == "xy" {
		nv = n * sm // the scope's line: every step of every sample
	}
	polarMap := clampSel(o.PolarMap, takens.PolarCount-1)
	pts := make([][3]float64, nv)
	for m := range pts {
		i := m / sm
		f := float32(m%sm) / float32(sm)
		var v [3]float32
		for c := range 3 {
			p0, p1, p2, p3 := at(c, i-1), at(c, i), at(c, i+1), at(c, i+2)
			v[c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f)
		}
		if key == "polar" {
			// The map bends the vector's length and keeps its direction,
			// applied after the spline as the page applies it.
			s := takens.PolarScale(polarMap, float32(math.Sqrt(float64(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))), o.PolarDrive)
			v[0], v[1], v[2] = v[0]*s, v[1]*s, v[2]*s
		}
		pts[m] = [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
	}
	return Figure{Kind: FigurePath, Points: pts}, true
}

// StereoFigure is a StereoScope's vertices as a figure.
func StereoFigure(v []float32) Figure {
	pts := make([][3]float64, len(v)/4)
	for m := range pts {
		pts[m] = [3]float64{float64(v[4*m]), float64(v[4*m+1]), float64(v[4*m+2])}
	}
	return Figure{Kind: FigurePath, Points: pts}
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
	e := takens.EstimateEmbedding(x, takens.TauSamples(takens.TauMax, sr), 8)
	if e.Tau < 1 {
		return 0, false
	}
	ref := float32(e.Tau)
	if sr > 0 && sr != takens.RefRate {
		ref = float32(e.Tau) * float32(takens.RefRate) / float32(sr)
	}
	return float32(int(ref + 0.5)), true
}
