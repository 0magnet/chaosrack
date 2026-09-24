//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// Scope Pong — an homage to the glensstuff.com analog Oscilloscope Pong,
// where the whole game is a voltage pair driving a scope's X/Y inputs. The
// court, net, score ticks, paddles and ball are all one continuous beam
// path resampled to the trail length, exactly like an analog multiplexed
// display: strokes that overlap the court outline hide in it, and the two
// unavoidable jumps to the ball read as the faint retrace beams a real
// unblanked scope shows.
//
// Play: W/S drives the left paddle, ↑/↓ the right. A side with no human
// input for ~10 s hands its paddle back to the machine, so the mode boots
// as a self-playing attract demo. First to 9 resets the match. The knobs
// set ball speed, paddle size, and the machine player's skill.

// pongGame is Scope Pong: the ball, the paddles, the score, the keys and the
// audio context it holds.
type pongGame struct {
	ballSpeed float32
	paddleH   float32 // paddle full height
	aiSkill   float32
	bx, by    float64 // ball position
	vx, vy    float64 // ball direction (unit-ish)
	padL      float64
	padR      float64
	scoreL    int
	scoreR    int
	serve     int // frames until serve (pause after a point)
	humanL    int // frames of human control left on each side
	humanR    int
	keyW      bool
	keyS      bool
	keyUp     bool
	keyDn     bool
	wired     bool
	ctxHeld   bool
	strokes   [][]float64 // scratch stroke list for the blanked beam
	shownL    int         // scores last latched onto the Scoreboard LEDs
	shownR    int
	syncTick  int

	// active tracks mode residency so entry setup runs once per entry, not
	// on every panel rebuild (patchbay/template toggles rebuild the panel too).
	active bool
}

var pong = pongGame{
	ballSpeed: 1,
	paddleH:   0.42,
	aiSkill:   0.7,
	vx:        0.6,
	vy:        0.23,
	shownL:    -1,
	shownR:    -1,
}

const (
	pongW    = 1.5  // court half-width
	pongH    = 1.0  // court half-height
	pongPadX = 1.38 // paddle |x|
	pongBall = 0.05 // ball diamond radius
)

// pongStep advances one frame of game state, honoring the Speed control the
// way the integrators do (sub-steps × dt scale).
func (p *pongGame) step() {
	k := float64(speedScale) * float64(speedSteps)
	ph := float64(p.paddleH) / 2

	// Paddles: human while recently touched, machine otherwise.
	move := func(pad *float64, up, dn bool, human *int, aiming bool) {
		if *human > 0 {
			*human--
			// Keys nudge; pointer control (pongPointerPaddle) writes the pad
			// position directly, so with no key held this just clamps.
			d := 0.0
			if up {
				d += 0.032
			}
			if dn {
				d -= 0.032
			}
			*pad += d * k
		} else {
			// Machine: chase the ball when it's incoming, drift home when not.
			target := 0.0
			if aiming {
				target = p.by
			}
			maxSpd := (0.006 + 0.030*float64(p.aiSkill)) * k
			d := target - *pad
			if d > maxSpd {
				d = maxSpd
			}
			if d < -maxSpd {
				d = -maxSpd
			}
			*pad += d
		}
		if *pad > pongH-ph {
			*pad = pongH - ph
		}
		if *pad < -(pongH - ph) {
			*pad = -(pongH - ph)
		}
	}
	move(&p.padL, p.keyW, p.keyS, &p.humanL, p.vx < 0)
	move(&p.padR, p.keyUp, p.keyDn, &p.humanR, p.vx > 0)

	if p.serve > 0 {
		p.serve--
		return
	}

	sp := 0.020 * float64(p.ballSpeed) * k
	p.bx += p.vx * sp
	p.by += p.vy * sp

	// Wall bounce.
	if p.by > pongH-pongBall && p.vy > 0 || p.by < -(pongH-pongBall) && p.vy < 0 {
		p.vy = -p.vy
		p.beep(226, 90)
	}
	// Paddle bounce: reflect at the paddle plane when the ball face covers it,
	// with english from the hit offset and a little speed-up.
	hit := func(pad float64) bool { return math.Abs(p.by-pad) <= ph+pongBall }
	if p.bx > pongPadX-pongBall && p.vx > 0 && hit(p.padR) {
		p.vx = -math.Abs(p.vx) * 1.04
		p.vy += (p.by - p.padR) / ph * 0.35
		p.beep(459, 90)
	}
	if p.bx < -(pongPadX-pongBall) && p.vx < 0 && hit(p.padL) {
		p.vx = math.Abs(p.vx) * 1.04
		p.vy += (p.by - p.padL) / ph * 0.35
		p.beep(459, 90)
	}
	if v := math.Abs(p.vx); v > 1.6 { // keep returns playable
		p.vx = p.vx / v * 1.6
	}
	if v := math.Abs(p.vy); v > 1.2 {
		p.vy = p.vy / v * 1.2
	}

	// Point scored: tick the winner, serve toward the loser.
	if p.bx > pongW+0.25 {
		p.scoreL++
		p.serveBall(-1)
	}
	if p.bx < -(pongW + 0.25) {
		p.scoreR++
		p.serveBall(1)
	}
	if p.scoreL > 9 || p.scoreR > 9 {
		p.scoreL, p.scoreR = 0, 0
	}
}

// pongServeBall re-centers the ball and aims it at dir (±1) after a pause.
func (p *pongGame) serveBall(dir float64) {
	p.bx, p.by = 0, 0
	p.vx = dir * 0.6
	p.vy = (jamRand() - 0.5) * 0.8
	p.serve = 45
	p.beep(490, 220)
}

// ── Beam drawing ─────────────────────────────────────────────────────────

// generatePong draws the frame as blanked-beam strokes (beamLines): court
// border, a properly dashed net, detached score marks, paddles, ball — and
// NOTHING between them. The retrace is blanked, as a real scope's z-axis
// would be.
func (p *pongGame) generatePong() {
	if !p.wired {
		p.wireInput()
	}
	p.step()
	p.syncScoreboard()

	ph := float64(p.paddleH) / 2
	strokes := p.strokes[:0]
	// Court border (one closed polyline).
	strokes = append(strokes, []float64{
		-pongW, pongH, pongW, pongH, pongW, -pongH, -pongW, -pongH, -pongW, pongH})
	// Net: real dashes down the middle, no zigzag workaround needed.
	for y := pongH - 0.04; y > -pongH; y -= 0.12 {
		strokes = append(strokes, []float64{0, y, 0, y - 0.06})
	}
	// Score marks: detached ticks hanging under the top edge.
	for i := 0; i < p.scoreL; i++ {
		x := -(0.18 + 0.11*float64(i))
		strokes = append(strokes, []float64{x, pongH - 0.03, x, pongH - 0.11})
	}
	for i := 0; i < p.scoreR; i++ {
		x := 0.18 + 0.11*float64(i)
		strokes = append(strokes, []float64{x, pongH - 0.03, x, pongH - 0.11})
	}
	// Paddles: slim closed bars.
	paddle := func(x, pad float64) []float64 {
		const w = 0.016
		return []float64{x - w, pad - ph, x - w, pad + ph, x + w, pad + ph, x + w, pad - ph, x - w, pad - ph}
	}
	strokes = append(strokes, paddle(-pongPadX, p.padL), paddle(pongPadX, p.padR))
	// Ball diamond (blinks while serving).
	if p.serve == 0 || p.serve%10 < 5 {
		strokes = append(strokes, []float64{
			p.bx - pongBall, p.by, p.bx, p.by + pongBall,
			p.bx + pongBall, p.by, p.bx, p.by - pongBall,
			p.bx - pongBall, p.by})
	}
	p.strokes = strokes
	if v := beamLines(strokes, 0); v > 0 {
		gpu.uploadVerticesOnly(vertBuf[:v*4], beamDrawMode(), v)
	}
}

// pongSyncScoreboard latches the Scoreboard module's LEDs when a score
// changes, and spins the paddle pots to track the live paddles (motorized
// pots: the machine plays its own knobs) — except a pot the user is
// actually holding, which is theirs.
func (p *pongGame) syncScoreboard() {
	p.syncTick++
	if p.syncTick%3 == 0 { // pot writes throttled — 20 Hz reads smooth
		pongKnobGuard = true
		for _, s := range []struct {
			sl  js.Value
			pad float64
		}{{pongPadSlL, p.padL}, {pongPadSlR, p.padR}} {
			if !s.sl.Truthy() || (kb.active && kb.slider.Equal(s.sl)) {
				continue
			}
			s.sl.Set("value", strconv.FormatFloat(s.pad/pongH, 'f', 2, 64))
			s.sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
		pongKnobGuard = false
	}
	if p.scoreL == p.shownL && p.scoreR == p.shownR {
		return
	}
	p.shownL, p.shownR = p.scoreL, p.scoreR
	if l := dom.Doc.Call("getElementById", "pong-score-l"); l.Truthy() {
		l.Set("textContent", strconv.Itoa(p.scoreL))
	}
	if r := dom.Doc.Call("getElementById", "pong-score-r"); r.Truthy() {
		r.Set("textContent", strconv.Itoa(p.scoreR))
	}
}

// ── Input + sound ────────────────────────────────────────────────────────

// pongWireInput installs the paddle key listeners once (lazily on the first
// generated frame). Handlers no-op outside pong mode. A real keydown is a
// user gesture, so it also lifts the audio context for the beeps.
func (p *pongGame) wireInput() {
	p.wired = true
	set := func(key string, down bool) bool {
		switch key {
		case "w":
			p.keyW = down
			p.humanL = 600
		case "s":
			p.keyS = down
			p.humanL = 600
		case "arrowup":
			p.keyUp = down
			p.humanR = 600
		case "arrowdown":
			p.keyDn = down
			p.humanR = 600
		default:
			return false
		}
		return true
	}
	dom.Doc.Call("addEventListener", "keydown", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		if selectedMode != "pong" {
			return nil
		}
		if t := e.Get("target"); t.Truthy() {
			switch strings.ToLower(t.Get("tagName").String()) {
			case "input", "select", "textarea":
				return nil
			}
		}
		if set(strings.ToLower(e.Get("key").String()), true) {
			e.Call("preventDefault") // arrows must not scroll the page
			if !p.ctxHeld {
				p.ctxHeld = acquireAudioCtx("pong").Truthy()
			}
		}
		return nil
	}))
	dom.Doc.Call("addEventListener", "keyup", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		set(strings.ToLower(a[0].Get("key").String()), false)
		return nil
	}))
}

// pongPointerPaddle drives the paddle on the pointer's half of the screen
// toward the court height under it — mouse or finger, and two fingers play
// both paddles. The touched side goes human (the same ~10 s window the
// keys use) so the machine hands over immediately.
func (p *pongGame) pointerPaddle(cx, cy float64) {
	r := glctx.Canvas.Call("getBoundingClientRect")
	h := r.Get("height").Float()
	if h <= 0 {
		return
	}
	fy := (cy - r.Get("top").Float()) / h
	// A little gain so mid-screen gestures reach the court's corners even
	// when the fitted court doesn't span the full canvas height.
	y := (0.5 - fy) * 2 * pongH * 1.3
	if y > pongH {
		y = pongH
	}
	if y < -pongH {
		y = -pongH
	}
	if cx < r.Get("left").Float()+r.Get("width").Float()/2 {
		p.padL = y
		p.humanL = 600
	} else {
		p.padR = y
		p.humanR = 600
	}
}

// pongBeep plays one classic square blip (hit 459 Hz, wall 226 Hz, point
// 490 Hz) through the shared context. Silent until a real key grants the
// context, and outside pong mode.
func (p *pongGame) beep(freq float64, ms int) {
	if !p.ctxHeld || selectedMode != "pong" {
		return
	}
	ctx := audioCtxRef()
	if !ctx.Truthy() {
		return
	}
	osc := ctx.Call("createOscillator")
	g := ctx.Call("createGain")
	osc.Set("type", "square")
	osc.Get("frequency").Set("value", freq)
	now := ctx.Get("currentTime").Float()
	dur := float64(ms) / 1000
	g.Get("gain").Call("setValueAtTime", 0.08, now)
	g.Get("gain").Call("linearRampToValueAtTime", 0, now+dur)
	osc.Call("connect", g)
	g.Call("connect", ctx.Get("destination"))
	osc.Call("start")
	osc.Call("stop", now+dur+0.01)
}

// syncPongExtras runs on every panel rebuild (from buildParamPanel, beside
// the Graphic Artist switches): entering pong starts a fresh match facing
// the camera with the spin stopped — it's a scope game, not a model;
// leaving drops the beep lease and any held keys.
func (p *pongGame) syncPongExtras(mode string) {
	if sect := dom.Doc.Call("getElementById", "pong-module"); sect.Truthy() {
		if mode == "pong" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if mode == "pong" {
		if p.active {
			return
		}
		p.active = true
		p.scoreL, p.scoreR = 0, 0
		p.padL, p.padR = 0, 0
		p.humanL, p.humanR = 0, 0
		p.serveBall(1)
		normalizeOrientation()
		return
	}
	p.active = false
	p.keyW, p.keyS, p.keyUp, p.keyDn = false, false, false, false
	if p.ctxHeld {
		releaseAudioCtx("pong")
		p.ctxHeld = false
	}
}
