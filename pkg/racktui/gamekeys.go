package racktui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
)

// GameKeys is a source with a game that plays from the keyboard, as the
// page's Pong takes W/S and ↑/↓ for its paddles.
type GameKeys interface {
	// GameKey presses (down) or releases a game's key — "w", "s", "up" or
	// "down" — and reports whether a game took it: none is played, and the
	// key is the panel's own.
	GameKey(key string, down bool) bool
}

// gameKeyHold is how long a key counts as held after its last press, on a
// terminal that reports no releases: longer than a held key's repeat
// interval, short enough that a paddle stops about when the key is let go.
const gameKeyHold = 150 * time.Millisecond

// gameKeys is the keys a game holds down.
type gameKeys struct {
	// held is when each key held with no release reported lets go.
	held map[string]time.Time
	// releases is whether the terminal reports releases (the kitty keyboard
	// protocol does): from the first one seen, a key is held until its own.
	releases bool
}

// gameKeyName is a key as a game names it, or "".
func gameKeyName(ev *tcell.EventKey) string {
	switch ev.Key() {
	case tcell.KeyUp:
		return "up"
	case tcell.KeyDown:
		return "down"
	case tcell.KeyRune:
		switch k := strings.ToLower(ev.Str()); k {
		case "w", "s":
			return k
		}
	}
	return ""
}

// gameKey offers ev to the game being played, and reports whether it took
// it. Only with the window put away, when the scene is all there is: then
// the game's keys are the game's, and q, p and the rest stay the panel's.
func (p *panel) gameKey(ev *tcell.EventKey) bool {
	gk, ok := p.src.(GameKeys)
	if !ok || p.winMode != winHidden {
		return false
	}
	name := gameKeyName(ev)
	if name == "" {
		return false
	}
	if !ev.Pressed() {
		p.gk.releases = true
		delete(p.gk.held, name)
		return gk.GameKey(name, false)
	}
	if !gk.GameKey(name, true) {
		return false
	}
	if !p.gk.releases {
		if p.gk.held == nil {
			p.gk.held = map[string]time.Time{}
		}
		p.gk.held[name] = time.Now().Add(gameKeyHold)
	}
	return true
}

// letGoGameKeys releases the keys whose hold has run out.
func (p *panel) letGoGameKeys(now time.Time) {
	gk, ok := p.src.(GameKeys)
	if !ok {
		return
	}
	for name, t := range p.gk.held {
		if now.After(t) {
			delete(p.gk.held, name)
			gk.GameKey(name, false)
		}
	}
}
