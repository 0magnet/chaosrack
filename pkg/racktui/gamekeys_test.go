package racktui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
)

// gameRack is a fakeRack playing a game that takes keys.
type gameRack struct {
	fakeRack
	playing bool
	down    map[string]bool
}

func (g *gameRack) GameKey(key string, down bool) bool {
	if !g.playing {
		return false
	}
	g.down[key] = down
	return true
}

// With the window put away, the game's keys are the game's: W is held until
// the terminal reports its release, or, when it reports none, until a moment
// after its last press. With the window up they are the panel's.
func TestTheGamesKeysGoToTheGameWithTheWindowAway(t *testing.T) {
	g := &gameRack{fakeRack: fakeRack{ctls: []Control{dial("zoom", 0, 1, 0.1, 0, "0")}}, playing: true, down: map[string]bool{}}
	p := newPanel(&g.fakeRack)
	p.src = g
	w := tcell.NewEventKey(tcell.KeyRune, "w", tcell.ModNone)
	if p.gameKey(w) {
		t.Fatal("the window is up, and the game took W")
	}
	p.winMode = winHidden
	if !p.gameKey(w) || !g.down["w"] {
		t.Fatal("the game did not take W with the window away")
	}
	p.letGoGameKeys(time.Now())
	if !g.down["w"] {
		t.Fatal("W let go at once")
	}
	p.letGoGameKeys(time.Now().Add(2 * gameKeyHold))
	if g.down["w"] {
		t.Fatal("W, never released, is still held")
	}
	if !p.gameKey(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone)) || !g.down["up"] {
		t.Fatal("↑ is not the game's")
	}
	g.playing = false
	if p.gameKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone)) {
		t.Error("with no game played, ↓ went to the game")
	}
}
