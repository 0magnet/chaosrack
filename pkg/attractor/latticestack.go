package attractor

import (
	"io"
	"slices"
	"strconv"
	"time"
	"unicode"

	"github.com/0magnet/lattice"
	"github.com/0magnet/xterm-go/vt"
)

// The Lattice's stack of terminals, shared by the page (lattice_js.go, whose
// file comment says what it is) and a native rack: the programs, each in a
// real terminal of its own, run on the wall's clock and told the view's
// turn, and the cells of their screens placed in the stack. How the cells
// are drawn is each side's own: glyphs on the page, points natively.

// LatticeSheet is one terminal and the program drawing in it.
type LatticeSheet struct {
	prog *lattice.Program
	term *vt.Terminal
	sh   lattice.Sheet
	next float64 // when this program next draws, ms

	// writes counts what the program has written to the terminal: the
	// program sends nothing when its screen has not changed, so an unchanged
	// count is an unchanged screen.
	writes *uint64
}

// LatticeStack is the Lattice's knobs and its sheets.
type LatticeStack struct {
	shapeF, styleF, nF, spinF, tiltF float32
	lookF, opacF, stacksF, turnF     float32

	sheets   []*LatticeSheet
	sig      string // the knobs the sheets were started with
	rr       int    // the program a frame starts with, round robin
	sentPose lattice.Quat
}

// NewLatticeStack is the Lattice at the page's defaults.
func NewLatticeStack() *LatticeStack {
	return &LatticeStack{shapeF: latticeSphere, nF: 12, spinF: 20, tiltF: 20, opacF: 10}
}

// LatticeKnobs are the Lattice's knobs, by the page's ids.
var LatticeKnobs = []string{"lattice-shape", "lattice-spin", "lattice-tilt", "lattice-n", "lattice-opac", "lattice-style", "lattice-look", "lattice-stacks", "lattice-turn"}

// Set turns knob id to v, reporting whether the Lattice has it.
func (l *LatticeStack) Set(id string, v float32) bool {
	p := map[string]*float32{
		"lattice-shape": &l.shapeF, "lattice-spin": &l.spinF, "lattice-tilt": &l.tiltF,
		"lattice-n": &l.nF, "lattice-opac": &l.opacF, "lattice-style": &l.styleF,
		"lattice-look": &l.lookF, "lattice-stacks": &l.stacksF, "lattice-turn": &l.turnF,
	}[id]
	if p == nil {
		return false
	}
	*p = v
	return true
}

// latticeSphere is the sphere's place on the shape knob, which is where it starts.
var latticeSphere = float32(max(0, slices.Index(lattice.ShapeNames(), "sphere")))

// latticeStyles are the styles in the order the knob turns through them.
var latticeStyles = []string{"lines", "ascii", "shade", "solid"}

// latticeStacks are STACKS's positions: which families of sheets there are.
// AUTO keeps the three while they face the viewer and drops one as it turns
// edge on, where it shows next to nothing — its programs stop as well.
var latticeStacks = []string{"xyz", "auto", "xz", "z"}

// latticeTurns are TURN's positions: what turning the view turns. BOTH turns
// the stack and the solid in it together. LATTICE turns the stack and holds
// the solid still, so its surface moves through the grid as the grid turns
// round it. SOLID holds the stack still and turns the solid inside it. The
// programs are told as any program is told anything, on their input: a
// pose (lattice.PoseSequence), the view's turn or its inverse.
var latticeTurns = []string{"both", "lattice", "solid"}

// latticeAutoMin is how squarely, as the cosine squared, a family must face
// the viewer for AUTO to keep it: 0.04 is within about 12° of edge on.
const latticeAutoMin = 0.04

// latticeFPS is the programs' frame rate. A voxel picture changes in steps,
// so more than this draws the same screen again.
const latticeFPS = 15

// LatticeFPS is how often the programs draw: as often as a figure is worth
// handing them.
const LatticeFPS = latticeFPS

// latticeExtent is how far the camera is fitted to: the volume's corners,
// nearly, since it turns inside a cube of ±1.
const latticeExtent = 1.4

// LatticeExtent is the reach of the stack, which frames it.
const LatticeExtent = latticeExtent

// args are the command line each sheet's program is started with, and n the
// rows of its terminal: no -n, because the terminal's size is the resolution.
func (l *LatticeStack) args(fig bool) (n int, args []string) {
	n = max(2, int(l.nF+0.5))
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }
	args = []string{"-style", latticeStyles[pick(l.styleF, len(latticeStyles))], "-fps", strconv.Itoa(latticeFPS)}
	if fig {
		return n, append(args, "-spin", "0", "-pitch", "0")
	}
	return n, append(args,
		"-shape", lattice.ShapeNames()[pick(l.shapeF, len(lattice.ShapeNames()))],
		"-spin", f(l.spinF),
		"-pitch", f(l.tiltF))
}

// vtWriter is a terminal's input, as the program sees its terminal, counting
// what passes through it.
type vtWriter struct {
	t      *vt.Terminal
	writes *uint64
}

func (w vtWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.t.Write(p)
		*w.writes++
	}
	return len(p), nil
}

// Ensure gives every sheet a terminal and starts its program in it, the same
// way it would be started from a shell, when the knobs ask for different ones —
// each shown fig, when there is one to draw, as -figure would read one from
// a file — and reports the rows a sheet has and whether it started them.
func (l *LatticeStack) Ensure(fig *lattice.Figure, nowMs float64) (n int, started bool) {
	n, args := l.args(fig != nil)
	sig := joinArgs(args)
	if fig != nil {
		sig += "(figure)"
	}
	if sig == l.sig {
		return n, false
	}
	l.sig = sig
	l.sheets = l.sheets[:0]
	for _, a := range []lattice.Axis{lattice.X, lattice.Y, lattice.Z} {
		for k := range n {
			opts := vt.NewOptions()
			opts.Cols, opts.Rows, opts.Scrollback = 2*n, n, 0
			term := vt.NewTerminal(opts)
			full := append(append([]string(nil), args...), "-axis", a.String(), "-slice", strconv.Itoa(k))
			prog, err := lattice.NewProgram(full, io.Discard)
			if err != nil {
				continue // the knobs' ranges keep this from happening
			}
			prog.Resize(2*n, n)
			if fig != nil {
				prog.Show(fig)
			}
			writes := new(uint64)
			if err := prog.Enter(vtWriter{term, writes}); err != nil {
				continue
			}
			l.sheets = append(l.sheets, &LatticeSheet{prog: prog, term: term, sh: prog.Sheet(), writes: writes})
			_ = prog.Frame(time.Now()) //nolint:errcheck // a first picture, before the staggering starts; vtWriter cannot fail
		}
	}
	for i, s := range l.sheets {
		s.next = nowMs + 1000.0/latticeFPS*float64(i)/float64(len(l.sheets))
	}
	l.sentPose = lattice.Quat{} // new programs: tell them again
	return n, true
}

// Pose tells the programs the view's turn vq as TURN asks: the stack and the
// solid turned together, the solid held still while the stack turns, or the
// stack held square while the solid turns. It reports whether the stack is
// held square.
func (l *LatticeStack) Pose(vq lattice.Quat) (square bool) {
	pose := lattice.Identity
	switch latticeTurns[pick(l.turnF, len(latticeTurns))] {
	case "lattice": // the solid turned back by as much as the stack: still, in the room
		pose = lattice.Quat{vq[0], -vq[1], -vq[2], -vq[3]}
	case "solid": // the stack square to the screen, the view's turn the solid's
		pose, square = vq, true
	}
	if pose != l.sentPose {
		l.sentPose = pose
		seq := []byte(lattice.PoseSequence(pose))
		for _, s := range l.sheets {
			s.prog.Input(seq)
		}
	}
	return square
}

// Families are the families STACKS shows, seen along look, the direction
// toward the viewer in the stack's frame.
func (l *LatticeStack) Families(look lattice.Vec3) (on [3]bool) {
	switch latticeStacks[pick(l.stacksF, len(latticeStacks))] {
	case "xyz":
		return [3]bool{true, true, true}
	case "xz":
		return [3]bool{lattice.X: true, lattice.Z: true}
	case "z":
		return [3]bool{lattice.Z: true}
	}
	best := 0
	for a := range 3 {
		on[a] = look[a]*look[a] >= latticeAutoMin
		if look[a]*look[a] > look[best]*look[best] {
			best = a
		}
	}
	on[best] = true // whatever the angle, one family faces the viewer most
	return on
}

// Run lets the programs of the families kept draw, each on its own clock at
// latticeFPS, staggered, for no longer than budget: a frame that cannot get
// round them all starts where it stopped next time.
func (l *LatticeStack) Run(nowMs float64, now time.Time, on [3]bool, budget time.Duration) {
	interval := 1000.0 / latticeFPS
	deadline := now.Add(budget)
	for k := range len(l.sheets) {
		i := (l.rr + k) % len(l.sheets)
		s := l.sheets[i]
		if !on[s.sh.Axis] || nowMs < s.next {
			continue
		}
		if time.Now().After(deadline) {
			l.rr = i
			break
		}
		_ = s.prog.Frame(now) //nolint:errcheck // vtWriter cannot fail
		if s.next += interval; s.next < nowMs {
			s.next = nowMs + interval // fallen behind: start again from now
		}
	}
}

// Cells hands put every character of the kept families' screens, where it
// sits in the stack (±1 each way), its color, and whether it is a cell's
// background rather than a glyph: half a character a voxel, as the page
// draws its two halves.
func (l *LatticeStack) Cells(n int, on [3]bool, put func(x, y, z float32, r, g, b float32, background bool)) {
	_, mid := latticeCuts(n)
	for _, s := range l.sheets {
		if !on[s.sh.Axis] {
			continue
		}
		b := lattice.BasisOf(s.sh.Axis)
		plane := b.Normal.Scale(cellCenter(s.sh.Slice, n))
		center := func(pu, pv int) (float32, float32, float32) {
			c := plane.Add(b.Right.Scale(mid(pu))).Add(b.Up.Scale(mid(pv)))
			return float32(c[0]), float32(c[1]), float32(c[2])
		}
		buf := s.term.Buffer()
		for r := range n {
			if buf.YDisp+r >= buf.Lines.Length() {
				break
			}
			line := buf.Lines.Get(buf.YDisp + r)
			if line == nil {
				continue
			}
			k := n - 1 - r // the row's voxel, counted up from the bottom
			for col := range 2 * n {
				pu := col/2 + col%2 // the pane a character is in, across
				if bg := line.GetBg(col); bg&vt.AttrCMMask != 0 {
					r0, g0, b0 := vtColor(bg)
					x, y, z := center(pu, k)
					put(x, y, z, r0, g0, b0, true)
				}
				cp := line.GetCodePoint(col)
				if cp <= ' ' || cp > unicode.MaxRune {
					continue
				}
				cr, cg, cb := vtColor(line.GetFg(col))
				x, y, z := center(pu, k)
				put(x, y, z, cr, cg, cb, false)
			}
		}
	}
}

func joinArgs(a []string) string {
	s := ""
	for _, x := range a {
		s += x + " "
	}
	return s
}

// latticeCuts are where the other two families cross a sheet of n, in its own
// coordinates — the volume's edge, every voxel center, the other edge — and
// mid(i) the middle of the pane between cuts i and i+1.
func latticeCuts(n int) (cuts []float64, mid func(int) float64) {
	cuts = make([]float64, 0, n+2)
	cuts = append(cuts, -1)
	for k := range n {
		cuts = append(cuts, cellCenter(k, n))
	}
	cuts = append(cuts, 1)
	return cuts, func(i int) float64 { return (cuts[i] + cuts[i+1]) / 2 }
}

// cellCenter is the coordinate of the middle of slice i of n, as the program
// places it.
func cellCenter(i, n int) float64 { return (float64(i)+0.5)/float64(n)*2 - 1 }

// vtColor is a cell's foreground as the terminal holds it: truecolor, one of
// the 256-color palette, or the default, a pale gray.
func vtColor(fg uint32) (r, g, b float32) {
	switch fg & vt.AttrCMMask {
	case vt.AttrCMRGB:
		v := fg & vt.AttrRGBMask
		return float32(v>>16&0xff) / 255, float32(v>>8&0xff) / 255, float32(v&0xff) / 255
	case vt.AttrCMP16, vt.AttrCMP256:
		return xterm256(int(fg & vt.AttrPColorMask))
	}
	return 0.85, 0.85, 0.85
}

// xterm256 is a palette color, as xterm draws it.
func xterm256(i int) (r, g, b float32) {
	base := [16][3]uint8{
		{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
		{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	var c [3]uint8
	switch {
	case i < 16:
		c = base[max(i, 0)]
	case i < 232:
		lv := [6]uint8{0, 95, 135, 175, 215, 255}
		i -= 16
		c = [3]uint8{lv[i/36], lv[i/6%6], lv[i%6]}
	default:
		g := uint8(8 + 10*(min(i, 255)-232))
		c = [3]uint8{g, g, g}
	}
	return float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255
}

// VTColor is a terminal cell's color as three floats, or def where the cell
// has the terminal's default.
func VTColor(c uint32, def [3]float32) [3]float32 {
	if c&vt.AttrCMMask == 0 {
		return def
	}
	r, g, b := vtColor(c)
	return [3]float32{r, g, b}
}
