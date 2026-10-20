//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/meters"
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

// audioFeatures is the audio-feature analysis that modulates the attractors:
// its windows, its spectra and the features it extracts.
type audioFeatures struct {
	windowL []float32
	windowR []float32
	magsL   []float64          // persistent copy of the left-channel magnitudes
	magsA   []float64          // the same for MOD A, while MOD B's are computed
	prevMix []float64          // previous mixed magnitudes, for onset flux
	feat    map[string]float32 // smoothed feature values
	peak    map[string]float32 // adaptive normalization peaks

	// The Features module's ladders, and how many segments each has lit.
	ladder    [len(featNames)]js.Value
	ladderLit [len(featNames)]int

	// bands holds the current smoothed, adaptively-normalized band energies
	// per channel ("mono", "L", "R", the sends), each numEQBands long, 0..1
	// (modroute.go).
	bands ModBands
	// wave is each channel's newest sample, for the wave shape (ModWave).
	wave ModWave

	// monoScratch is reused by the per-frame mono band mix.
	monoScratch []float32
}

var af = audioFeatures{
	feat:        map[string]float32{},
	peak:        map[string]float32{},
	monoScratch: make([]float32, numEQBands),
}

var (
	audioMod bool
)

// normMap scales x by an adaptive per-key peak (instant rise, slow
// decay) → a level-independent 0..1 value.
func (a *audioFeatures) normMap(key string, x float32) float32 {
	p := a.peak[key]
	r := FeatureNorm(&p, x) // featurefollow.go, which a native rack follows with too
	a.peak[key] = p
	return r
}

// afSmooth: fast attack, slow release.
func afSmooth(cur, target float32) float32 { return FeatureSmooth(cur, target) }

// eqModValue returns the graphic-EQ modulation signal (0..1) for a channel
// and band-weight curve: the weight-normalized average of the channel's band
// energies, or with no band painted the loudest band (ModBands.Value). An
// empty channel → 0 (unrouted).
func (a *audioFeatures) eqModValue(channel string, weights []float32) float32 {
	if channel == "" {
		return 0
	}
	// The model's own output is a source like any other from here on: a
	// 0..1 signal the depth knob scales. It has no spectrum, so the band
	// EQ below means nothing to it and is skipped rather than applied to
	// a coordinate as though it were one.
	if isModelModSource(channel) {
		return modelModValue(channel)
	}
	return a.bands.Value(channel, weights)
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
func (a *audioFeatures) updateAudioFeatures() {
	if !audioMod {
		return
	}
	src := aud.ensureAudioSource()
	if !src.Ready() {
		return
	}
	if a.windowL == nil {
		a.windowL = make([]float32, sg.FFTSize)
		a.windowR = make([]float32, sg.FFTSize)
	}
	src.TimeDomainStereo(a.windowL, a.windowR)
	a.wave.Set("L", Newest(a.windowL))
	a.wave.Set("R", Newest(a.windowR))
	a.wave.Set("mono", (Newest(a.windowL)+Newest(a.windowR))/2)
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}

	setNorm := func(name string, raw float32) { a.feat[name] = afSmooth(a.feat[name], a.normMap(name, raw)) }
	setRaw := func(name string, val float32) { a.feat[name] = afSmooth(a.feat[name], clamp01(val)) }

	// The local FFT returns a shared scratch, and both channels stay live
	// through the stereo-mix loop below — persist L into its own buffer.
	if a.magsL == nil {
		a.magsL = make([]float64, sg.FFTSize/2+1)
	}
	magsL := a.magsL[:copy(a.magsL, meters.ComputeFFTMags(a.windowL))]
	magsR := meters.ComputeFFTMags(a.windowR)
	bL, mL, tL, cL := bandEnergies(magsL, sr)
	bR, mR, tR, cR := bandEnergies(magsR, sr)

	// Graphic-EQ band energies per channel (adaptively normalized + smoothed).
	rawL := computeBands(magsL, sr)
	rawR := computeBands(magsR, sr)
	a.bands.Store("L", rawL)
	a.bands.Store("R", rawR)
	for i := range a.monoScratch {
		a.monoScratch[i] = (rawL[i] + rawR[i]) / 2
	}
	a.bands.Store("mono", a.monoScratch)
	// The sends to modulation, MOD A and B, when a route reads one: a
	// window of each, mixed at the Mixer's gains, and its bands, as the
	// rack's two sides have theirs.
	if bus, ok := src.(*audiosrc.Bus); ok && bus.ModOn() && modReadsSends() {
		wa, wb := bus.ModWindow(sg.FFTSize)
		a.wave.Set(modSrcSendA, Newest(wa))
		a.wave.Set(modSrcSendB, Newest(wb))
		if a.magsA == nil {
			a.magsA = make([]float64, sg.FFTSize/2+1)
		}
		magsA := a.magsA[:copy(a.magsA, meters.ComputeFFTMags(wa))]
		a.bands.Store(modSrcSendA, computeBands(magsA, sr))
		a.bands.Store(modSrcSendB, computeBands(meters.ComputeFFTMags(wb), sr))
	}
	rL, rR := rmsOf(a.windowL), rmsOf(a.windowR)

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
	n := min(len(magsL), len(magsR))
	if a.prevMix == nil {
		a.prevMix = make([]float64, n)
	}
	for i := range n {
		mix := (magsL[i] + magsR[i]) / 2
		if d := mix - a.prevMix[i]; d > 0 {
			flux += d
		}
		a.prevMix[i] = mix
	}
	if a.normMap("_flux", float32(flux)) > 0.55 && a.feat["beat"] < 0.35 {
		a.feat["beat"] = 1
	} else {
		a.feat["beat"] *= 0.86
	}

	a.updateAudioMeters()
}

// syncAudioMod runs the feature layer and the modulator when there is a
// reason to: a route in the Mod matrix, the bifurcation's audio drive, or a
// feed the server streams, which asks nothing of the visitor. There is no
// switch for it any more. A plain visit with none of those does not open the
// microphone at load; the first route does.
func syncAudioMod() {
	on := anyModRoute() || bif.driveAudio || audioStreamed()
	if on != audioMod {
		setAudioMod(on)
	}
}

// anyModRoute reports whether any control is routed.
func anyModRoute() bool {
	for _, m := range pmod.params {
		if m.channel != "" && m.level != 0 {
			return true
		}
	}
	return false
}

// audioStreamed reports whether the audio is a feed from the server, which
// opens without asking, rather than the microphone, which asks.
func audioStreamed() bool {
	switch audioBackendKind() {
	case "ws", "websocket", "wt", "webtransport":
		return true
	}
	return false
}

// setAudioMod starts or stops the feature layer + per-parameter modulation
// (syncAudioMod decides which). Stopping also snaps the attractor state back
// to safety (see the reset in the else branch) so an over-modulated
// attractor recovers.
func setAudioMod(on bool) {
	audioMod = on
	if on {
		aud.ensureAudioSource()
	} else {
		resetAttractorState()
	}
	if !on {
		af.darkLadders() // nothing measured: no reading left standing
	}
}

// The Features module (feat-module): the six features as LED bar graphs, a
// ladder each, as a console's meter bridge shows its channels. A ladder's
// lit height is one style property, --lit, the number of segments lit
// (featladder.go); the stylesheet draws the segments and their colors from
// it, so a reading costs one write, and none when it lights the segments it
// already lit.

// featNames are the features the Features module shows, in the order of its
// ladders: the mono mix of each.
var featNames = [6]string{"amp", "bass", "mid", "treble", "centroid", "beat"}

// updateAudioMeters lights the ladders from this frame's features, while the
// module is on screen.
func (a *audioFeatures) updateAudioMeters() {
	if !onScreen.moduleOnScreen("feat-module") {
		return
	}
	for i, nm := range featNames {
		a.setLadder(i, featLit(a.feat[nm]))
	}
}

// darkLadders puts every ladder out.
func (a *audioFeatures) darkLadders() {
	for i := range featNames {
		a.setLadder(i, 0)
	}
}

// setLadder lights n segments of ladder i. The element is looked up again
// only when a write is due and the one held has left the page.
func (a *audioFeatures) setLadder(i, n int) {
	el := a.ladder[i]
	if n == a.ladderLit[i] && el.Truthy() {
		return
	}
	if !el.Truthy() || !el.Get("isConnected").Bool() {
		el = dom.Doc.Call("getElementById", "feat-bar-"+strconv.Itoa(i))
		if !el.Truthy() {
			return
		}
		a.ladder[i] = el
	}
	a.ladderLit[i] = n
	el.Get("style").Call("setProperty", "--lit", strconv.Itoa(n))
}
