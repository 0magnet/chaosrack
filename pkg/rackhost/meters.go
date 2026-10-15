package rackhost

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The measurement modules, natively: Distortion, Loudness, Wow & Flutter and
// the Counter, reading the rack's signal as the page's read its tap, and
// writing their displays as the page's do (attractor's meterreadouts.go).

// meterBank is the measurement modules and how far into the rack's signal
// they have read.
type meterBank struct {
	thd   *attractor.THDMeter
	lufs  *attractor.LoudnessReadout
	wf    *attractor.WowFlutterMeter
	cnt   attractor.FreqCounter
	wfWin int
	// wfBusy is a wow and flutter reading being taken.
	wfBusy bool
	read   int
	// item is each display's picture item, by its id.
	item map[string]int
}

func newMeterBank() meterBank {
	return meterBank{thd: attractor.NewTHDMeter(), lufs: attractor.NewLoudnessReadout(), wf: attractor.NewWowFlutterMeter()}
}

// findMeters finds the measurement modules' displays, and writes what each
// last said into them. Some lie in the bank's region, so are put up with each
// bank: called with r.mu held whenever one is, as the readouts are found.
func (r *Rack) findMeters() {
	m := &r.meters
	m.item = map[string]int{}
	for i, it := range r.pic.Items {
		if it != nil && strings.HasPrefix(it.Mark, "led ") {
			m.item[strings.TrimPrefix(it.Mark, "led ")] = i
		}
	}
	if m.thd == nil {
		return // the bank is put up before the meters are made
	}
	r.showMeter(m.thd.Readouts())
	r.showMeter(m.lufs.Readouts())
	r.showMeter(m.wf.Readouts())
	r.showMeter(m.cnt.Readouts())
}

// runMeters feeds the meters what the rack's signal has made since they
// last read, and writes what each says when it measures. Called with r.mu
// held, as the panel looks.
func (r *Rack) runMeters(nowMs float64) {
	m := &r.meters
	m.thd.Harm = int(r.number("thd-harm") + 0.5)
	m.thd.PeriodMs = r.number("thd-rate")
	m.lufs.Target = float32(r.number("lufs-target"))
	m.lufs.PeriodMs = r.number("lufs-rate")
	m.wf.Nominal = float32(r.number("wf-nom"))
	m.wf.PeriodMs = r.number("wf-rate")
	if w := int(r.number("wf-win") + 0.5); w != m.wfWin {
		// A new window is a new measurement: nothing of the last one stands.
		m.wfWin, m.wf = w, attractor.NewWowFlutterMeter()
		m.wf.WindowSec = max(w, 1)
	}
	l, rt, next := r.sound.since(m.read)
	m.read = next
	if len(l) == 0 {
		return
	}
	mix := attractor.FoldChan(0, l, rt)
	m.thd.Push(attractor.FoldChan(float32(r.number("thd-chan")), l, rt))
	if m.thd.Measure(nowMs, sampleRate) {
		r.showMeter(m.thd.Readouts())
	}
	m.lufs.Add(l, rt, sampleRate)
	if m.lufs.Measure(nowMs) {
		r.showMeter(m.lufs.Readouts())
	}
	m.wf.Push(mix, sampleRate)
	if !m.wfBusy {
		if x, due := m.wf.Due(nowMs, sampleRate); due {
			// Seconds of signal: read off the panel's turn, as the page reads it
			// in a worker, and written when it is done.
			m.wfBusy = true
			wf, nom := m.wf, float64(m.wf.Nominal)
			go func() {
				res := meters.AnalyzeWowFlutter(x, sampleRate, nom)
				r.mu.Lock()
				defer r.mu.Unlock()
				m.wfBusy = false
				if m.wf == wf { // not a reading of a window since let go
					wf.SetResult(res)
					r.showMeter(wf.Readouts())
				}
			}()
		}
	}
	if m.cnt.Count(mix, r.number("counter-trig")/100, r.number("counter-gatesel"), sampleRate) {
		r.showMeter(m.cnt.Readouts())
	}
}

// showMeter writes what a meter's displays say: the seven-segment font's
// blank sign as a space, and trimmed, as the catalog holds a display's text,
// which it draws right-aligned. Called with r.mu held.
func (r *Rack) showMeter(rs []attractor.Readout) {
	for _, ro := range rs {
		if i, ok := r.meters.item[ro.ID]; ok {
			r.write(i, strings.TrimSpace(strings.ReplaceAll(ro.Text, led.Blank, " ")))
		}
	}
}
