package rackhost

import (
	"strconv"
	"strings"
)

// The Visual head's programmable switches, model-sw-0..2 beside Screen:
// what each does for the model on show, and the legend over it that says so,
// as the page programs them (modelparts_js.go).

// modelSwitchCount is how many programmable switches the head has.
const modelSwitchCount = 3

// modelSwitch is what one programmable switch does for a model: a latching
// one stands where on says and is thrown by set; a momentary one presses
// and springs back.
type modelSwitch struct {
	legend string
	on     func() bool
	set    func(bool)
	press  func()
}

// modelSwitches is what the head's switches do for model, in order. Called
// with r.mu and the sound's lock held.
func (r *Rack) modelSwitches(model string) []modelSwitch {
	s := r.sound
	g := &s.games
	switch model {
	case "custom":
		return r.customSwitches()
	case "pong":
		return []modelSwitch{{legend: "rset", press: g.pong.Restart}}
	case "bounceball":
		return []modelSwitch{{legend: "drop", press: g.ball.Drop}}
	case "recurrence":
		a := &r.analyzers
		return []modelSwitch{{legend: "trnd", on: func() bool { return !a.trendOff }, set: func(v bool) { a.trendOff = !v }}}
	case "stlfile":
		return []modelSwitch{{legend: "load", press: func() { r.prompt = stlPath }}}
	case "fvf":
		return []modelSwitch{
			{legend: "fx", on: func() bool { return !s.wob.Bypassed() }, set: func(v bool) { s.wob.SetBypass(!v) }},
			{legend: "lstn", on: func() bool { return s.listen.on }, set: func(v bool) {
				s.listen.on = v
				if v {
					s.listen.proc = s.wob.Processor(sampleRate)
				}
			}},
			// The page asks its server to route the machine's audio through
			// the wobbulator; a rack with no server has nothing to ask, so
			// the switch stays down, as the page's does on a server that
			// cannot route.
			{legend: "rout", on: func() bool { return false }, set: func(bool) {}},
		}
	}
	return nil
}

// modelSwitchSet throws model switch id as its control now stands. Called
// with r.mu and the sound's lock held.
func (r *Rack) modelSwitchSet(id string) (more [][2]string) {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "model-sw-"))
	if err != nil {
		return nil
	}
	down := r.value(id) == "1"
	sws := r.modelSwitches(r.model())
	if n >= len(sws) {
		// A switch the model does not use stays down.
		if down {
			more = append(more, [2]string{id, "0"})
		}
		return more
	}
	sw := sws[n]
	if sw.press != nil {
		// Momentary: it does its part and springs back.
		if down {
			sw.press()
			more = append(more, [2]string{id, "0"})
		}
		return more
	}
	sw.set(down)
	// Every latching switch stands where its state does now: this one, if it
	// would not go, and the others, which one switch can move (Custom's
	// flavors are one at a time).
	for i, o := range sws {
		if o.on == nil {
			continue
		}
		sid := "model-sw-" + strconv.Itoa(i)
		if want := map[bool]string{true: "1", false: "0"}[o.on()]; r.value(sid) != want {
			more = append(more, [2]string{sid, want})
		}
	}
	return more
}

// showModelSwitches programs the head's switches for model: each legend says
// what its switch does, and a latching one stands where its state does.
// Called with r.mu held, when the model changes.
func (r *Rack) showModelSwitches(model string) {
	s := r.sound
	s.mu.Lock()
	sws := r.modelSwitches(model)
	stands := make([]string, modelSwitchCount)
	legends := make([]string, modelSwitchCount)
	for i := range modelSwitchCount {
		stands[i] = "0"
		if i < len(sws) {
			legends[i] = sws[i].legend
			if sws[i].on != nil && sws[i].on() {
				stands[i] = "1"
			}
		}
	}
	s.mu.Unlock()
	for i := range modelSwitchCount {
		id := "model-sw-" + strconv.Itoa(i)
		if j, ok := r.legendItem[id]; ok {
			r.write(j, legends[i])
		}
		if k, ok := r.byID[id]; ok && r.ctls[k].Value != stands[i] {
			r.ctls[k].Value = stands[i]
			r.turn(&r.ctls[k])
		}
	}
}

// findLegends finds each programmable switch's legend: the display at the
// top of its box. Called with r.mu held, once.
func (r *Rack) findLegends() {
	r.legendItem = map[string]int{}
	for i := range modelSwitchCount {
		id := "model-sw-" + strconv.Itoa(i)
		b, ok := r.pic.Ctls[id]
		if !ok {
			continue
		}
		for j, it := range r.pic.Items[:r.outside] {
			if it == nil || it.Fill == nil || it.Angle != nil {
				continue
			}
			if abs(it.X-b[0]) < 1 && abs(it.Y-b[1]) < 1 && it.H < b[3]*0.7 {
				r.legendItem[id] = j
				break
			}
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// listenInto plays the capture through the wobbulator into fx, n samples,
// while FVF's Listen switch is up. Called with the sound's lock held.
func (s *sound) listenInto(fx []float32, n int) {
	p := s.listen.proc
	for j := range n {
		fx[j] += p.Process((s.capL[j] + s.capR[j]) * 0.5)
	}
}

// listening reports whether FVF's Listen is playing: its switch up, with
// FVF on show. Called with the sound's lock held.
func (s *sound) listening() bool {
	return s.listen.on && s.listen.shown && s.listen.proc != nil
}
