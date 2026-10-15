package rackhost

import (
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/analysis"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/recurrence"
)

// The model's readout line: the page's live displays beside a model's knobs —
// a scope's correlation, Takens' measured τ, the transfer function's delay,
// the recurrence plot's RQA, the waterfall's RT60 — written as the page
// writes them, from the same measurements.

// readoutLine is where the bank on show keeps its live displays.
type readoutLine struct {
	// item is each display's picture item, by the label beside it.
	item map[string]int
	// btns are the line's ↻ buttons, which measure again.
	btns []int
	// corr is the scope's correlation readout this frame.
	corr string
	// rqa is the recurrence plot's RR, DET and LAM, measured on the page's
	// clock and only when the matrix has moved since.
	rqa      [3]string
	rqaNext  float64
	recDirty bool
	// trend is the RQA's record for the TREND chart on the monitor
	// (trend.go), and rqaRes the last measurement it was fed.
	trend  rqaTrend
	rqaRes recurrence.RQAResult
	lyap   lyapProbe
}

// findReadouts finds the bank's live displays: each is a part the page marks
// as a model readout, filled, after its label. Called with r.mu held, when a
// bank is put up.
func (r *Rack) findReadouts() {
	ro := &r.analyzers.ro
	ro.item = map[string]int{}
	ro.btns = ro.btns[:0]
	label := ""
	for i := r.outside; i < len(r.pic.Items); i++ {
		it := r.pic.Items[i]
		if strings.HasPrefix(it.Mark, "btn ") {
			ro.btns = append(ro.btns, i)
			continue
		}
		switch it.Mark {
		case "mro":
		default:
			if strings.HasPrefix(it.Mark, "led ") {
				// A display with an id of its own: the Analysis readout's, on
				// every bank, and a model's own, as the morph's.
				ro.item[strings.TrimPrefix(it.Mark, "led ")] = i
				label = "" // its label is its own
			}
			continue
		}
		if it.Fill == nil {
			label = it.Text
			continue
		}
		if label != "" {
			ro.item[label] = i
			label = ""
		}
	}
	ro.corr = "r --"
	ro.rqa = [3]string{"--.-", "---", "---"}
}

// showReadouts writes model key's live displays. Called with r.mu held,
// after the frame's measurements.
func (r *Rack) showReadouts(key string, nowMs float64) {
	a := &r.analyzers
	ro := &a.ro
	put := func(label, s string) {
		if i, ok := ro.item[label]; ok {
			r.write(i, s)
		}
	}
	r.showLyapunov(key, nowMs, put)
	switch key {
	case "sprottmorph":
		put("smorph-led", a.morph.run.Label())
	case "stlfile":
		if r.stl.label != "" {
			put("stlfile-led", r.stl.label)
		}
	case "stereo", "xy":
		put("corr", ro.corr)
	case "takens":
		put("meas", a.tau.Text(float32(r.number("takens-tau"))))
	case "xfer":
		put("dly", a.xfer.DelayText())
	case "bifurcation":
		put("cur", a.bif.cur)
	case "waterfall":
		put("rt60", a.wfall.w.RT60Text())
	case "recurrence":
		if nowMs >= ro.rqaNext {
			// A scan of the whole matrix for three numbers a person reads a
			// few times a second: on the page's clock, as the page runs it,
			// and only when the matrix has moved. The trend takes a reading
			// every tick regardless: its axis is the clock.
			ro.rqaNext = nowMs + recurrence.RQASamplePeriodMs
			if ro.recDirty {
				ro.recDirty = false
				ro.rqaRes, ro.rqa = a.rec.MeasureRQA()
			}
			ro.trend.sample(nowMs, a.rec.RQASeam(float32(r.number("takens-tau")), a.lastFlow), ro.rqaRes)
		}
		for i, l := range []string{"rr", "det", "lam"} {
			put(l, ro.rqa[i])
		}
	}
}

// measureTau is the Takens mode's one-shot measurement of τ, which the page
// runs the first time the mode has a full window: it turns the τ knob to what
// it finds, unless somebody chose a τ. Called with r.mu held.
func (r *Rack) measureTau() {
	tau, ch := float32(r.number("takens-tau")), float32(r.number("takens-chan"))
	l, rt := r.sound.window(attractor.TakensEstMax)
	if t, ok := r.analyzers.tau.AutoMeasure(tau, ch, attractor.FoldChan(ch, l, rt), sampleRate); ok {
		_ = r.set("takens-tau", strconv.FormatFloat(float64(t), 'f', -1, 32)) //nolint:errcheck // within the knob's reach by construction
	}
}

// lyapProbe is the Analysis readout: the largest Lyapunov exponent of the
// model on show, measured on demand as the page measures it — a few hundred
// thousand integration steps, nothing once and a frame killer every frame.
type lyapProbe struct {
	want, have string  // the model and constants to measure, and measured
	mode       string  // the model want is of
	due        float64 // when to measure want: after a knob has held still
	val        string
	verdict    string
	cache      map[string][2]string // by model and constants
}

// showLyapunov measures model key's exponent once its constants have held
// still, and writes it and its verdict through put. Called with r.mu held.
func (r *Rack) showLyapunov(key string, nowMs float64, put func(label, s string)) {
	lp := &r.analyzers.ro.lyap
	var b strings.Builder
	b.WriteString(key)
	for _, p := range dynamics.Params(key) {
		if p.Value != nil {
			b.WriteString("|" + r.value(p.ID))
		}
	}
	if k := b.String(); k != lp.want {
		// The page's debounce: a model change waits for its panel, and a knob
		// drag fires a change per pixel.
		delay := 400.0
		if key != lp.mode {
			delay = 120
		}
		lp.want, lp.mode, lp.due = k, key, nowMs+delay
		lp.val, lp.verdict = "measuring…", ""
	}
	if lp.have != lp.want && nowMs >= lp.due {
		lp.have = lp.want
		t, ok := lp.cache[lp.want]
		if !ok {
			t[0], t[1] = attractor.LyapunovTexts(analysis.LyapunovFor(key))
			// Not the two whose system is more than their knobs: Custom's is
			// the equations typed into it, and the morph reprograms itself as
			// it runs.
			if key != "custom" && key != "sprottmorph" {
				if lp.cache == nil {
					lp.cache = map[string][2]string{}
				}
				lp.cache[lp.want] = t
			}
		}
		lp.val, lp.verdict = t[0], t[1]
	}
	put("lyap-led", lp.val)
	put("lyap-verdict", lp.verdict)
}

// remeasure is a ↻ on the readout line: Takens' measures τ again whatever is
// set, and the Lyapunov readout measures the model again, as the page's do.
// Called with r.mu held.
func (r *Rack) remeasure(id string) bool {
	a := &r.analyzers
	switch id {
	case "takens-remeasure":
		ch := float32(r.number("takens-chan"))
		l, rt := r.sound.window(attractor.TakensEstMax)
		if t, ok := a.tau.Measure(attractor.FoldChan(ch, l, rt), sampleRate); ok {
			_ = r.set("takens-tau", strconv.FormatFloat(float64(t), 'f', -1, 32)) //nolint:errcheck // within the knob's reach by construction
		}
		return true
	case "lufs-reset":
		// Loudness's RESET: the integration starts again.
		r.meters.lufs.Reset()
		r.showMeter(r.meters.lufs.Readouts())
		return true
	case "lyap-remeasure":
		lp := &a.ro.lyap
		lp.have, lp.due = "", 0
		lp.val, lp.verdict = "measuring…", ""
		return true
	}
	return false
}
