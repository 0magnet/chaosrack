//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// Audio-feature analysis for modulating the attractors. When "Audio mod"
// is on we snapshot the latest stereo window each frame, FFT each channel,
// and derive a small set of smoothed, ~0..1 features per channel (L / R)
// plus a mono mix. Features are adaptively normalized so they track the
// audio's relative dynamics, not its absolute level — modulation stays
// constant regardless of system volume (like the spectrogram). Per-
// parameter routing (audiomod_js.go) reads these by name.
//
// Feature names: amp,bass,mid,treble,centroid,beat (mono mix) and the
// L-/R- prefixed per-channel variants (beat is mono only).

var (
	audioMod bool

	afWindowL []float32
	afWindowR []float32
	afMagsL   []float64 // persistent copy of the left-channel magnitudes
	afPrevMix []float64 // previous mixed magnitudes, for onset flux

	afFeat = map[string]float32{} // smoothed feature values
	afPeak = map[string]float32{} // adaptive normalization peaks

	afOverlay   js.Value
	afMeterFill [6]js.Value
	afFrameCnt  int
)

// afNormMap scales x by an adaptive per-key peak (instant rise, slow
// decay) → a level-independent 0..1 value.
func afNormMap(key string, x float32) float32 {
	p := afPeak[key]
	if x > p {
		p = x
	} else {
		p *= 0.9995
	}
	afPeak[key] = p
	if p < 1e-9 {
		return 0
	}
	r := x / p
	if r > 1 {
		r = 1
	}
	return r
}

// afSmooth: fast attack, slow release.
func afSmooth(cur, target float32) float32 {
	const attack, release = 0.6, 0.12
	if target > cur {
		return cur + (target-cur)*attack
	}
	return cur + (target-cur)*release
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// numEQBands is the resolution of the per-parameter graphic-EQ modulation
// source: the spectrum is split into this many log-spaced bands (low→high).
const numEQBands = 8

// afBand holds the current smoothed, adaptively-normalized band energies per
// channel ("mono","L","R"), each a []float32 of length numEQBands in 0..1.
var afBand = map[string][]float32{}

// afEQKeys precomputes the adaptive-normalization map keys per channel/band so
// the per-frame band update doesn't build 3×numEQBands strings every frame.
var afEQKeys = func() map[string][]string {
	m := map[string][]string{}
	for _, ch := range []string{"mono", "L", "R"} {
		ks := make([]string, numEQBands)
		for i := range ks {
			ks[i] = "eq:" + ch + ":" + strconv.Itoa(i)
		}
		m[ch] = ks
	}
	return m
}()

// afMonoScratch is reused by the per-frame mono band mix.
var afMonoScratch = make([]float32, numEQBands)

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
		out[b] += float32(m)
	}
	return out
}

// eqModValue returns the graphic-EQ modulation signal (0..1) for a channel
// and band-weight curve: the weight-normalized average of the channel's band
// energies. Empty channel or all-zero weights → 0 (unrouted).
func eqModValue(channel string, weights []float32) float32 {
	if channel == "" {
		return 0
	}
	bands := afBand[channel]
	if len(bands) == 0 {
		return 0
	}
	var sum, wsum, maxb float32
	for i := 0; i < len(bands); i++ {
		var w float32
		if i < len(weights) {
			w = weights[i]
		}
		sum += w * bands[i]
		wsum += w
		if bands[i] > maxb {
			maxb = bands[i]
		}
	}
	var v float32
	if wsum <= 0 {
		// No EQ bands painted (weights nil or all zero) → follow the loudest
		// band, so a channel + level ALONE gives a strong, beat-following
		// drive. Averaging all 8 bands diluted quiet music to near-nothing,
		// which made modulation feel dead until cranked to the max. Painting
		// bands then shapes/weights the response instead.
		v = maxb
	} else {
		v = sum / wsum
	}
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return v
}

// bandEnergies sums magnitudes in bass/mid/treble bands and computes the
// spectral centroid (0..1) for one channel's FFT.
func bandEnergies(mags []float64, sr int) (bass, mid, treble, centroid float64) {
	nyq := float64(sr) / 2
	var total, cw float64
	for i, m := range mags {
		f := float64(i) / float64(len(mags)) * nyq
		switch {
		case f < 250:
			bass += m
		case f < 2000:
			mid += m
		default:
			treble += m
		}
		total += m
		cw += f * m
	}
	if total > 0 {
		centroid = cw / total / nyq
	}
	return
}

func rmsOf(w []float32) float32 {
	var s float32
	for _, x := range w {
		s += x * x
	}
	return float32(math.Sqrt(float64(s / float32(len(w)))))
}

// updateAudioFeatures refreshes all features. Cheap (two FFTs/frame). No-op
// unless Audio mod is on and the source is delivering samples.
func updateAudioFeatures() {
	if !audioMod {
		return
	}
	src := ensureAudioSource()
	if src == nil || !src.Ready() {
		return
	}
	if afWindowL == nil {
		afWindowL = make([]float32, sg.FFTSize)
		afWindowR = make([]float32, sg.FFTSize)
	}
	src.TimeDomainStereo(afWindowL, afWindowR)
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}

	setNorm := func(name string, raw float32) { afFeat[name] = afSmooth(afFeat[name], afNormMap(name, raw)) }
	setRaw := func(name string, val float32) { afFeat[name] = afSmooth(afFeat[name], clamp01(val)) }

	// The local FFT returns a shared scratch, and both channels stay live
	// through the stereo-mix loop below — persist L into its own buffer.
	if afMagsL == nil {
		afMagsL = make([]float64, sg.FFTSize/2)
	}
	magsL := afMagsL[:copy(afMagsL, computeFFTMags(afWindowL))]
	magsR := computeFFTMags(afWindowR)
	bL, mL, tL, cL := bandEnergies(magsL, sr)
	bR, mR, tR, cR := bandEnergies(magsR, sr)

	// Graphic-EQ band energies per channel (adaptively normalized + smoothed).
	rawL := computeBands(magsL, sr)
	rawR := computeBands(magsR, sr)
	storeBands := func(ch string, raw []float32) {
		cur := afBand[ch]
		if cur == nil {
			cur = make([]float32, numEQBands)
			afBand[ch] = cur
		}
		keys := afEQKeys[ch]
		for i := 0; i < numEQBands; i++ {
			cur[i] = afSmooth(cur[i], afNormMap(keys[i], raw[i]))
		}
	}
	storeBands("L", rawL)
	storeBands("R", rawR)
	for i := range afMonoScratch {
		afMonoScratch[i] = (rawL[i] + rawR[i]) / 2
	}
	storeBands("mono", afMonoScratch)
	rL, rR := rmsOf(afWindowL), rmsOf(afWindowR)

	setNorm("L-amp", rL)
	setNorm("R-amp", rR)
	setNorm("L-bass", float32(bL))
	setNorm("R-bass", float32(bR))
	setNorm("L-mid", float32(mL))
	setNorm("R-mid", float32(mR))
	setNorm("L-treble", float32(tL))
	setNorm("R-treble", float32(tR))
	setRaw("L-centroid", float32(cL))
	setRaw("R-centroid", float32(cR))

	// Mono mix = average of the raw channel values.
	setNorm("amp", (rL+rR)/2)
	setNorm("bass", float32((bL+bR)/2))
	setNorm("mid", float32((mL+mR)/2))
	setNorm("treble", float32((tL+tR)/2))
	setRaw("centroid", float32((cL+cR)/2))

	// Onset/beat from mixed-magnitude spectral flux → decaying pulse.
	var flux float64
	n := len(magsL)
	if n > len(magsR) {
		n = len(magsR)
	}
	if afPrevMix == nil {
		afPrevMix = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		mix := (magsL[i] + magsR[i]) / 2
		if d := mix - afPrevMix[i]; d > 0 {
			flux += d
		}
		afPrevMix[i] = mix
	}
	if afNormMap("_flux", float32(flux)) > 0.55 && afFeat["beat"] < 0.35 {
		afFeat["beat"] = 1
	} else {
		afFeat["beat"] *= 0.86
	}

	updateAudioMeters()
}

// setAudioMod toggles the feature layer + per-parameter modulation. When
// off it also snaps the attractor state back to safety (see the reset in
// the else branch) so an over-modulated attractor recovers.
func setAudioMod(on bool) {
	audioMod = on
	if on {
		ensureAudioSource()
	} else {
		resetAttractorState()
	}
	updateMetersVisibility()
	// Show/hide the adjacent Modulation module (no param rebuild → the param
	// knobs never move; a whole module just appears/disappears beside them).
	if panel := doc.Call("getElementById", "controls-panel"); panel.Truthy() {
		cl := panel.Get("classList")
		if on {
			cl.Call("add", "am-on")
			cl.Call("remove", "am-off")
		} else {
			cl.Call("add", "am-off")
			cl.Call("remove", "am-on")
		}
	}
	updateViewModRows()
	quantizeModuleWidths()
}

// metersEnabled is the "Meters" switch state (independent of Audio mod). The
// overlay shows only when Audio mod is on AND this is enabled.
var metersEnabled = true

// updateMetersVisibility shows the top-left feature meters iff Audio mod is on
// and the Meters switch is enabled; otherwise hides them.
func updateMetersVisibility() {
	if audioMod && metersEnabled {
		showAudioMeters()
	} else if afOverlay.Truthy() {
		afOverlay.Get("style").Set("display", "none")
	}
}

// showAudioMeters builds (once) a small top-left overlay of the mono
// feature bars.
func showAudioMeters() {
	if !afOverlay.Truthy() {
		labels := [6]string{"amp", "bass", "mid", "treble", "cntr", "beat"}
		afOverlay = doc.Call("createElement", "div")
		afOverlay.Set("id", "audio-meters")
		st := afOverlay.Get("style")
		st.Set("position", "fixed")
		st.Set("top", "8px")
		st.Set("left", "8px")
		st.Set("padding", "6px 8px")
		st.Set("background", "rgba(0,0,0,0.6)")
		st.Set("font-family", "monospace")
		st.Set("font-size", "10px")
		st.Set("color", "#ccc")
		st.Set("z-index", "var(--z-hud)") // HUD level (with info overlay); below a recovered panel — see z-scale
		st.Set("pointer-events", "none")
		for i, lab := range labels {
			row := doc.Call("createElement", "div")
			row.Get("style").Set("display", "flex")
			row.Get("style").Set("alignItems", "center")
			row.Get("style").Set("margin", "1px 0")
			name := doc.Call("createElement", "span")
			name.Set("textContent", lab)
			name.Get("style").Set("width", "34px")
			track := doc.Call("createElement", "div")
			track.Get("style").Set("width", "80px")
			track.Get("style").Set("height", "6px")
			track.Get("style").Set("background", "#333")
			fill := doc.Call("createElement", "div")
			fill.Get("style").Set("height", "6px")
			fill.Get("style").Set("width", "0%")
			fill.Get("style").Set("background", "#4caf50")
			track.Call("appendChild", fill)
			row.Call("appendChild", name)
			row.Call("appendChild", track)
			afOverlay.Call("appendChild", row)
			afMeterFill[i] = fill
		}
		body.Call("appendChild", afOverlay)
	}
	afOverlay.Get("style").Set("display", "block")
	positionAudioMeters() // keep clear of a left/top-docked control panel
}

func updateAudioMeters() {
	if !afOverlay.Truthy() {
		return
	}
	afFrameCnt++
	if afFrameCnt%6 != 0 {
		return
	}
	names := [6]string{"amp", "bass", "mid", "treble", "centroid", "beat"}
	for i, nm := range names {
		if afMeterFill[i].Truthy() {
			afMeterFill[i].Get("style").Set("width", strconv.FormatFloat(float64(afFeat[nm]*100), 'f', 0, 64)+"%")
		}
	}
}
