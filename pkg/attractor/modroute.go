package attractor

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/led"
)

// Audio modulation's arithmetic, shared by the page (audiomod_js.go and
// modmatrix_js.go, which say what a route is and how the Mod matrix edits
// one) and a native rack: the sources a route can follow, the bands a
// route's EQ weighs, and what a routed knob reads at a route's depth.

// The routes' names for the Mixer's sends to modulation, MOD A and MOD B.
const (
	modSrcSendA = "modA"
	modSrcSendB = "modB"
)

// modChannels are the modulation sources, the Mod matrix's columns: the key
// its column is marked with, the name a route stores (paramMod.channel), and
// the group it is in. What each is, is the manual's (routing.md,
// mod-src=key). A route with no source is off, and has no column.
var modChannels = []struct{ key, name, group string }{
	{"st", "mono", "rack"},
	{"L", "L", "rack"},
	{"R", "R", "rack"},
	// The Mixer's two sends to modulation, which nothing else reads.
	{"A", modSrcSendA, "send"},
	{"B", modSrcSendB, "send"},
	// The model itself, closing the loop: the attractor's own current
	// output driving its own constants. A different system from the one
	// named on the dial, and deliberately so — see modelmod.go.
	{"x", modSrcModelX, "head"},
	{"y", modSrcModelY, "head"},
	{"z", modSrcModelZ, "head"},
	{"r", modSrcModelR, "head"},
}

// ModChannel is a modulation source: the key its matrix column is marked
// with, the name a route stores, and the group it is in.
type ModChannel struct{ Key, Name, Group string }

// ModChannels are the modulation sources, the Mod matrix's columns.
func ModChannels() []ModChannel {
	out := make([]ModChannel, len(modChannels))
	for i, c := range modChannels {
		out[i] = ModChannel{c.key, c.name, c.group}
	}
	return out
}

// IsModelModSource reports whether a route's source is the model itself
// rather than the sound (modelmod.go).
func IsModelModSource(name string) bool { return isModelModSource(name) }

// numEQBands is the resolution of the per-parameter graphic-EQ modulation
// source: the spectrum is split into this many log-spaced bands (low→high).
const numEQBands = 8

// ModBandCount is how many bands a route's EQ weighs.
const ModBandCount = numEQBands

// computeBands sums FFT magnitudes into numEQBands log-spaced frequency bands
// (≈30 Hz → Nyquist).
func computeBands(mags []float64, sr int) []float32 {
	nyq := float64(sr) / 2
	out := make([]float32, numEQBands)
	if nyq <= 30 {
		return out
	}
	logMin, logMax := math.Log(30), math.Log(nyq)
	for i, m := range mags {
		f := float64(i) / float64(len(mags)) * nyq
		if f < 30 {
			continue
		}
		b := int((math.Log(f) - logMin) / (logMax - logMin) * float64(numEQBands))
		if b < 0 {
			b = 0
		} else if b >= numEQBands {
			b = numEQBands - 1
		}
		out[b] += float32(m) //nolint:gosec // b is clamped to [0, numEQBands) immediately above
	}
	return out
}

// ComputeBands is computeBands, for a native rack.
func ComputeBands(mags []float64, sr int) []float32 { return computeBands(mags, sr) }

// ModBands are each channel's band energies as a route weighs them: each
// band divided by its own recent peak, so it reads the same at any level,
// and smoothed to rise fast and fall slowly, as the page's features are.
type ModBands struct {
	peak map[string]float32
	band map[string][]float32
	keys map[string][]string
}

// Store takes in channel ch's raw band energies, once a frame.
func (m *ModBands) Store(ch string, raw []float32) {
	if m.band == nil {
		m.peak, m.band, m.keys = map[string]float32{}, map[string][]float32{}, map[string][]string{}
	}
	cur, keys := m.band[ch], m.keys[ch]
	if cur == nil {
		// The peaks' keys made once a channel, not a frame.
		cur, keys = make([]float32, numEQBands), make([]string, numEQBands)
		for i := range keys {
			keys[i] = "eq:" + ch + ":" + strconv.Itoa(i)
		}
		m.band[ch], m.keys[ch] = cur, keys
	}
	for i := range numEQBands {
		p := m.peak[keys[i]]
		cur[i] = FeatureSmooth(cur[i], FeatureNorm(&p, raw[i]))
		m.peak[keys[i]] = p
	}
}

// Value is the modulation signal, 0..1, of channel ch weighed by a route's
// band weights: their weighted average. With no band painted it follows the
// loudest band, so a channel and a depth alone give a strong, beat-following
// drive; averaging all eight diluted quiet music to near nothing.
func (m *ModBands) Value(ch string, weights []float32) float32 {
	bands := m.band[ch]
	if ch == "" || len(bands) == 0 {
		return 0
	}
	var sum, wsum, maxb float32
	for i := range bands {
		var w float32
		if i < len(weights) {
			w = weights[i]
		}
		sum += w * bands[i]
		wsum += w
		maxb = max(maxb, bands[i])
	}
	v := maxb
	if wsum > 0 {
		v = sum / wsum
	}
	return min(max(v, 0), 1)
}

// ModTarget is what a routed knob reads: base, moved by depth level times
// the source's f across the knob's lo..hi, held to it; a knob of whole
// steps holds a step until the value is well into the next (quantizeHeld,
// whose last answer held is, when hasHeld).
func ModTarget(base, level, f, lo, hi, step, held float32, hasHeld bool) float32 {
	v := min(max(base+level*f*(hi-lo), lo), hi)
	if led.StepDecimals(step) == 0 {
		v = quantizeHeld(v, held, hasHeld, lo, hi, step)
	}
	return v
}

// ModViewRow is one of the Mod matrix's rows for the view rather than the
// model: its route's id, its name, the control it moves and that control's
// reach as a route moves it.
type ModViewRow struct {
	ID, Label, Control string
	Min, Max           float32
}

// ModViewRows are the matrix's view rows, in its order (modmatrix_js.go):
// View's spins and position, then the Colors module's period, shift and
// trail. TRAIL is a fraction of the trail a route takes away, and SHIFT
// wraps round the palette (audiomod_js.go).
var ModViewRows = []ModViewRow{
	{"view-spinx", "spin X", "rotation-controls-x", -1, 1},
	{"view-spiny", "spin Y", "rotation-controls-y", -1, 1},
	{"view-spinz", "spin Z", "rotation-controls-z", -1, 1},
	{"view-panx", "pan X", "pan-x", -8, 8},
	{"view-pany", "pan Y", "pan-y", -8, 8},
	{"view-zoom", "zoom", "camera-zoom", -95, 95},
	{"view-rfreq", "period", "rainbow-freq", 0.05, 20},
	{"view-pshift", "shift", "palette-shift", -1, 1},
	{"view-trail", "trail", "trail-slider", 0.02, 1},
}

// DisplayText is s as a character display spells it: the characters it has,
// one space between words.
func DisplayText(s string) string { return displayText(s) }

// ModelModNorm is a model coordinate v as a source, 0..1 across the figure's
// reach extent (modelmod.go).
func ModelModNorm(v, extent float32) float32 { return modelModNorm(v, extent) }

// ModelModSmooth moves a model source from prev toward target, as the page
// smooths it a frame.
func ModelModSmooth(prev, target float32) float32 { return modelModSmooth(prev, target) }
