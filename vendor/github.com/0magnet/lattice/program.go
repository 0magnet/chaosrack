package lattice

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Program is the lattice command as a library: the same flags, writing the
// same bytes. The command runs it on a ticker into its terminal; a host that
// keeps its own terminals (a page drawing many of them as sheets) runs it once
// a frame into each. Only who owns the clock differs.
type Program struct {
	o  options
	sc *screen

	// What the input has done to the solid (input.go): the keys' turns, and
	// the host's pose.
	keyYaw, keyPitch float64
	pose             Quat
	in               []byte // the start of a sequence still arriving

	// How the solid stood when the screen was last drawn: a solid that has
	// not moved draws the same screen, so it is not drawn again.
	drawnAs turn
	drawn   bool
}

// options are the command line's.
type options struct {
	sheet       Sheet
	shape       string
	style       Style
	coloring    Coloring
	color       bool
	spin, pitch float64 // degrees a second, degrees
	fps         float64
	once, tile  bool
	cols        int // the terminal's width, for -tile
}

// NewProgram parses a command line (without the program name). Usage and
// errors are written to stderr, as the command's are.
func NewProgram(args []string, stderr io.Writer) (*Program, error) {
	fs := flag.NewFlagSet("lattice", flag.ContinueOnError)
	fs.SetOutput(stderr)
	n := fs.Int("n", 16, "voxels along each axis")
	axis := fs.String("axis", "z", "the sheet's axis: x, y or z")
	slice := fs.Int("slice", -1, "the sheet's slice, 0 to n-1 (default the middle)")
	shape := fs.String("shape", "sphere", "the solid: "+strings.Join(ShapeNames(), ", "))
	style := fs.String("style", "lines", "how the surface is written: lines, ascii, shade or solid")
	mono := fs.Bool("mono", false, "one color, not colored by position")
	noColor := fs.Bool("no-color", os.Getenv("NO_COLOR") != "", "no color escapes at all")
	spin := fs.Float64("spin", 20, "degrees a second the solid turns")
	pitch := fs.Float64("pitch", 20, "degrees the solid is tilted toward the viewer")
	fps := fs.Float64("fps", 20, "frames a second")
	once := fs.Bool("once", false, "print one frame and exit")
	tile := fs.Bool("tile", false, "draw every slice of the axis, side by side")
	cols := fs.Int("cols", envCols(), "the terminal's width, for -tile (default $COLUMNS, or 160)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	o := options{shape: *shape, color: !*noColor, spin: *spin, pitch: *pitch, fps: *fps, once: *once, tile: *tile, cols: *cols}
	if *n < 2 || *n > 256 {
		return nil, fmt.Errorf("-n %d: want 2 to 256", *n)
	}
	a, ok := ParseAxis(*axis)
	if !ok {
		return nil, fmt.Errorf("-axis %q: want x, y or z", *axis)
	}
	if *slice < 0 {
		*slice = *n / 2
	}
	if *slice >= *n {
		return nil, fmt.Errorf("-slice %d: want 0 to %d", *slice, *n-1)
	}
	o.sheet = Sheet{Axis: a, Slice: *slice, N: *n}
	if _, ok := Shapes[*shape]; !ok {
		return nil, fmt.Errorf("-shape %q: want one of %s", *shape, strings.Join(ShapeNames(), ", "))
	}
	if o.style, ok = Styles[*style]; !ok {
		return nil, fmt.Errorf("-style %q: want lines, ascii, shade or solid", *style)
	}
	if *mono {
		o.coloring = Mono
	}
	if !(o.fps > 0) {
		return nil, fmt.Errorf("-fps %v: want more than 0", o.fps)
	}
	return &Program{o: o, pose: Identity}, nil
}

// envCols is the terminal's width as the shell reports it, or 160.
func envCols() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return 160
}

// Sheet is the section this program draws.
func (p *Program) Sheet() Sheet { return p.o.sheet }

// Once reports -once: print one frame and stop.
func (p *Program) Once() bool { return p.o.once }

// Interval is the time between frames, -fps's.
func (p *Program) Interval() time.Duration { return time.Duration(float64(time.Second) / p.o.fps) }

// Enter takes over terminal w: the alternate screen, the cursor hidden.
func (p *Program) Enter(w io.Writer) error {
	p.sc = newScreen(w, p.o.color)
	return p.sc.enter()
}

// Frame draws the solid as it stands at t, sending only what changed.
func (p *Program) Frame(t time.Time) error {
	if p.sc == nil {
		return fmt.Errorf("lattice: Frame before Enter")
	}
	tu := turn{yaw: p.o.yaw(t) + p.keyYaw, pitch: p.o.pitch*math.Pi/180 + p.keyPitch, pose: p.pose}
	if p.drawn && tu == p.drawnAs {
		return nil
	}
	p.drawnAs, p.drawn = tu, true
	return p.sc.draw(p.o.frameAt(tu))
}

// Leave gives the terminal back.
func (p *Program) Leave() error {
	if p.sc == nil {
		return nil
	}
	return p.sc.leave()
}

// Print writes the frame at t to w as lines of text, for -once.
func (p *Program) Print(w io.Writer, t time.Time) error { return plain(w, p.o.frame(t), p.o.color) }

// solid is the shape as it stands at t. The angle comes from the wall clock,
// not from when this process started, so every sheet's process agrees on it.
func (o options) solid(t time.Time) Shape { return o.solidAt(o.turnAt(t)) }

// turn is how the solid stands: its turn and tilt, then the host's pose.
type turn struct {
	yaw, pitch float64
	pose       Quat
}

// turnAt is the solid's stand at t with no input.
func (o options) turnAt(t time.Time) turn {
	return turn{yaw: o.yaw(t), pitch: o.pitch * math.Pi / 180, pose: Identity}
}

// yaw is the solid's turn at t, in radians.
func (o options) yaw(t time.Time) float64 {
	sec := float64(t.UnixNano()%int64(1e12)) / 1e9 // wraps every ~16 minutes, invisibly at any whole-degree spin
	return math.Mod(o.spin*sec, 360) * math.Pi / 180
}

// solidAt is the shape as it stands at tu.
func (o options) solidAt(tu turn) Shape {
	return Posed(Turned(Shapes[o.shape], tu.yaw, tu.pitch), tu.pose)
}

// frame is what this program draws at t: its sheet, or with -tile every
// sheet of its axis.
func (o options) frame(t time.Time) Frame { return o.frameAt(o.turnAt(t)) }

// frameAt is frame with the solid standing as tu says.
func (o options) frameAt(tu turn) Frame {
	s := o.solidAt(tu)
	if !o.tile {
		return Render(o.sheet, s, o.style, o.coloring)
	}
	fs := make([]Frame, o.sheet.N)
	for k := range fs {
		sh := o.sheet
		sh.Slice = k
		fs[k] = Render(sh, s, o.style, o.coloring)
	}
	return tiled(fs, o.sheet.Axis, o.cols)
}

// tiled lays sheets out left to right and then down, as many to a row as fit
// in cols, each under a label naming it.
func tiled(fs []Frame, a Axis, cols int) Frame {
	if len(fs) == 0 {
		return Frame{}
	}
	w, h := fs[0].Cols+1, fs[0].Rows+1 // a column between, a label row above
	per := max(1, (cols+1)/w)
	rows := (len(fs) + per - 1) / per
	out := Frame{Cols: per*w - 1, Rows: rows * h}
	out.Cells = make([]Cell, out.Cols*out.Rows)
	for k, f := range fs {
		x0, y0 := (k%per)*w, (k/per)*h
		for i, ch := range a.String() + strconv.Itoa(k) {
			if x0+i < out.Cols {
				out.Cells[y0*out.Cols+x0+i] = Cell{Ch: ch, R: 0x70, G: 0x70, B: 0x70}
			}
		}
		for r := range f.Rows {
			copy(out.Cells[(y0+1+r)*out.Cols+x0:], f.Cells[r*f.Cols:(r+1)*f.Cols])
		}
	}
	return out
}
