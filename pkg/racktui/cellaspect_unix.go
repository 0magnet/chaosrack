//go:build unix && !js

package racktui

import (
	"os"

	"golang.org/x/sys/unix"
)

// measuredCellAspect is the terminal's own cell shape, height over width,
// from the size in pixels it reports for its window, or 0 when it reports
// none: kitty, foot, WezTerm and xterm fill the pixel size in, and a
// terminal that leaves it zero keeps the default guess.
func measuredCellAspect() float64 {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // read only

	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ) //nolint:gosec // a file descriptor fits an int
	if err != nil || ws.Row == 0 || ws.Col == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0
	}
	return (float64(ws.Ypixel) / float64(ws.Row)) / (float64(ws.Xpixel) / float64(ws.Col))
}
