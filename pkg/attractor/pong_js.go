//go:build js && wasm

package attractor

import (
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
	pongStrokes    [][]float64 // scratch stroke list for the blanked beam
	pongShownL     = -1        // scores last latched onto the Scoreboard LEDs
	pongShownR     = -1
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

// ── Beam drawing ─────────────────────────────────────────────────────────

// generatePong draws the frame as blanked-beam strokes (beamLines): court
// border, a properly dashed net, detached score marks, paddles, ball — and
// NOTHING between them. The retrace is blanked, as a real scope's z-axis
// would be.
func generatePong() {
	if !pongWired {
		pongWireInput()
	}
	pongStep()
	pongSyncScoreboard()

	ph := float64(pongPaddleH) / 2
	strokes := pongStrokes[:0]
	// Court border (one closed polyline).
	strokes = append(strokes, []float64{
		-pongW, pongH, pongW, pongH, pongW, -pongH, -pongW, -pongH, -pongW, pongH})
	// Net: real dashes down the middle, no zigzag workaround needed.
	for y := pongH - 0.04; y > -pongH; y -= 0.12 {
		strokes = append(strokes, []float64{0, y, 0, y - 0.06})
	}
	// Score marks: detached ticks hanging under the top edge.
	for i := 0; i < pongScoreL; i++ {
		x := -(0.18 + 0.11*float64(i))
		strokes = append(strokes, []float64{x, pongH - 0.03, x, pongH - 0.11})
	}
	for i := 0; i < pongScoreR; i++ {
		x := 0.18 + 0.11*float64(i)
		strokes = append(strokes, []float64{x, pongH - 0.03, x, pongH - 0.11})
	}
	// Paddles: slim closed bars.
	paddle := func(x, pad float64) []float64 {
		const w = 0.016
		return []float64{x - w, pad - ph, x - w, pad + ph, x + w, pad + ph, x + w, pad - ph, x - w, pad - ph}
	}
	strokes = append(strokes, paddle(-pongPadX, pongPadL), paddle(pongPadX, pongPadR))
	// Ball diamond (blinks while serving).
	if pongServe == 0 || pongServe%10 < 5 {
		strokes = append(strokes, []float64{
			pongBX - pongBall, pongBY, pongBX, pongBY + pongBall,
			pongBX + pongBall, pongBY, pongBX, pongBY - pongBall,
			pongBX - pongBall, pongBY})
	}
	pongStrokes = strokes
	if v := beamLines(strokes, 0); v > 0 {
		uploadVerticesOnly(vertBuf[:v*4], beamDrawMode(), v)
	}
}

// pongSyncScoreboard latches the Scoreboard module's LEDs when a score
// changes (cheap check, no DOM writes on quiet frames).
func pongSyncScoreboard() {
	if pongScoreL == pongShownL && pongScoreR == pongShownR {
		return
	}
	pongShownL, pongShownR = pongScoreL, pongScoreR
	if l := doc.Call("getElementById", "pong-score-l"); l.Truthy() {
		l.Set("textContent", strconv.Itoa(pongScoreL))
	}
	if r := doc.Call("getElementById", "pong-score-r"); r.Truthy() {
		r.Set("textContent", strconv.Itoa(pongScoreR))
	}
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
	if sect := doc.Call("getElementById", "pong-module"); sect.Truthy() {
		if mode == "pong" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
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
