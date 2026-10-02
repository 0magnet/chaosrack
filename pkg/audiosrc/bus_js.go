//go:build js && wasm

package audiosrc

// Bus is the rack's signal, RACK L and R, as everything that analyzes it
// reads it: a mix of the rack's sources, each at its own gain on each side.
// It is the analysis half of the Mixer (attractor's mixer_js.go); the other
// half, what the speakers play, is made in Web Audio.
//
// The sources made here are the capture, the generators and the model, each
// at a gain on each side (Mix). The instruments that only exist in Web Audio
// — the keys, the drums, the tone matrix, the models' own sounds — come back
// as one stereo stream already mixed (Return), the way a send comes back on a
// return.
//
// They are three kinds of source with three clocks. The capture and the
// return have a backlog: Drain hands over what arrived. The generators and
// the model are made as they are read, so they are made to match: as many
// samples as the capture delivered when it is in the mix, as the return
// delivered when it is and the capture is not, and a full buffer when
// neither is (FuncGen.Drain's contract, which the bus keeps).
type Bus struct {
	Gen *FuncGen

	// Mix is each source's gain on RACK L ([0]) and RACK R ([1]); 0 is not
	// in the mix, and a negative gain inverts it.
	Mix [2][InCount]float32

	// Capture is the capture, made on first need: nil, or a source not yet
	// Ready, reads as silence. Return is the Web Audio sources' sends, and
	// ReturnOn whether anything is sent on it. Model fills dst, n samples of
	// the model's x, y and z together (one step of the model moves all
	// three): the next stretch of its signal when advance is set, and the
	// latest window, at no cost to that signal, otherwise. Nil reads as
	// silence.
	Capture  func() Source
	Return   func() Source
	ReturnOn bool
	Model    func(dst *[3][]float32, n int, advance bool)

	// ModMix is each source's gain on the two sends only modulation reads,
	// MOD A ([0]) and MOD B ([1]), and ModReturn the Web Audio sources' sends
	// to them, as Return is to the rack's. Nothing drains them: modulation
	// reads a window of them a frame (ModWindow), so they keep no clock.
	ModMix      [2][InCount]float32
	ModReturn   func() Source
	ModReturnOn bool

	capL, capR []float32
	retL, retR []float32
	tmpL, tmpR []float32
	model      [3][]float32
	mono       []float32
	osc        [OscCount][]float32
	winL, winR []float32
	modA, modB []float32
	mrtL, mrtR []float32
}

// In is a source the bus mixes.
type In int

// The sources. A generator's is InGen plus its index.
const (
	InCaptureL In = iota
	InCaptureR
	InGen
)

// The model's coordinates, after the generators, and how many sources there are.
const (
	InModelX = InGen + OscCount + In(iota)
	InModelY
	InModelZ
	InCount
)

// GenOf is the generator in is, and whether it is one.
func GenOf(in In) (int, bool) {
	i := int(in - InGen)
	return i, i >= 0 && i < OscCount
}

// NewBus is a bus on f with nothing in its mix.
func NewBus(f *FuncGen) *Bus { return &Bus{Gen: f} }

// on reports whether in is in the mix on either side.
func (b *Bus) on(in In) bool { return uses(&b.Mix, in) }

// captureOn reports whether the capture is in the mix.
func (b *Bus) captureOn() bool { return b.on(InCaptureL) || b.on(InCaptureR) }

// ready is s if it is delivering, else nil.
func ready(f func() Source) Source {
	if f == nil {
		return nil
	}
	if s := f(); s != nil && s.Ready() {
		return s
	}
	return nil
}

// grow makes s at least n long.
func grow(s []float32, n int) []float32 {
	if cap(s) < n {
		return make([]float32, n, n+n/2)
	}
	return s[:n]
}

// mixGains is a matrix of gains, each source's on two sides.
type mixGains = [2][InCount]float32

// uses reports whether mix has in on either side.
func uses(mix *mixGains, in In) bool { return mix[0][in] != 0 || mix[1][in] != 0 }

// render fills l and r, n samples of the mix, with the capture's half
// already in b.capL and b.capR and the return's in b.retL and b.retR. The
// generators and the model move on n samples when advance is set and only
// if the mix has them in it: nothing else listens to that clock.
func (b *Bus) render(l, r []float32, n int, advance bool) {
	b.prepare(&b.Mix, n, advance)
	b.mixInto(l, r, n, &b.Mix, b.retL, b.retR, b.ReturnOn)
}

// prepare makes n samples of each generator and of the model, as far as mix
// uses them: the next n when advance is set, the latest window otherwise.
func (b *Bus) prepare(mix *mixGains, n int, advance bool) {
	gens, model := false, false
	for i := range OscCount {
		gens = gens || uses(mix, InGen+In(i))
	}
	for c := InModelX; c <= InModelZ; c++ {
		model = model || uses(mix, c)
	}
	if gens {
		for i := range OscCount {
			b.osc[i] = grow(b.osc[i], n)
		}
		if advance {
			sr := float64(b.Gen.sr)
			for k := range n {
				for i := range OscCount {
					v := b.Gen.osc[i].step(sr)
					if !b.Gen.Audible(i) {
						v = 0
					}
					b.osc[i][k] = float32(v)
				}
			}
		} else {
			for i := range OscCount {
				if uses(mix, InGen+In(i)) {
					b.Gen.RenderOsc(i, b.osc[i])
				}
			}
		}
	}
	if model {
		b.renderModel(n, advance)
	}
}

// mixInto fills l and r with n samples of mix over the prepared sources, and
// the return retL and retR when retOn.
func (b *Bus) mixInto(l, r []float32, n int, mix *mixGains, retL, retR []float32, retOn bool) {
	for side, dst := range [2][]float32{l[:n], r[:n]} {
		ret := retL
		if side == 1 {
			ret = retR
		}
		if retOn && len(ret) >= n {
			copy(dst, ret[:n])
		} else {
			clear(dst)
		}
		for in := range InCount {
			g := mix[side][in]
			if g == 0 {
				continue
			}
			src := b.source(in)
			if len(src) < n {
				continue
			}
			for k := range n {
				dst[k] += g * src[k]
			}
		}
	}
}

// source is the samples source in has made for this pass.
func (b *Bus) source(in In) []float32 {
	switch {
	case in == InCaptureL:
		return b.capL
	case in == InCaptureR:
		return b.capR
	case in >= InModelX && in <= InModelZ:
		return b.model[in-InModelX]
	}
	g, _ := GenOf(in)
	return b.osc[g]
}

// renderModel fills b.model with n samples of the model's x, y and z.
func (b *Bus) renderModel(n int, advance bool) {
	for c := range b.model {
		b.model[c] = grow(b.model[c], n)
		clear(b.model[c])
	}
	if b.Model != nil {
		b.Model(&b.model, n, advance)
	}
}

// returnRate is the return's sample rate over the bus's: 1 when they agree,
// which is whenever the capture is the microphone (the same audio context)
// or not in the mix at all.
func (b *Bus) returnRate(ret Source) float64 {
	sr := b.SampleRate()
	if rr := ret.SampleRate(); rr > 0 && sr > 0 {
		return float64(rr) / float64(sr)
	}
	return 1
}

// fillReturn puts n samples of the return in b.retL and b.retR, drained when
// drain is set and the latest window otherwise, resampled to the bus's rate.
func (b *Bus) fillReturn(ret Source, n int, drain bool) {
	b.fillFrom(ret, n, drain, &b.retL, &b.retR)
}

// fillFrom puts n samples of ret, a return, in *dl and *dr, as fillReturn
// does the rack's.
func (b *Bus) fillFrom(ret Source, n int, drain bool, dl, dr *[]float32) {
	*dl, *dr = grow(*dl, n), grow(*dr, n)
	rate := b.returnRate(ret)
	m := max(1, int(float64(n)*rate+0.5))
	b.tmpL, b.tmpR = grow(b.tmpL, m), grow(b.tmpR, m)
	got := m
	if drain {
		got = ret.DrainStereo(b.tmpL, b.tmpR)
	} else {
		ret.TimeDomainStereo(b.tmpL, b.tmpR)
	}
	resample(*dl, b.tmpL[:got], m)
	resample(*dr, b.tmpR[:got], m)
}

// resample stretches src, which should have been want samples long, over
// dst by linear interpolation; what it is short of want is silence at the
// start, so the newest sample stays last.
func resample(dst, src []float32, want int) {
	if len(src) == 0 || len(dst) == 0 {
		clear(dst)
		return
	}
	if len(src) == len(dst) {
		copy(dst, src)
		return
	}
	pad := want - len(src)
	step := float64(want) / float64(len(dst))
	for k := range dst {
		x := float64(k)*step - float64(pad)
		if x < 0 {
			dst[k] = 0
			continue
		}
		i := int(x)
		if i >= len(src)-1 {
			dst[k] = src[len(src)-1]
			continue
		}
		f := float32(x - float64(i))
		dst[k] = src[i]*(1-f) + src[i+1]*f
	}
}

// DrainStereo is the mix since the last call: what the capture delivered,
// the return resampled to match, and the generators and the model made to
// match.
func (b *Bus) DrainStereo(l, r []float32) int {
	if len(l) != len(r) {
		panic("audiosrc: DrainStereo requires len(l) == len(r)")
	}
	n := len(l)
	b.capL, b.capR = grow(b.capL, n), grow(b.capR, n)
	capOn := b.captureOn()
	var ret Source
	if b.ReturnOn {
		ret = ready(b.Return)
	}
	switch s := ready(b.Capture); {
	case capOn && s != nil:
		n = s.DrainStereo(b.capL, b.capR)
		b.capL, b.capR = b.capL[:n], b.capR[:n]
	case capOn:
		clear(b.capL)
		clear(b.capR)
	case ret != nil:
		// The return is the clock: the generators make as many samples
		// as it delivered.
		b.retL, b.retR = grow(b.retL, n), grow(b.retR, n)
		n = ret.DrainStereo(b.retL, b.retR)
		b.retL, b.retR = b.retL[:n], b.retR[:n]
		b.render(l, r, n, true)
		return n
	}
	if ret != nil {
		b.fillReturn(ret, n, true)
	} else {
		b.retL, b.retR = grow(b.retL, n), grow(b.retR, n)
		clear(b.retL)
		clear(b.retR)
	}
	b.render(l, r, n, true)
	return n
}

// TimeDomainStereo is the mix's latest window: the capture's and the
// return's, and the generators and the model made fresh.
func (b *Bus) TimeDomainStereo(l, r []float32) {
	b.window(l, r, len(l))
}

// window fills l and r with the latest n samples of the mix, moving nothing
// on.
func (b *Bus) window(l, r []float32, n int) {
	b.capL, b.capR = grow(b.capL, n), grow(b.capR, n)
	if s := ready(b.Capture); s != nil && b.captureOn() {
		s.TimeDomainStereo(b.capL, b.capR)
	} else {
		clear(b.capL)
		clear(b.capR)
	}
	if ret := ready(b.Return); ret != nil && b.ReturnOn {
		b.fillReturn(ret, n, false)
	} else {
		b.retL, b.retR = grow(b.retL, n), grow(b.retR, n)
		clear(b.retL)
		clear(b.retR)
	}
	b.render(l, r, n, false)
}

// Drain is the mix as mono, since the last call.
func (b *Bus) Drain(dst []float32) int {
	b.mono = grow(b.mono, len(dst))
	r := b.mono
	n := b.DrainStereo(dst, r)
	for k := range n {
		dst[k] = (dst[k] + r[k]) / 2
	}
	return n
}

// TimeDomain is the mix's latest window, as mono.
func (b *Bus) TimeDomain(dst []float32) []float32 {
	b.mono = grow(b.mono, len(dst))
	r := b.mono
	b.TimeDomainStereo(dst, r)
	for k := range dst {
		dst[k] = (dst[k] + r[k]) / 2
	}
	return dst
}

// BusWindow is the latest stretch of the rack's signal and the capture, for
// a display that draws them rather than analyzes them.
type BusWindow struct {
	L, R       []float32 // RACK L and R
	CapL, CapR []float32 // the capture, whether or not it is in the mix
}

// Window is the latest n samples of the mix and of the capture, moving
// nothing on: the capture's and the return's latest windows, each generator
// drawn from its shadow (FuncGen.RenderOsc) and the model's latest stretch.
// The slices are the bus's own, rewritten by the next call.
func (b *Bus) Window(n int) BusWindow {
	b.winL, b.winR = grow(b.winL, n), grow(b.winR, n)
	b.window(b.winL, b.winR, n)
	// The capture is drawn even when the mix leaves it out: a scope on CAP
	// L is a probe on the input, wherever the Mixer sends it.
	if s := ready(b.Capture); s != nil && !b.captureOn() {
		s.TimeDomainStereo(b.capL, b.capR)
	}
	return BusWindow{L: b.winL, R: b.winR, CapL: b.capL, CapR: b.capR}
}

// SampleRate is the capture's while it is in the mix and delivering, and
// the generators', which follow it, otherwise.
func (b *Bus) SampleRate() int {
	if s := ready(b.Capture); s != nil && b.captureOn() && s.SampleRate() > 0 {
		b.Gen.sr = s.SampleRate()
	} else if b.Gen.sr <= 0 {
		b.Gen.sr = 48000
	}
	return b.Gen.sr
}

// Channels is two: RACK L and R.
func (b *Bus) Channels() int { return 2 }

// captureOnly reports whether the capture is all there is in the mix.
func (b *Bus) captureOnly() bool {
	if !b.captureOn() || b.ReturnOn {
		return false
	}
	for in := InGen; in < InCount; in++ {
		if b.on(in) {
			return false
		}
	}
	return true
}

// Ready is false only while the mix is the capture alone and the capture is
// not yet delivering: anything else in it is made here, and ready now.
func (b *Bus) Ready() bool {
	if b.captureOnly() {
		return ready(b.Capture) != nil
	}
	return true
}

// Err is the capture's, while the mix is the capture alone.
func (b *Bus) Err() error {
	if b.Capture == nil || !b.captureOnly() {
		return nil
	}
	if s := b.Capture(); s != nil {
		return s.Err()
	}
	return nil
}

// Close does nothing: the capture outlives a change of mix.
func (b *Bus) Close() {}

// Feeds reports whether generator i is in the mix.
func (b *Bus) Feeds(i int) bool { return b.on(InGen + In(i)) }

// SweepPosition reports how far through one pass of the log sweep the first
// generator in the mix playing it, and let through, is, 0..1 — what an
// impulse-response measurement needs to know which frequency it is looking
// at, and when a pass has come round. 0 when none is: a position that never
// wraps, which the measurement reads as no sweep of ours running.
func (b *Bus) SweepPosition() float64 {
	for i := range OscCount {
		o := &b.Gen.osc[i]
		if b.Feeds(i) && o.wave == WaveSweep && o.amp > 0 && !o.off && b.Gen.Audible(i) {
			return o.stim.SweepPosition()
		}
	}
	return 0
}

// ModWindow is the latest n samples of MOD A and MOD B, the sends only
// modulation reads, moving nothing on: the capture's and the return's
// latest windows, the generators from their shadows and the model's latest
// stretch, mixed at ModMix's gains. The slices are the bus's own, rewritten
// by the next call.
func (b *Bus) ModWindow(n int) (a, c []float32) {
	b.modA, b.modB = grow(b.modA, n), grow(b.modB, n)
	b.capL, b.capR = grow(b.capL, n), grow(b.capR, n)
	if s := ready(b.Capture); s != nil && (uses(&b.ModMix, InCaptureL) || uses(&b.ModMix, InCaptureR)) {
		s.TimeDomainStereo(b.capL, b.capR)
	} else {
		clear(b.capL)
		clear(b.capR)
	}
	retOn := false
	if ret := ready(b.ModReturn); ret != nil && b.ModReturnOn {
		b.fillFrom(ret, n, false, &b.mrtL, &b.mrtR)
		retOn = true
	}
	b.prepare(&b.ModMix, n, false)
	b.mixInto(b.modA, b.modB, n, &b.ModMix, b.mrtL, b.mrtR, retOn)
	return b.modA, b.modB
}

// ModOn reports whether anything is on either send.
func (b *Bus) ModOn() bool {
	if b.ModReturnOn {
		return true
	}
	for in := range InCount {
		if uses(&b.ModMix, in) {
			return true
		}
	}
	return false
}
