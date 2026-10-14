package rackhost

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/scopegames"
)

// The scope games — the Bouncing Ball and Scope Pong (scopegames) — played
// here as the page plays them: stepped sixty frames a second on the clock,
// drawn in the scene, their blips on the Mixer's FX column, their scores on
// their displays and Pong's paddle pots turning with the paddles. Each is
// silent until a hand is on it, as the page's are: the ball's blips wait for
// any hand on the rack, which on the page is what lets audio start, and
// Pong's for a hand on a paddle, which on the page is a key.

// gameFPS is the page's frame rate, which a game steps at.
const gameFPS = 60

// frameSamples is one frame, in samples.
const frameSamples = sampleRate / gameFPS

// maxCatchUp is the most frames one look steps a game: a panel that looked
// away does not come back to a burst of blips.
const maxCatchUp = 30

// blip is a game's blip sounding: a sine or a square falling from its level
// to nothing, stopped a moment after, as the page's oscillator and gain ramp
// make it.
type blip struct {
	wave     int
	hz, gain float64
	wait, n  int
	dur      int // the fall, in samples
	ph       float64
}

func (b *blip) next() (float64, bool) {
	if b.wait > 0 {
		b.wait--
		return 0, true
	}
	if b.n >= b.dur+sampleRate/100 {
		return 0, false
	}
	g := 0.0
	if b.n < b.dur {
		g = b.gain * (1 - float64(b.n)/float64(b.dur))
	}
	dt := b.hz / sampleRate
	x := osc(b.wave, b.ph, dt)
	b.ph = frac(b.ph + dt)
	b.n++
	return g * x, true
}

// games is the scope games' play: which is being played, their state, and
// their blips sounding.
type games struct {
	key     string // the game being played, or ""
	running bool   // stepping: the model on, and not paused
	ball    scopegames.Ball
	ring    [][3]float64 // the ball's trail, oldest first
	pong    scopegames.Pong
	// pongArmed is a hand having played a paddle, which lets Pong be heard.
	pongArmed bool
	now       func() time.Time // the clock the games step on
	last      time.Time
	frac      float64
	frame     int // the frame stepping within a look, for its blips' wait
	blips     []blip
	strokes   [][]float64
}

// isGame reports whether model is one of the scope games.
func isGame(model string) bool { return model == "bounceball" || model == "pong" }

// newGames is the games at their knobs' defaults.
func newGames() games {
	return games{ball: scopegames.NewBall(rand.Float64), pong: scopegames.NewPong(rand.Float64), now: time.Now}
}

// wire points the games' blips at s's FX column. Called once, when the
// sound has its place.
func (s *sound) wireGames() {
	g := &s.games
	g.ball.Beep = func(hz float64, ms int) {
		if s.touched {
			g.blip(audiosrc.WaveSine, hz, scopegames.BallBlipGain, ms)
		}
	}
	g.pong.Beep = func(hz float64, ms int) {
		if g.pongArmed {
			g.blip(audiosrc.WaveSquare, hz, scopegames.PongBlipGain, ms)
		}
	}
}

// blip sounds a blip, at the frame being stepped.
func (g *games) blip(wave int, hz, gain float64, ms int) {
	if len(g.blips) >= 64 {
		return
	}
	g.blips = append(g.blips, blip{wave: wave, hz: hz, gain: gain,
		wait: g.frame * frameSamples, dur: ms * sampleRate / 1000})
}

// heard reports whether a game is being played where a hand lets it be
// heard, touched saying whether a hand has been on the rack. Called with the
// sound's lock held.
func (g *games) heard(touched bool) bool {
	if len(g.blips) > 0 {
		return true
	}
	if !g.running {
		return false
	}
	return g.key == "pong" && g.pongArmed || g.key == "bounceball" && touched
}

// render adds n samples of the blips into out, and lets go of the ones
// done.
func (g *games) render(out []float32, n int) {
	clear(out[:n])
	live := g.blips[:0]
	for _, b := range g.blips {
		on := true
		for j := 0; j < n && on; j++ {
			var x float64
			x, on = b.next()
			out[j] += float32(x)
		}
		if on {
			live = append(live, b)
		}
	}
	g.blips = live
}

// enter starts game key as the page does when it is chosen: the ball dropped
// with its trail warmed up silent, a fresh match of Pong.
func (g *games) enter(key string, trail int) {
	g.key = key
	g.ring = g.ring[:0]
	switch key {
	case "bounceball":
		g.ball.Enter()
		g.ball.Warm = true
		for range trail {
			g.ball.Step(scopegames.BallFrameDT)
			g.ring = append(g.ring, [3]float64{g.ball.X, g.ball.Y, 0})
		}
		g.ball.Warm = false
	case "pong":
		g.pong.Enter()
	}
}

// step advances the game being played one frame, sub sub-steps of scale.
func (g *games) step(sub int, scale float64, trail int) {
	switch g.key {
	case "bounceball":
		for range sub {
			g.ball.Step(scopegames.BallFrameDT * scale)
			g.ring = append(g.ring, [3]float64{g.ball.X, g.ball.Y, 0})
		}
		if over := len(g.ring) - max(trail, 2); over > 0 {
			g.ring = append(g.ring[:0], g.ring[over:]...)
		}
	case "pong":
		g.pong.Step(float64(sub) * scale)
	}
}

// gameShown is what a game shows on the panel after a look: its displays
// and Pong's paddles.
type gameShown struct {
	key            string
	kicks          int
	scoreL, scoreR int
	padL, padR     float64
}

// playGames steps the game the model selector is on to now, then writes its
// displays and turns Pong's pots after its paddles. Called with r.mu held,
// each time the panel looks.
func (r *Rack) playGames() {
	model := r.value(r.selector)
	on := r.value("power-sw") != "0" && r.value("pause-sw") != "1"
	trail := int(r.number("trail-slider"))
	sub, scale := speedSteps(r.number("speed-slider"))
	s := r.sound
	s.mu.Lock()
	g := &s.games
	now := g.now()
	if !isGame(model) {
		g.key, g.running = "", false
		s.mu.Unlock()
		return
	}
	if model != g.key {
		g.enter(model, trail)
		g.last, g.frac = now, 0
	}
	g.running = on
	frames := now.Sub(g.last).Seconds()*gameFPS + g.frac
	g.last = now
	n := int(frames)
	g.frac = frames - float64(n)
	if !on {
		n = 0
	}
	for i := range min(n, maxCatchUp) {
		g.frame = i
		g.step(sub, scale, trail)
	}
	shown := gameShown{key: g.key, kicks: g.ball.Kicks, scoreL: g.pong.ScoreL, scoreR: g.pong.ScoreR,
		padL: g.pong.PadL, padR: g.pong.PadR}
	s.mu.Unlock()
	r.showGame(shown)
}

// showGame writes a game's displays, and turns Pong's paddle pots to its
// paddles, as the page's motorized pots follow the machine: not a hand, so
// the game is not told. Called with r.mu held.
func (r *Rack) showGame(g gameShown) {
	switch g.key {
	case "bounceball":
		r.showLED("bounce-kicks", strconv.Itoa(g.kicks))
	case "pong":
		r.showLED("pong-score-l", strconv.Itoa(g.scoreL))
		r.showLED("pong-score-r", strconv.Itoa(g.scoreR))
		for _, p := range []struct {
			id  string
			pad float64
		}{{"pong-pad-l", g.padL}, {"pong-pad-r", g.padR}} {
			i, ok := r.byID[p.id]
			if !ok {
				continue
			}
			c := &r.ctls[i]
			if v := strconv.FormatFloat(p.pad/scopegames.PongH, 'f', 2, 64); v != c.Value {
				c.Value = v
				r.turn(c)
			}
		}
	}
}

// showLED writes a game display the catalog marked "led ID".
func (r *Rack) showLED(id, text string) {
	for i, it := range r.pic.Items {
		if it != nil && it.Mark == "led "+id {
			r.write(i, text)
		}
	}
}

// isGameControl reports whether id is one of the games' knobs or the head's
// first switch, which drops the ball or restarts Pong.
func isGameControl(id string) bool {
	return strings.HasPrefix(id, "bounce-") || strings.HasPrefix(id, "pong-")
}

// gameSet reads a game's control into it. Called with r.mu and the sound's
// lock held.
func (r *Rack) gameSet(id string) {
	g := &r.sound.games
	f := float32(r.number(id))
	switch id {
	case "bounce-grav":
		g.ball.Grav = f
	case "bounce-rest":
		g.ball.Rest = f
	case "bounce-drift":
		g.ball.Drift = f
	case "bounce-height":
		g.ball.Height = f
	case "pong-speed":
		g.pong.BallSpeed = f
	case "pong-paddle":
		g.pong.PaddleH = f
	case "pong-skill":
		g.pong.AISkill = f
	case "pong-pad-l", "pong-pad-r":
		// A hand on a pot seizes its paddle, and is a hand on the game.
		g.pong.Seize(id == "pong-pad-l", float64(f))
		g.pongArmed = true
	}
}

// figure is the game being played as a figure, framed to its court: the
// ball's trail as a path, Pong's beam strokes as lines. Called with the
// sound's lock held.
func (g *games) figure() (fig attractor.Figure, fit [][3]float64) {
	switch g.key {
	case "bounceball":
		fig = attractor.Figure{Kind: attractor.FigurePath, Points: append([][3]float64(nil), g.ring...)}
		const r = scopegames.BallWall + 0.1
		return fig, [][3]float64{{-r, -r, 0}, {r, r, 0}}
	case "pong":
		g.strokes = g.pong.Strokes(g.strokes[:0])
		fig.Kind = attractor.FigureLines
		for _, st := range g.strokes {
			base := len(fig.Points)
			for i := 0; i+1 < len(st); i += 2 {
				fig.Points = append(fig.Points, [3]float64{st[i], st[i+1], 0})
			}
			for k := base; k+1 < len(fig.Points); k++ {
				fig.Edges = append(fig.Edges, uint16(k), uint16(k+1)) //nolint:gosec // G115: a frame of Pong is a few hundred points
			}
		}
		const w, h = scopegames.PongW + 0.05, scopegames.PongH + 0.05
		return fig, [][3]float64{{-w, -h, 0}, {w, h, 0}}
	}
	return fig, nil
}

// hand is a hand on the rack, which lets the ball's blips be heard.
func (s *sound) hand() {
	s.mu.Lock()
	s.touched = true
	s.mu.Unlock()
}

// GameKey plays Pong's paddles from the keyboard, as the page's W/S and ↑/↓
// do: a key held moves its paddle, and the side becomes a hand's. Another
// model takes no keys.
func (r *Rack) GameKey(key string, down bool) bool {
	r.mu.Lock()
	pong := r.value(r.selector) == "pong"
	r.mu.Unlock()
	if !pong {
		return false
	}
	s := r.sound
	s.mu.Lock()
	defer s.mu.Unlock()
	p := &s.games.pong
	switch key {
	case "w":
		p.KeyW, p.HumanL = down, scopegames.PongHuman
	case "s":
		p.KeyS, p.HumanL = down, scopegames.PongHuman
	case "up":
		p.KeyUp, p.HumanR = down, scopegames.PongHuman
	case "down":
		p.KeyDn, p.HumanR = down, scopegames.PongHuman
	default:
		return false
	}
	if down {
		// A hand on a paddle, as a pot turned is: Pong is heard from now.
		s.games.pongArmed = true
	}
	return true
}
