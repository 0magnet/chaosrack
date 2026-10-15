package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The measurement modules' readings, shared by the page (thd_js.go,
// lufs_js.go, wf_js.go, counter_js.go, which say what each instrument is and
// why it reads as it does) and a native rack: what each takes in, when it
// measures, and what each of its displays says. Where the samples come from,
// and how a display is written, are each side's own.

// Readout is what one display says, by the display's id.
type Readout struct{ ID, Text string }

// THDWindow is how much signal one distortion reading is taken over.
const THDWindow = 16384

// THDMeter is the Distortion module: THD, THD+N, SINAD, ENOB, the
// fundamental and the level.
type THDMeter struct {
	PeriodMs float64 // RATE: how often a reading is taken
	Harm     int     // how many harmonics count
	win      meters.SlidingWindow
	buf      []float32
	nextMs   float64
	res      meters.DistortionResult
}

// NewTHDMeter is the Distortion module at the page's defaults.
func NewTHDMeter() *THDMeter { return &THDMeter{PeriodMs: 400, Harm: 10} }

// Push takes in samples, newest last.
func (d *THDMeter) Push(x []float32) {
	d.win.Resize(THDWindow)
	d.win.Push(x)
}

// Measure takes a reading at nowMs, at sample rate sr, if the window is full
// and one is due, and reports whether it did.
func (d *THDMeter) Measure(nowMs float64, sr int) bool {
	if !d.win.Full() || nowMs < d.nextMs {
		return false
	}
	d.nextMs = nowMs + d.PeriodMs
	if len(d.buf) != THDWindow {
		d.buf = make([]float32, THDWindow)
	}
	d.win.Linear(d.buf)
	d.res = meters.AnalyzeDistortion(d.buf, sr, d.Harm)
	return true
}

// Readouts is what the module's displays say.
func (d *THDMeter) Readouts() []Readout { return THDReadouts(d.res) }

// THDReadouts is what the Distortion module's displays say of reading r.
func THDReadouts(r meters.DistortionResult) []Readout {
	if !r.OK {
		return []Readout{
			{"thd-led", "  --.---"}, {"thdn-led", "  --.---"}, {"thd-sinad-led", "  --.-"},
			{"thd-enob-led", "--.--"}, {"thd-fund-led", "-----.-"}, {"thd-level-led", "  --.-"},
		}
	}
	sinad, enob := "  >99.9", ">16.0"
	if r.SINAD < 900 {
		sinad, enob = led.Format(r.SINAD, 3, 1, false), led.Format(r.ENOB, 2, 2, false)
	}
	return []Readout{
		{"thd-led", led.Format(meters.AsPercent(r.THD), 2, 3, false)},
		{"thdn-led", led.Format(meters.AsPercent(r.THDN), 2, 3, false)},
		{"thd-sinad-led", sinad},
		{"thd-enob-led", enob},
		{"thd-fund-led", led.Format(r.Fundamental, 5, 1, false)},
		{"thd-level-led", led.Format(20*math.Log10(math.Max(r.Level, 1e-9)), 3, 1, true)},
	}
}

// LoudnessReadout is the Loudness module: momentary, short-term and
// integrated loudness, the range, the true peak and the distance to target.
type LoudnessReadout struct {
	PeriodMs float64 // RATE
	Target   float32 // TGT, in LUFS
	meter    *meters.LoudnessMeter
	nextMs   float64
	res      meters.LoudnessResult
}

// NewLoudnessReadout is the Loudness module at the page's defaults, its
// readings all at the floor.
func NewLoudnessReadout() *LoudnessReadout {
	l := &LoudnessReadout{PeriodMs: 200, Target: -23}
	l.clear()
	return l
}

func (l *LoudnessReadout) clear() {
	f := meters.LoudnessFloor
	l.res = meters.LoudnessResult{Momentary: f, ShortTerm: f, Integrated: f, TruePeak: f}
}

// Add takes in a stereo stretch at sample rate sr, newest last.
func (l *LoudnessReadout) Add(left, right []float32, sr int) {
	if l.meter == nil {
		l.meter = meters.NewLoudnessMeter(sr)
	} else if l.meter.SampleRate() != sr {
		l.meter.Reset(sr)
	}
	l.meter.Add(left, right)
	for _, ch := range [2][]float32{left, right} {
		if p := meters.TruePeak(ch); p > 0 {
			l.meter.SetTruePeak(p)
		}
	}
}

// Measure takes a reading at nowMs if one is due, and reports whether it
// did.
func (l *LoudnessReadout) Measure(nowMs float64) bool {
	if l.meter == nil || nowMs < l.nextMs {
		return false
	}
	l.nextMs = nowMs + l.PeriodMs
	l.res = l.meter.Result()
	return true
}

// Reset starts the integration again, as the module's RESET does.
func (l *LoudnessReadout) Reset() {
	if l.meter != nil {
		l.meter.Reset(l.meter.SampleRate())
	}
	l.clear()
}

// Readouts is what the module's displays say.
func (l *LoudnessReadout) Readouts() []Readout { return LoudnessReadouts(l.res, l.Target) }

// LoudnessReadouts is what the Loudness module's displays say of reading r,
// against target.
func LoudnessReadouts(r meters.LoudnessResult, target float32) []Readout {
	lufs := func(v float64, ok bool) string {
		if !ok || v <= meters.LoudnessFloor {
			return "  --.-"
		}
		return led.Format(v, 3, 1, true)
	}
	lra, d := "  --.-", "  --.-"
	if r.OK {
		lra = led.Format(r.LRA, 3, 1, false)
		if r.Integrated > meters.LoudnessFloor {
			d = led.Format(r.Integrated-float64(target), 3, 1, true)
		}
	}
	return []Readout{
		{"lufs-m-led", lufs(r.Momentary, true)},
		{"lufs-s-led", lufs(r.ShortTerm, true)},
		{"lufs-i-led", lufs(r.Integrated, r.OK)},
		{"lufs-lra-led", lra},
		{"lufs-tp-led", lufs(r.TruePeak, true)},
		{"lufs-delta-led", d},
	}
}

// WowFlutterMeter is the Wow & Flutter module: speed error, wow, flutter,
// weighted flutter and the carrier.
type WowFlutterMeter struct {
	PeriodMs  float64 // RATE
	WindowSec int     // WINDOW
	Nominal   float32 // NOM: the test tone's frequency
	win       meters.SlidingWindow
	nextMs    float64
	res       meters.WowFlutterResult
}

// NewWowFlutterMeter is the Wow & Flutter module at the page's defaults.
func NewWowFlutterMeter() *WowFlutterMeter {
	return &WowFlutterMeter{PeriodMs: 500, WindowSec: 10, Nominal: meters.WfCarrier}
}

// Push takes in samples at sample rate sr, newest last.
func (w *WowFlutterMeter) Push(x []float32, sr int) {
	w.win.Resize(sr * w.WindowSec)
	w.win.Push(x)
}

// Measure takes a reading at nowMs, at sample rate sr, if one is due, and
// reports whether it did.
func (w *WowFlutterMeter) Measure(nowMs float64, sr int) bool {
	x, ok := w.Due(nowMs, sr)
	if ok {
		w.SetResult(meters.AnalyzeWowFlutter(x, sr, float64(w.Nominal)))
	}
	return ok
}

// Due reports whether a reading is due at nowMs, and the signal to take it
// over, a copy of its own: for a caller that measures elsewhere, as a
// reading over seconds of signal is too long to take on a panel's turn.
func (w *WowFlutterMeter) Due(nowMs float64, sr int) ([]float32, bool) {
	if nowMs < w.nextMs {
		return nil, false
	}
	w.nextMs = nowMs + w.PeriodMs
	w.win.Resize(sr * w.WindowSec)
	x := make([]float32, sr*w.WindowSec)
	return x[:w.win.Linear(x)], true
}

// SetResult is a reading taken elsewhere (Due).
func (w *WowFlutterMeter) SetResult(r meters.WowFlutterResult) { w.res = r }

// Readouts is what the module's displays say.
func (w *WowFlutterMeter) Readouts() []Readout { return WowFlutterReadouts(w.res) }

// WowFlutterReadouts is what the Wow & Flutter module's displays say of
// reading r.
func WowFlutterReadouts(r meters.WowFlutterResult) []Readout {
	pct := func(v float64, signed bool) string {
		if !r.OK {
			return "  --.---"
		}
		return led.Format(v, 2, 3, signed)
	}
	carrier := "-----.-"
	if r.OK {
		carrier = led.Format(r.Carrier, 5, 1, false)
	}
	return []Readout{
		{"wf-speed-led", pct(r.SpeedPct, true)},
		{"wf-wow-led", pct(r.WowPct, false)},
		{"wf-flutter-led", pct(r.FlutterPct, false)},
		{"wf-weighted-led", pct(r.WeightedPct, false)},
		{"wf-carrier-led", carrier},
	}
}

// FreqCounter is the Counter module: cycles counted through a trigger band
// over a gate, and the gate lamp that flips each time one closes.
type FreqCounter struct {
	cycles, samples int
	state           int // +1 above the band, −1 below, 0 unarmed
	// GateLit is the gate lamp.
	GateLit bool
	hz      float64
	read    bool
}

// Count takes in samples at sample rate sr, the band ±trig (in full scale)
// and the gate gateSec long, and reports whether a gate closed on them.
func (f *FreqCounter) Count(x []float32, trig, gateSec float64, sr int) bool {
	th := max(trig, 1e-4) // trig at 0 still needs hysteresis or it chatters
	for _, v := range x {
		s := float64(v)
		switch {
		case s >= th:
			if f.state < 0 {
				f.cycles++ // one full swing = one cycle
			}
			f.state = 1
		case s <= -th:
			f.state = -1 // armed on the first low excursion
		}
	}
	f.samples += len(x)
	if gateSec <= 0 {
		gateSec = 1
	}
	if sr <= 0 || f.samples < int(gateSec*float64(sr)) {
		return false
	}
	f.hz = float64(f.cycles) / (float64(f.samples) / float64(sr))
	f.read = true
	f.cycles, f.samples = 0, 0
	f.GateLit = !f.GateLit
	return true
}

// Readouts is what the module's display says: nothing until a gate closes.
func (f *FreqCounter) Readouts() []Readout {
	if !f.read {
		return nil
	}
	return []Readout{CounterReadout(f.hz)}
}

// CounterReadout is what the Counter's display says of hz.
func CounterReadout(hz float64) Readout { return Readout{"counter-led", led.Format(hz, 5, 1, false)} }
