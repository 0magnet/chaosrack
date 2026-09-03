// Package anim registers the terminal animations as demos.
//
// The animations themselves live in github.com/0magnet/termanim, which knows
// nothing about this showcase: each takes a tcell.Screen it is handed and
// never creates or destroys one. That is the same arrangement as the proxima
// demo, and it is what lets the same code be a page here and a command in a
// terminal.
package anim

import (
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/aquarium"
	"github.com/0magnet/termanim/boids"
	"github.com/0magnet/termanim/bonsai"
	"github.com/0magnet/termanim/clock"
	"github.com/0magnet/termanim/cube"
	"github.com/0magnet/termanim/donut"
	"github.com/0magnet/termanim/fire"
	"github.com/0magnet/termanim/fireworks"
	"github.com/0magnet/termanim/langton"
	"github.com/0magnet/termanim/lavalamp"
	"github.com/0magnet/termanim/life"
	"github.com/0magnet/termanim/matrix"
	"github.com/0magnet/termanim/maze"
	"github.com/0magnet/termanim/metaballs"
	"github.com/0magnet/termanim/moire"
	"github.com/0magnet/termanim/pipes"
	"github.com/0magnet/termanim/plasma"
	"github.com/0magnet/termanim/rain"
	"github.com/0magnet/termanim/sand"
	"github.com/0magnet/termanim/snow"
	"github.com/0magnet/termanim/starfield"
	"github.com/0magnet/termanim/tunnel"

	"github.com/0magnet/tuiwasm/demos"
)

// seeded wraps an animation that takes a seed. The seed is read when the demo
// starts rather than at registration, so opening the same demo twice does not
// replay it.
func seeded(run func(tcell.Screen, int64) error) func(tcell.Screen, int, int) error {
	return func(s tcell.Screen, _, _ int) error {
		return run(s, time.Now().UnixNano())
	}
}

// plain wraps the two animations that are a pure function of time and have
// nothing to randomize.
func plain(run func(tcell.Screen) error) func(tcell.Screen, int, int) error {
	return func(s tcell.Screen, _, _ int) error { return run(s) }
}

// registered is what this package puts in the registry. Kept in a variable so
// the tests can see this package's own entries: the registry is shared with
// every other demo package and keyed by name, so reading it back could not tell
// which came from here.
var registered = []demos.Demo{
	{Name: "fire", Desc: "a heat grid seeded with noise and cooled upward", Screen: seeded(fire.Run)},
	{Name: "plasma", Desc: "summed sine waves of position, offset by time", Screen: plain(plasma.Run)},
	{Name: "metaballs", Desc: "blobs that bulge and merge as they approach", Screen: seeded(metaballs.Run)},
	{Name: "moire", Desc: "two drifting ripples interfering", Screen: plain(moire.Run)},
	{Name: "lavalamp", Desc: "wax that heats, rises, cools and sinks", Screen: seeded(lavalamp.Run)},
	{Name: "tunnel", Desc: "flying down a textured tube", Screen: seeded(tunnel.Run)},
	{Name: "starfield", Desc: "stars streaming past the viewer", Screen: seeded(starfield.Run)},
	{Name: "donut", Desc: "a lit torus with a z-buffer", Screen: seeded(donut.Run)},
	{Name: "cube", Desc: "a rotating wireframe solid, shaded by depth", Screen: seeded(cube.Run)},
	{Name: "boids", Desc: "flocking by separation, alignment and cohesion", Screen: seeded(boids.Run)},
	{Name: "rain", Desc: "drops with depth, slant, streaks and splashes", Screen: seeded(rain.Run)},
	{Name: "snow", Desc: "flakes that sway, settle and drift into banks", Screen: seeded(snow.Run)},
	{Name: "fireworks", Desc: "shells that rise, burst and droop into willows", Screen: seeded(fireworks.Run)},
	{Name: "life", Desc: "Conway's life, colored by how long a cell has lived", Screen: seeded(life.Run)},
	{Name: "langton", Desc: "Langton's ants: chaos, then the highway", Screen: seeded(langton.Run)},
	{Name: "sand", Desc: "grains heaping at their angle of repose", Screen: seeded(sand.Run)},
	{Name: "maze", Desc: "a maze carved by backtracking, then solved", Screen: seeded(maze.Run)},
	{Name: "matrix", Desc: "falling columns of glyphs", Screen: seeded(matrix.Run), Mirror: mirrorKana},
	{Name: "clock", Desc: "an analog clock, after aclock", Screen: plain(clock.Run)},
	{Name: "aquarium", Desc: "fish swimming past swaying seaweed", Screen: seeded(aquarium.Run)},
	{Name: "pipes", Desc: "pipes growing and turning, with correct elbows", Screen: seeded(pipes.Run)},
	{Name: "bonsai", Desc: "a bonsai tree growing branch by branch", Screen: seeded(bonsai.Run)},
}

func init() {
	for _, d := range registered {
		demos.Register(d)
	}
}

// mirrorKana reports whether a glyph is a half-width katakana, which the matrix
// demo asks to have drawn flipped left-to-right.
//
// The film's code is a custom typeface whose katakana are mirrored, and Unicode
// encodes no mirrored kana — there are 428 mirrored pairs in the standard and
// not one is a kana — so the demo draws ordinary kana and asks the terminal to
// turn them round. Only the kana: the numerals and the Z in the set are not
// mirrored here, and a terminal that cannot flip anything shows the whole
// alphabet the usual way round rather than showing nothing.
func mirrorKana(s string) bool {
	for _, r := range s {
		// Halfwidth Katakana, from ｦ to ﾟ. The punctuation below it — the
		// brackets and the ideographic full stop — is left alone.
		if r < 0xFF66 || r > 0xFF9F {
			return false
		}
	}
	return s != ""
}

// demoList returns what this package registers, for its own tests. The registry
// itself is keyed by name and shared with every other demo package, so a test
// that read it back could not tell which entries came from here.
func demoList() []demos.Demo { return registered }
