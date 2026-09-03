// Package proxima registers Escape from Proxima 5 as a demo.
//
// The game itself is Garrett D'Amore's, Apache 2.0, and lives in a fork:
// github.com/0magnet/proxima5. What the fork changes is recorded there —
// tcell v1 to v2, a package rather than a command, and a screen it is given
// rather than one it makes.
//
// It earns its place by being a real program rather than a demonstration:
// sprites, collision, levels, a status bar, written in 2016 to be played in
// a terminal, none of it aware that it is now in a page.
package proxima

import (
	"strconv"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	game "github.com/0magnet/proxima5"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name:   "proxima",
		Desc:   "Escape from Proxima 5 — gdamore's tcell space shooter",
		Screen: run,
	})
}

func run(screen tcell.Screen, cols, rows int) error {
	// The game asks for at least 80x24 and is unplayable below it. Saying so
	// beats letting it draw off the edge of a small window.
	if cols < 80 || rows < 24 {
		return tooSmall(screen, cols, rows)
	}
	g := game.NewGame(screen)
	if err := g.Init(); err != nil {
		return err
	}
	return g.Run()
}

func tooSmall(screen tcell.Screen, cols, rows int) error {
	msg := "Proxima 5 needs at least 80x24 — this window is " +
		strconv.Itoa(cols) + "x" + strconv.Itoa(rows) + ". Make it bigger and reopen."
	st := tcell.StyleDefault.Foreground(color.Red)
	for i, r := range []rune(msg) {
		if i < cols {
			screen.SetContent(i, 0, r, nil, st)
		}
	}
	screen.Show()
	return nil
}
