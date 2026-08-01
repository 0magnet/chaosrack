//go:build js && wasm

package attractor

import (
	"math"
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

var (
	pongBallSpeed float32 = 1
	pongPaddleH   float32 = 0.42 // paddle full height
	pongAISkill   float32 = 0.7
)

const (
	pongW    = 1.5  // court half-width
	pongH    = 1.0  // court half-height
	pongPadX = 1.38 // paddle |x|
	pongBall = 0.05 // ball diamond radius
)

var (
	pongBX, pongBY float64             // ball position
	pongVX, pongVY float64 = 0.6, 0.23 // ball direction (unit-ish)
	pongPadL       float64
	pongPadR       float64
	pongScoreL     int
	pongScoreR     int
	pongServe      int // frames until serve (pause after a point)
	pongHumanL     int // frames of human control left on each side
	pongHumanR     int
	pongKeyW       bool
	pongKeyS       bool
	pongKeyUp      bool
	pongKeyDn      bool
	pongWired      bool
	pongCtxHeld    bool
	pongPath       []float64 // scratch waypoint buffer (x,y pairs)
)

// pongStep advances one frame of game state, honoring the Speed control the
// way the integrators do (sub-steps × dt scale).
func pongStep() {
	k := float64(speedScale) * float64(speedSteps)
	ph := float64(pongPaddleH) / 2

	// Paddles: human while recently touched, machine otherwise.
	move := func(pad *float64, up, dn bool, human *int, aiming bool) {
		if *human > 0 {
			*human--
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
				target = pongBY
			}
			maxSpd := (0.006 + 0.030*float64(pongAISkill)) * k
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
	move(&pongPadL, pongKeyW, pongKeyS, &pongHumanL, pongVX < 0)
	move(&pongPadR, pongKeyUp, pongKeyDn, &pongHumanR, pongVX > 0)

	if pongServe > 0 {
		pongServe--
		return
	}

	sp := 0.020 * float64(pongBallSpeed) * k
	pongBX += pongVX * sp
	pongBY += pongVY * sp

	// Wall bounce.
	if pongBY > pongH-pongBall && pongVY > 0 || pongBY < -(pongH-pongBall) && pongVY < 0 {
		pongVY = -pongVY
		pongBeep(226, 90)
	}
	// Paddle bounce: reflect at the paddle plane when the ball face covers it,
	// with english from the hit offset and a little speed-up.
	hit := func(pad float64) bool { return math.Abs(pongBY-pad) <= ph+pongBall }
	if pongBX > pongPadX-pongBall && pongVX > 0 && hit(pongPadR) {
		pongVX = -math.Abs(pongVX) * 1.04
		pongVY += (pongBY - pongPadR) / ph * 0.35
		pongBeep(459, 90)
	}
	if pongBX < -(pongPadX-pongBall) && pongVX < 0 && hit(pongPadL) {
		pongVX = math.Abs(pongVX) * 1.04
		pongVY += (pongBY - pongPadL) / ph * 0.35
		pongBeep(459, 90)
	}
	if v := math.Abs(pongVX); v > 1.6 { // keep returns playable
		pongVX = pongVX / v * 1.6
	}
	if v := math.Abs(pongVY); v > 1.2 {
		pongVY = pongVY / v * 1.2
	}

	// Point scored: tick the winner, serve toward the loser.
	if pongBX > pongW+0.25 {
		pongScoreL++
		pongServeBall(-1)
	}
	if pongBX < -(pongW + 0.25) {
		pongScoreR++
		pongServeBall(1)
	}
	if pongScoreL > 9 || pongScoreR > 9 {
		pongScoreL, pongScoreR = 0, 0
	}
}

// pongServeBall re-centers the ball and aims it at dir (±1) after a pause.
func pongServeBall(dir float64) {
	pongBX, pongBY = 0, 0
	pongVX = dir * 0.6
	pongVY = (jamRand() - 0.5) * 0.8
	pongServe = 45
	pongBeep(490, 220)
}

// ── Beam path ────────────────────────────────────────────────────────────

func pongMoveTo(x, y float64) { pongPath = append(pongPath, x, y) }

// generatePong builds the frame's single-stroke beam tour — border, score
// ticks, net, paddles, ball — and resamples it by arc length into the trail
// buffer, so beam speed is uniform along the stroke like a real XY scope
// fed a multiplexed drawing signal.
func generatePong() {
	if !pongWired {
		pongWireInput()
	}
	pongStep()

	ph := float64(pongPaddleH) / 2
	pongPath = pongPath[:0]
	// Court border, a closed loop from top-center.
	pongMoveTo(0, pongH)
	pongMoveTo(pongW, pongH)
	pongMoveTo(pongW, -pongH)
	pongMoveTo(-pongW, -pongH)
	pongMoveTo(-pongW, pongH)
	pongMoveTo(0, pongH)
	// Left score ticks: walk out along the top edge (overdrawing the border,
	// so only the dips show) and back.
	tick := func(sign float64, n int) {
		for i := 0; i < n; i++ {
			x := sign * (0.18 + 0.11*float64(i))
			pongMoveTo(x, pongH)
			pongMoveTo(x, pongH-0.08)
			pongMoveTo(x, pongH)
		}
		pongMoveTo(0, pongH)
	}
	tick(-1, pongScoreL)
	// Net: a tight zigzag down the middle — reads as the dashed net without
	// needing beam blanking.
	zig := 0.018
	for y := pongH; y > -pongH; y -= 0.1 {
		pongMoveTo(zig, y-0.05)
		zig = -zig
		pongMoveTo(0, y-0.1)
	}
	// Along the bottom edge to the left paddle's column, then the paddle as
	// a slim outlined bar.
	paddle := func(x, pad float64) {
		w := 0.016
		pongMoveTo(x-w, pad-ph)
		pongMoveTo(x-w, pad+ph)
		pongMoveTo(x+w, pad+ph)
		pongMoveTo(x+w, pad-ph)
		pongMoveTo(x-w, pad-ph)
	}
	pongMoveTo(-pongPadX, -pongH)
	paddle(-pongPadX, pongPadL)
	// Retrace beam to the ball (visible — authentic unblanked-scope jump),
	// drawn as a diamond so it reads at any trail density.
	if pongServe == 0 || pongServe%10 < 5 { // blink while serving
		pongMoveTo(pongBX-pongBall, pongBY)
		pongMoveTo(pongBX, pongBY+pongBall)
		pongMoveTo(pongBX+pongBall, pongBY)
		pongMoveTo(pongBX, pongBY-pongBall)
		pongMoveTo(pongBX-pongBall, pongBY)
	}
	// Retrace to the right paddle, up to the top edge, and the right score
	// ticks on the walk back to top-center.
	paddle(pongPadX, pongPadR)
	pongMoveTo(pongPadX, pongH)
	tick(1, pongScoreR)

	// Resample the tour by arc length into the trail buffer.
	n := len(pongPath) / 2
	vertices := vertBuf[:steps*4]
	total := 0.0
	for i := 1; i < n; i++ {
		dx := pongPath[i*2] - pongPath[i*2-2]
		dy := pongPath[i*2+1] - pongPath[i*2-1]
		total += math.Hypot(dx, dy)
	}
	if total <= 0 || steps < 2 {
		return
	}
	invN := float32(1) / float32(steps-1)
	seg := 1
	segStart := 0.0
	segLen := math.Hypot(pongPath[2]-pongPath[0], pongPath[3]-pongPath[1])
	for i := 0; i < steps; i++ {
		s := total * float64(i) / float64(steps-1)
		for s > segStart+segLen && seg < n-1 {
			segStart += segLen
			seg++
			segLen = math.Hypot(pongPath[seg*2]-pongPath[seg*2-2], pongPath[seg*2+1]-pongPath[seg*2-1])
		}
		f := 0.0
		if segLen > 0 {
			f = (s - segStart) / segLen
		}
		x := pongPath[seg*2-2] + (pongPath[seg*2]-pongPath[seg*2-2])*f
		y := pongPath[seg*2-1] + (pongPath[seg*2+1]-pongPath[seg*2-1])*f
		j := i * 4
		vertices[j] = float32(x)
		vertices[j+1] = float32(y)
		vertices[j+2] = 0
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

// ── Input + sound ────────────────────────────────────────────────────────

// pongWireInput installs the paddle key listeners once (lazily on the first
// generated frame). Handlers no-op outside pong mode. A real keydown is a
// user gesture, so it also lifts the audio context for the beeps.
func pongWireInput() {
	pongWired = true
	set := func(key string, down bool) bool {
		switch key {
		case "w":
			pongKeyW = down
			pongHumanL = 600
		case "s":
			pongKeyS = down
			pongHumanL = 600
		case "arrowup":
			pongKeyUp = down
			pongHumanR = 600
		case "arrowdown":
			pongKeyDn = down
			pongHumanR = 600
		default:
			return false
		}
		return true
	}
	doc.Call("addEventListener", "keydown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
			if !pongCtxHeld {
				pongCtxHeld = acquireAudioCtx("pong").Truthy()
			}
		}
		return nil
	}))
	doc.Call("addEventListener", "keyup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		set(strings.ToLower(a[0].Get("key").String()), false)
		return nil
	}))
}

// pongBeep plays one classic square blip (hit 459 Hz, wall 226 Hz, point
// 490 Hz) through the shared context. Silent until a real key grants the
// context, and outside pong mode.
func pongBeep(freq float64, ms int) {
	if !pongCtxHeld || selectedMode != "pong" {
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

// pongActive tracks mode residency so entry setup runs once per entry, not
// on every panel rebuild (patchbay/template toggles rebuild the panel too).
var pongActive bool

// syncPongExtras runs on every panel rebuild (from buildParamPanel, beside
// the Graphic Artist switches): entering pong starts a fresh match facing
// the camera with the spin stopped — it's a scope game, not a model;
// leaving drops the beep lease and any held keys.
func syncPongExtras(mode string) {
	if mode == "pong" {
		if pongActive {
			return
		}
		pongActive = true
		pongScoreL, pongScoreR = 0, 0
		pongPadL, pongPadR = 0, 0
		pongHumanL, pongHumanR = 0, 0
		pongServeBall(1)
		normalizeOrientation()
		return
	}
	pongActive = false
	pongKeyW, pongKeyS, pongKeyUp, pongKeyDn = false, false, false, false
	if pongCtxHeld {
		releaseAudioCtx("pong")
		pongCtxHeld = false
	}
}
