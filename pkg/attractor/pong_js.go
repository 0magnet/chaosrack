//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/scopegames"
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
	scopegames.Pong
	potL    float32 // the paddle pots, as the bank knobs read (motorized: see syncScoreboard)
	potR    float32
	wired   bool
	ctxHeld bool
	strokes [][]float64 // scratch stroke list for the blanked beam
	shownL  int         // scores last latched onto the Scoreboard LEDs
	shownR  int
	// syncTick throttles the pot writes.
	syncTick int

	// active tracks mode residency so entry setup runs once per entry, not
	// on every panel rebuild (patchbay/template toggles rebuild the panel too).
	active bool
}

var pong = pongGame{Pong: scopegames.NewPong(jamRand), shownL: -1, shownR: -1}

func init() { pong.Beep = pong.beep }

// pongH is the court's half-height.
const pongH = scopegames.PongH

// ── Beam drawing ─────────────────────────────────────────────────────────

// generatePong draws the frame as blanked-beam strokes (beamLines): court
// border, a properly dashed net, detached score marks, paddles, ball — and
// NOTHING between them. The retrace is blanked, as a real scope's z-axis
// would be.
func (p *pongGame) generatePong() {
	if !p.wired {
		p.wireInput()
	}
	p.Step(float64(sim.speedScale) * float64(sim.speedSteps))
	p.syncScoreboard()

	p.strokes = p.Strokes(p.strokes[:0])
	if v := beamLines(p.strokes, 0); v > 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:v*4], beamDrawMode(), v)
	}
}

// syncScoreboard latches the Scoreboard module's LEDs when a score
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
		}{{pongPot("l"), p.PadL}, {pongPot("r"), p.PadR}} {
			if !s.sl.Truthy() || (kb.active && kb.slider.Equal(s.sl)) {
				continue
			}
			s.sl.Set("value", strconv.FormatFloat(s.pad/pongH, 'f', 2, 64))
			dom.Fire(s.sl, "input")
		}
		pongKnobGuard = false
	}
	if p.ScoreL == p.shownL && p.ScoreR == p.shownR {
		return
	}
	p.shownL, p.shownR = p.ScoreL, p.ScoreR
	if l := dom.Doc.Call("getElementById", "pong-score-l"); l.Truthy() {
		setDotText(l, strconv.Itoa(p.ScoreL))
	}
	if r := dom.Doc.Call("getElementById", "pong-score-r"); r.Truthy() {
		setDotText(r, strconv.Itoa(p.ScoreR))
	}
}

// ── Input + sound ────────────────────────────────────────────────────────

// wireInput installs the paddle key listeners once (lazily on the first
// generated frame). Handlers no-op outside pong mode. A real keydown is a
// user gesture, so it also lifts the audio context for the beeps.
func (p *pongGame) wireInput() {
	p.wired = true
	set := func(key string, down bool) bool {
		switch key {
		case "w":
			p.KeyW = down
			p.HumanL = scopegames.PongHuman
		case "s":
			p.KeyS = down
			p.HumanL = scopegames.PongHuman
		case "arrowup":
			p.KeyUp = down
			p.HumanR = scopegames.PongHuman
		case "arrowdown":
			p.KeyDn = down
			p.HumanR = scopegames.PongHuman
		default:
			return false
		}
		return true
	}
	dom.On(dom.Doc, "keydown", func(this js.Value, a []js.Value) any {
		e := a[0]
		if run.selectedMode != "pong" {
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
	})
	dom.On(dom.Doc, "keyup", func(this js.Value, a []js.Value) any {
		set(strings.ToLower(a[0].Get("key").String()), false)
		return nil
	})
}

// pointerPaddle drives the paddle on the pointer's half of the screen
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
		p.PadL = y
		p.HumanL = scopegames.PongHuman
	} else {
		p.PadR = y
		p.HumanR = scopegames.PongHuman
	}
}

// beep plays one classic square blip (hit 459 Hz, wall 226 Hz, point
// 490 Hz) through the shared context. Silent until a real key grants the
// context, and outside pong mode.
func (p *pongGame) beep(freq float64, ms int) {
	if !p.ctxHeld || run.selectedMode != "pong" {
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
	g.Get("gain").Call("setValueAtTime", scopegames.PongBlipGain, now)
	g.Get("gain").Call("linearRampToValueAtTime", 0, now+dur)
	osc.Call("connect", g)
	mixEnsure(ctx)
	g.Call("connect", mixIn("fx")) // the Mixer's SOUNDS column
	osc.Call("start")
	osc.Call("stop", now+dur+0.01)
}

// syncPongExtras runs on every panel rebuild (from buildParamPanel, beside
// the Graphic Artist switches): entering pong starts a fresh match facing
// the camera with the spin stopped — it's a scope game, not a model;
// leaving drops the beep lease and any held keys.
func (p *pongGame) syncPongExtras(mode string) {
	// The module is in the rack whatever the model (panelhtml_js.go): the rack
	// is the same instrument under every model, so its panels do not come and go.
	if mode == "pong" {
		if p.active {
			return
		}
		p.active = true
		p.Enter()
		normalizeOrientation()
		return
	}
	p.active = false
	p.KeyW, p.KeyS, p.KeyUp, p.KeyDn = false, false, false, false
	if p.ctxHeld {
		releaseAudioCtx("pong")
		p.ctxHeld = false
	}
}
