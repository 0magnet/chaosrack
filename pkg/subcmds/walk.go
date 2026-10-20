package subcmds

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// What a picture looks like besides the model: its background, and, over an
// animation, a walk through the Colors module's two knobs.
//
// A WALK is the page's portrait loop done properly. The page's way was to turn
// the knobs between screenshots while auto-rotate went on turning the model,
// so every change of colors was also a jump in the pose. Here the colors are
// a function of the frame and so is the camera, separately: the palette steps
// at the segment boundaries and the turn does not notice.

// walkStep is one setting of the two knobs: --colors and --gradient.
type walkStep struct {
	colors int
	grad   string
}

// parseWalk reads --walk: COLORS/GRADIENT steps separated by commas, e.g.
// "4/z,2/x,3/y,1/trail". An empty walk is none.
func parseWalk(s string) ([]walkStep, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []walkStep
	for part := range strings.SplitSeq(s, ",") {
		c, g, ok := strings.Cut(strings.TrimSpace(part), "/")
		n, err := strconv.Atoi(c)
		if !ok || err != nil || n < 1 || n > 4 {
			return nil, fmt.Errorf("--walk %q: each step is COLORS/GRADIENT, COLORS 1-4 and GRADIENT x, y, z or trail", part)
		}
		switch g {
		case "x", "y", "z", "trail":
		default:
			return nil, fmt.Errorf("--walk %q: the gradient is x, y, z or trail", part)
		}
		out = append(out, walkStep{n, g})
	}
	return out, nil
}

// gradientAt is frame i of n's gradient: the walk's step for that frame, or
// the flags' setting when there is no walk. A still is frame 0.
func gradientAt(i, n int) rasterview.Gradient {
	steps, err := parseWalk(renderWalk)
	if err != nil || len(steps) == 0 {
		return gradientOf(renderColors, renderGrad)
	}
	k := 0
	if n > 0 {
		k = min(i*len(steps)/n, len(steps)-1)
	}
	return gradientOf(steps[k].colors, steps[k].grad)
}

// background is --background as a color: "#rrggbb", the page's canvas black
// by default.
func background() color.RGBA {
	s := strings.TrimPrefix(strings.TrimSpace(renderBG), "#")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 6 {
		return color.RGBA{0, 0, 0, 255}
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} //nolint:gosec // three bytes of a six-digit hex
}

// backgroundHex is background as SVG writes it.
func backgroundHex() string {
	c := background()
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
