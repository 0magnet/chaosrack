//go:build js && wasm

package attractor

import (
	"io"
	"math"
	"slices"
	"strconv"
	"syscall/js"
	"time"
	"unicode"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/lattice"
	"github.com/0magnet/xterm-go/vt"
	"github.com/go-gl/mathgl/mgl32"
)

// The Lattice: a solid drawn in depth by terminals.
//
// github.com/0magnet/lattice is an ordinary terminal program that draws one
// cross-section of a turning solid. Here 3N copies of it run, each in a
// terminal of its own, and the terminals are stacked in space as transparent
// sheets: N across each axis, three families each perpendicular to the other
// two. Whichever way the stack is turned one family faces the viewer and the
// other two are edge on, so the solid is seen in depth from every side.
//
// THE TERMINALS ARE REAL. Each is a vt.Terminal, xterm-go's emulator without
// its renderer, and each program writes into it exactly the bytes it writes
// into any terminal: the same flags, the same escape sequences, parsed the
// same way. Only the drawing is this file's — every sheet's screen read cell
// by cell and drawn as glyphs on a clear sheet — because a page may hold about
// sixteen WebGL contexts and a lattice is dozens of terminals. A cell left at
// the default background is clear, which is what a translucent terminal
// emulator does with it too.
//
// THE CLOCK IS THE WALL'S. The programs turn the solid by the time of day, so
// a lattice run in real terminals beside the page is in step with this one.

// latticeModel is the Lattice: its knobs, its sheets and how they are drawn.
type latticeModel struct {
	shapeF, styleF, nF, spinF, tiltF float32
	lookF, opacF, stacksF, turnF     float32

	sheets []latticeSheet
	sig    string // the knobs the sheets were started with
	fitted bool
	rr     int // the program a frame starts with, round robin

	// TURN: the matrix the stack is drawn with, and the pose last sent to
	// the programs.
	stackMat mgl32.Mat4
	sentPose lattice.Quat

	atlas glyphAtlas
	pipe  glyphPipe

	// The background panes, made once per n (makePanes).
	paneVerts   []float32
	paneCenters [][3]float32
	paneAxis    []lattice.Axis
	paneN       int
	paneSheets  int

	// A frame: its pieces, sorted, and the vertices made from them.
	pieces []latticePiece
	sorted []latticePiece
	keys   []uint64
	keys2  []uint64
	verts  []float32
	last   latticeFrameKey // what the vertices on the GPU were made for
}

// latticeSheet is one terminal and the program drawing in it.
type latticeSheet struct {
	prog *lattice.Program
	term *vt.Terminal
	sh   lattice.Sheet
	next float64 // when this program next draws

	// writes counts what the program has written to the terminal, and built
	// is the count the cached pieces below were made at: the program sends
	// nothing when its screen has not changed, so an unchanged count is an
	// unchanged screen, and its pieces are made again only when it moves.
	writes *uint64
	built  uint64
	verts  []float32
	items  []latticeItem
}

// latticeItem is one cached piece of a sheet: six vertices at off in the
// sheet's verts, the center of the pane it lies in, and its layer.
type latticeItem struct {
	center [3]float32
	layer  int
	off    int
}

var lat = latticeModel{shapeF: latticeSphere, nF: 12, spinF: 20, tiltF: 20, opacF: 10}

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

func init() {
	registerGenerate("lattice", lat.generate)
	attractorParams["lattice"] = []paramDef{
		{"lattice-shape", "shp", &lat.shapeF, latticeSphere, 0, float32(len(lattice.ShapeNames()) - 1), 1},
		{"lattice-style", "styl", &lat.styleF, 0, 0, float32(len(latticeStyles) - 1), 1},
		{"lattice-n", "rows", &lat.nF, 12, 6, 24, 1},
		{"lattice-spin", "spin", &lat.spinF, 20, -90, 90, 1},
		{"lattice-tilt", "tilt", &lat.tiltF, 20, -90, 90, 1},
		{"lattice-look", "look", &lat.lookF, 0, 0, float32(len(latticeLooks) - 1), 1},
		{"lattice-opac", "opac", &lat.opacF, 10, 0, 100, 1},
		{"lattice-stacks", "stck", &lat.stacksF, 0, 0, float32(len(latticeStacks) - 1), 1},
		{"lattice-turn", "turn", &lat.turnF, 0, 0, float32(len(latticeTurns) - 1), 1},
	}
	paramLabels["lattice-shape"] = lattice.ShapeNames()
	paramLabels["lattice-style"] = latticeStyles
	paramLabels["lattice-look"] = latticeLooks
	paramLabels["lattice-stacks"] = latticeStacks
	paramLabels["lattice-turn"] = latticeTurns
	// How the sheets are shown is not the programs' business: changing it
	// must not restart the terminals.
	quietParams["lattice-look"] = true
	quietParams["lattice-opac"] = true
	quietParams["lattice-stacks"] = true
	quietParams["lattice-turn"] = true
}

// args are the command line each sheet's program is started with, and n the
// rows of its terminal: no -n, because the terminal's size is the resolution.
func (l *latticeModel) args() (n int, args []string) {
	n = max(2, int(l.nF+0.5))
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }
	return n, []string{
		"-shape", lattice.ShapeNames()[pick(l.shapeF, len(lattice.ShapeNames()))],
		"-style", latticeStyles[pick(l.styleF, len(latticeStyles))],
		"-spin", f(l.spinF),
		"-pitch", f(l.tiltF),
		"-fps", strconv.Itoa(latticeFPS),
	}
}

// latticeFPS is the programs' frame rate. A voxel picture changes in steps,
// so more than this draws the same screen again.
const latticeFPS = 15

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

// start gives every sheet a terminal and starts its program in it, the same
// way it would be started from a shell.
func (l *latticeModel) start(n int, args []string) {
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
			// The program learns the resolution from its terminal, as it would
			// from a window: as many voxels as rows, and as many sheets.
			prog.Resize(2*n, n)
			writes := new(uint64)
			if err := prog.Enter(vtWriter{term, writes}); err != nil {
				continue
			}
			l.sheets = append(l.sheets, latticeSheet{prog: prog, term: term, sh: prog.Sheet(), writes: writes, built: ^uint64(0)})
			_ = prog.Frame(time.Now()) //nolint:errcheck // a first picture, before the staggering starts; vtWriter cannot fail
		}
	}
	for i := range l.sheets {
		l.sheets[i].next = frameNowMs + 1000.0/latticeFPS*float64(i)/float64(len(l.sheets))
	}
	l.last = latticeFrameKey{}
	l.sentPose = lattice.Quat{} // new programs: tell them again
}

// generate is the model's frame: the programs draw when their interval is up,
// and every sheet's screen is drawn as it stands.
func (l *latticeModel) generate() {
	n, args := l.args()
	if sig := joinArgs(args); sig != l.sig {
		l.sig = sig
		l.start(n, args)
	}
	l.turn()
	m := l.stackMat
	look := lattice.Vec3{float64(m[2]), float64(m[6]), float64(m[10])} // toward the viewer, in the stack's frame
	on := l.families(look)
	// Each program draws on its own interval, and the intervals are staggered
	// across the sheets, so a frame carries a share of them rather than every
	// fourth frame carrying all: separate terminals, as they would be.
	// And within a budget of time a frame, round robin from where the last
	// frame stopped: on a busy machine the programs fall behind, as programs
	// in terminals do, rather than every one of them running every frame and
	// taking the rack down with them. A family that is not shown does not run.
	interval := 1000.0 / latticeFPS
	now := time.Now()
	deadline := now.Add(6 * time.Millisecond)
	for k := range len(l.sheets) {
		i := (l.rr + k) % len(l.sheets)
		s := &l.sheets[i]
		if !on[s.sh.Axis] || frameNowMs < s.next {
			continue
		}
		if time.Now().After(deadline) {
			l.rr = i
			break
		}
		_ = s.prog.Frame(now) //nolint:errcheck // vtWriter cannot fail
		if s.next += interval; s.next < frameNowMs {
			s.next = frameNowMs + interval // fallen behind: start again from now
		}
	}
	if !l.fitted {
		l.fitted = true
		view.fitOverride, view.fitExtent = latticeExtent, latticeExtent
		dist := fitDistFor(latticeExtent)
		view.initDist, view.defaultDist = dist, dist
		view.updateViewMatrix()
	}
	l.draw(n, look, on)
}

// turn sets what the view turns (TURN): the matrix the stack is drawn with,
// and the pose every program is sent when it changes.
func (l *latticeModel) turn() {
	q := mgl32.Mat4ToQuat(view.modelMat)
	vq := lattice.Quat{float64(q.W), float64(q.V[0]), float64(q.V[1]), float64(q.V[2])} // the view's turn
	l.stackMat, l.pipe.model = view.modelMat, &l.stackMat
	pose := lattice.Identity
	switch latticeTurns[pick(l.turnF, len(latticeTurns))] {
	case "lattice": // the solid turned back by as much as the stack: still, in the room
		pose = lattice.Quat{vq[0], -vq[1], -vq[2], -vq[3]}
	case "solid": // the stack square to the screen, the view's turn the solid's
		pose, l.stackMat = vq, mgl32.Ident4()
	}
	if pose == l.sentPose {
		return
	}
	l.sentPose = pose
	seq := []byte(lattice.PoseSequence(pose))
	for _, s := range l.sheets {
		s.prog.Input(seq)
	}
}

// families are the families STACKS shows, seen along look.
func (l *latticeModel) families(look lattice.Vec3) (on [3]bool) {
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

func joinArgs(a []string) string {
	s := ""
	for _, x := range a {
		s += x + " "
	}
	return s
}

// latticeExtent is how far the camera is fitted to: the volume's corners,
// nearly, since it turns inside a cube of ±1.
const latticeExtent = 1.4

// armFit makes the next frame frame the camera on the volume.
func (l *latticeModel) armFit() { l.fitted = false }

// How the sheets are put together on screen: LOOK.
//
// GLASS is how a desktop puts translucent terminals together, and the
// default. A terminal's transparency is its emulator's setting, never the
// program's: kitty's background_opacity, alacritty's window.opacity, a
// profile's "transparent background". It applies to the DEFAULT background —
// what a program gets by never setting one (SGR 49) — and leaves the text
// opaque; kitty and foot draw an explicit background color opaque as well,
// and so does this. The compositor then lays each window over what is behind
// it, back to front, by ordinary "over" blending. OPAC is that setting, for
// every sheet; the background is black.
//
// LIGHT is not a terminal at all but a volumetric display: no backgrounds,
// every glyph added to what is behind it like light, and each family faded by
// the square of the cosine to the view (which over the three families sums to
// one) so a voxel, drawn in three sheets, is as bright from any side. The
// fade is the alpha, which additive blending makes the brightness.
//
// GLASS HAS TO BE SORTED BY THE PIECE. Windows on a desktop are stacked, but
// these cut through each other, so no order of whole sheets is right from any
// side. They only meet along the lines through the voxel centers, though, so a
// sheet cut along those lines is (n+1)² panes that touch no other sheet, and
// every character's half lies inside exactly one pane: panes end at voxel
// centers, characters at voxel centers and edges. Each piece is sorted by its
// PANE's depth, then by layer — background, an explicit background color, the
// glyph — so a glyph is never put behind its own window's background.
//
// NOTHING IS DONE TWICE. A sheet's pieces are made when its screen changes;
// a frame computes only each piece's depth, sorts, and copies; and a frame
// whose view, settings and screens are all as they were is the last one drawn
// again, without even an upload.

// latticeLooks are LOOK's positions.
var latticeLooks = []string{"glass", "light"}

// latticePiece is one flat piece in a frame: six vertices at off, in the
// panes (sheet -1) or in a sheet's cache, its pane's nearness to the viewer,
// and its layer.
type latticePiece struct {
	near  float32
	layer int
	sheet int
	off   int
}

// The layers on one pane, back to front.
const (
	layerBackground = iota
	layerCellBackground
	layerGlyph
)

// latticeFrameKey is everything a frame's vertices depend on.
type latticeFrameKey struct {
	look   lattice.Vec3
	light  bool
	opac   float32
	on     [3]bool
	writes uint64
	ok     bool
}

// draw puts every shown sheet in the scene.
func (l *latticeModel) draw(n int, look lattice.Vec3, on [3]bool) {
	light := pick(l.lookF, len(latticeLooks)) == 1
	opac := float32(max(0, min(l.opacF, 100))) / 100
	cuts, mid := latticeCuts(n)
	su, sv := l.atlas.solid()

	key := latticeFrameKey{look: look, light: light, opac: opac, on: on, ok: true}
	for i := range l.sheets {
		s := &l.sheets[i]
		if !on[s.sh.Axis] {
			continue
		}
		if s.built != *s.writes {
			l.makeSheet(s, n, mid, su, sv)
		}
		key.writes += *s.writes
	}
	l.atlas.upload()
	if key == l.last {
		l.pipe.redraw(l.atlas.tex, light)
		return
	}
	l.last = key

	near := func(c [3]float32) float32 {
		return float32(float64(c[0])*look[0] + float64(c[1])*look[1] + float64(c[2])*look[2])
	}
	l.pieces = l.pieces[:0]
	if !light && opac > 0 {
		if l.paneN != n || l.paneSheets != len(l.sheets) {
			l.makePanes(n, cuts, mid, su, sv)
		}
		for j, c := range l.paneCenters {
			if on[l.paneAxis[j]] {
				l.pieces = append(l.pieces, latticePiece{near: near(c), layer: layerBackground, sheet: -1, off: j * 6 * glyphStride})
			}
		}
	}
	var weight [3]float32 // the alpha each family's glyphs are drawn at
	for a := range 3 {
		weight[a] = 1
		if light {
			weight[a] = float32(look[a] * look[a])
		}
	}
	for i := range l.sheets {
		s := &l.sheets[i]
		if !on[s.sh.Axis] || (light && weight[s.sh.Axis] < 0.01) {
			continue
		}
		for _, it := range s.items {
			if light && it.layer == layerCellBackground {
				continue // light has no backgrounds of any kind
			}
			l.pieces = append(l.pieces, latticePiece{near: near(it.center), layer: it.layer, sheet: i, off: it.off})
		}
	}

	// Back to front on glass; in any order as light, which only adds.
	if !light {
		l.sortPieces()
	}
	l.verts = l.verts[:0]
	for _, p := range l.pieces {
		src, alpha := l.paneVerts, opac
		if p.sheet >= 0 {
			src, alpha = l.sheets[p.sheet].verts, weight[l.sheets[p.sheet].sh.Axis]
		}
		at := len(l.verts)
		l.verts = append(l.verts, src[p.off:p.off+6*glyphStride]...)
		for k := range 6 {
			l.verts[at+k*glyphStride+8] = alpha
		}
	}
	l.pipe.draw(l.verts, l.atlas.tex, light)
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

// makeSheet makes sheet s's pieces from its screen as it stands: each glyph,
// and each explicit background color, in halves that lie in one pane each.
func (l *latticeModel) makeSheet(s *latticeSheet, n int, mid func(int) float64, su, sv float32) {
	s.built = *s.writes
	s.verts, s.items = s.verts[:0], s.items[:0]
	b := lattice.BasisOf(s.sh.Axis)
	plane := b.Normal.Scale(cellCenter(s.sh.Slice, n))
	center := func(pu, pv int) [3]float32 {
		c := plane.Add(b.Right.Scale(mid(pu))).Add(b.Up.Scale(mid(pv)))
		return [3]float32{float32(c[0]), float32(c[1]), float32(c[2])}
	}
	add := func(c [3]float32, layer int, u0, u1, v0, v1 float64, s0, t0, s1, t1, r, g, bl float32) {
		s.items = append(s.items, latticeItem{center: c, layer: layer, off: len(s.verts)})
		s.verts = appendQuad(s.verts, plane, b, u0, u1, v0, v1, s0, t0, s1, t1, r, g, bl)
	}
	hw, hh := 1/float64(2*n), 1/float64(n) // half a character, across and up
	buf := s.term.Buffer()
	for r := range n {
		if buf.YDisp+r >= buf.Lines.Length() {
			break
		}
		line := buf.Lines.Get(buf.YDisp + r)
		if line == nil {
			continue
		}
		vc := 1 - (float64(r)+0.5)/float64(n)*2
		k := n - 1 - r // the row's voxel, counted up from the bottom
		for col := range 2 * n {
			uc := (float64(col)+0.5)/float64(2*n)*2 - 1
			pu := col/2 + col%2 // the pane a character is in, across
			if bg := line.GetBg(col); bg&vt.AttrCMMask != 0 {
				r0, g0, b0 := vtColor(bg)
				add(center(pu, k+1), layerCellBackground, uc-hw, uc+hw, vc, vc+hh, su, sv, su, sv, r0, g0, b0)
				add(center(pu, k), layerCellBackground, uc-hw, uc+hw, vc-hh, vc, su, sv, su, sv, r0, g0, b0)
			}
			cp := line.GetCodePoint(col)
			if cp <= ' ' || cp > unicode.MaxRune {
				continue
			}
			u0, v0, u1, v1, ok := l.atlas.slot(rune(cp)) //nolint:gosec // bounded by unicode.MaxRune just above
			if !ok {
				continue
			}
			cr, cg, cb := vtColor(line.GetFg(col))
			vm := (v0 + v1) / 2
			// The top and bottom halves, each in its own pane.
			add(center(pu, k+1), layerGlyph, uc-hw, uc+hw, vc, vc+hh, u0, v0, u1, vm, cr, cg, cb)
			add(center(pu, k), layerGlyph, uc-hw, uc+hw, vc-hh, vc, u0, vm, u1, v1, cr, cg, cb)
		}
	}
}

// makePanes makes every sheet's background panes, and their centers, for
// draw to put in order: the panes are cut along the lines where the other
// families cross, so no pane crosses another sheet.
func (l *latticeModel) makePanes(n int, cuts []float64, mid func(int) float64, su, sv float32) {
	l.paneVerts, l.paneCenters, l.paneAxis = l.paneVerts[:0], l.paneCenters[:0], l.paneAxis[:0]
	for _, s := range l.sheets {
		b := lattice.BasisOf(s.sh.Axis)
		plane := b.Normal.Scale(cellCenter(s.sh.Slice, n))
		for pv := range n + 1 {
			for pu := range n + 1 {
				l.paneVerts = appendQuad(l.paneVerts, plane, b, cuts[pu], cuts[pu+1], cuts[pv], cuts[pv+1], su, sv, su, sv, 0, 0, 0)
				c := plane.Add(b.Right.Scale(mid(pu))).Add(b.Up.Scale(mid(pv)))
				l.paneCenters = append(l.paneCenters, [3]float32{float32(c[0]), float32(c[1]), float32(c[2])})
				l.paneAxis = append(l.paneAxis, s.sh.Axis)
			}
		}
	}
	l.paneN, l.paneSheets = n, len(l.sheets)
}

// appendQuad appends the rectangle u0..u1 by v0..v1 of the sheet whose plane
// passes through plane, textured from s0,t0 (its top left) to s1,t1, in one
// color. Its alpha is set as it is drawn.
func appendQuad(dst []float32, plane lattice.Vec3, b lattice.Basis, u0, u1, v0, v1 float64, s0, t0, s1, t1, r, g, bl float32) []float32 {
	at := func(u, v float64) [3]float32 {
		p := plane.Add(b.Right.Scale(u)).Add(b.Up.Scale(v))
		return [3]float32{float32(p[0]), float32(p[1]), float32(p[2])}
	}
	tl, tr, bot, br := at(u0, v1), at(u1, v1), at(u0, v0), at(u1, v0)
	put := func(p [3]float32, s, t float32) {
		dst = append(dst, p[0], p[1], p[2], s, t, r, g, bl, 1)
	}
	put(tl, s0, t0)
	put(bot, s0, t1)
	put(tr, s1, t0)
	put(tr, s1, t0)
	put(bot, s0, t1)
	put(br, s1, t1)
	return dst
}

// sortPieces puts the pieces in order back to front: by depth, then by
// layer, then in the order they were made. A stable radix sort on depth and
// layer, because a comparison sort calls a comparator a million times a frame
// at n = 24, which under wasm was a third of the frame; a radix sort calls
// nothing, and being stable it keeps the order they were made in for free.
func (l *latticeModel) sortPieces() {
	n := len(l.pieces)
	if cap(l.keys) < n {
		l.keys, l.keys2 = make([]uint64, n), make([]uint64, n)
	}
	if cap(l.sorted) < n {
		l.sorted = make([]latticePiece, n)
	}
	keys, tmp := l.keys[:n], l.keys2[:n]
	for i, p := range l.pieces {
		// A float's bits, made to order as the float does: flip a negative's
		// every bit, set a positive's sign bit. The piece's index rides below
		// bit 32, where the sort does not look.
		b := math.Float32bits(p.near)
		if b&(1<<31) != 0 {
			b = ^b
		} else {
			b |= 1 << 31
		}
		keys[i] = (uint64(b)<<2|uint64(p.layer))<<32 | uint64(i) //nolint:gosec // i indexes l.pieces: small and never negative
	}
	var count [1 << 12]int
	for shift := 32; shift < 32+36; shift += 12 {
		clear(count[:])
		for _, k := range keys {
			count[k>>shift&(1<<12-1)]++
		}
		sum := 0
		for d, c := range count {
			count[d], sum = sum, sum+c
		}
		for _, k := range keys {
			d := k >> shift & (1<<12 - 1)
			tmp[count[d]] = k
			count[d]++
		}
		keys, tmp = tmp, keys
	}
	sorted := l.sorted[:n]
	for i, k := range keys {
		sorted[i] = l.pieces[k&(1<<32-1)]
	}
	l.pieces, l.sorted = sorted, l.pieces
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

// glyphAtlas is every character drawn so far, white on clear, in one texture:
// a glyph is drawn into it the first time a sheet shows it.
type glyphAtlas struct {
	canvas, ctx, tex js.Value
	slots            map[rune]int
	dirty            bool
}

const (
	atlasSize       = 1024
	atlasCellW      = 32 // a character cell, twice as tall as wide
	atlasCellH      = 64
	atlasCols       = atlasSize / atlasCellW
	atlasRows       = atlasSize / atlasCellH
	atlasFontPx     = 54
	atlasCellsTotal = atlasCols * atlasRows
)

// slot is r's place in the atlas as texture coordinates, drawn now if new.
func (a *glyphAtlas) slot(r rune) (u0, v0, u1, v1 float32, ok bool) {
	if a.slots == nil {
		a.canvas = dom.Doc.Call("createElement", "canvas")
		a.canvas.Set("width", atlasSize)
		a.canvas.Set("height", atlasSize)
		a.ctx = a.canvas.Call("getContext", "2d")
		a.ctx.Set("font", strconv.Itoa(atlasFontPx)+"px monospace")
		a.ctx.Set("textAlign", "center")
		a.ctx.Set("textBaseline", "middle")
		a.ctx.Set("fillStyle", "#fff")
		a.slots = map[rune]int{}
		// Slot 0 is solid, for what is a color and not a character: a
		// background.
		a.ctx.Call("fillRect", 0, 0, atlasCellW, atlasCellH)
		a.slots[solidRune] = 0
	}
	i, seen := a.slots[r]
	if !seen {
		if len(a.slots) >= atlasCellsTotal {
			return 0, 0, 0, 0, false
		}
		i = len(a.slots)
		a.slots[r] = i
		x, y := i%atlasCols*atlasCellW, i/atlasCols*atlasCellH
		a.ctx.Call("fillText", string(r), x+atlasCellW/2, y+atlasCellH/2)
		a.dirty = true
	}
	x, y := float32(i%atlasCols*atlasCellW), float32(i/atlasCols*atlasCellH)
	return x / atlasSize, y / atlasSize, (x + atlasCellW) / atlasSize, (y + atlasCellH) / atlasSize, true
}

// solidRune is the atlas's solid cell, which no character can be.
const solidRune rune = -1

// solid is a texture coordinate inside the solid cell.
func (a *glyphAtlas) solid() (s, t float32) {
	u0, v0, u1, v1, _ := a.slot(solidRune)
	return (u0 + u1) / 2, (v0 + v1) / 2
}

// upload hands the atlas to the GPU when a glyph has been added to it.
func (a *glyphAtlas) upload() {
	gl := glctx.GL
	if a.tex.IsUndefined() || a.tex.IsNull() {
		if a.canvas.IsUndefined() {
			return
		}
		a.tex = gl.Call("createTexture")
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), a.tex)
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
		a.dirty = true
	}
	if !a.dirty {
		return
	}
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), a.tex)
	gl.Call("pixelStorei", gl.Get("UNPACK_FLIP_Y_WEBGL"), false)
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), a.canvas)
	a.dirty = false
}

const glyphVertSrc = `
	attribute vec3 aPos;
	attribute vec2 aUV;
	attribute vec4 aCol;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	varying vec2 vUV;
	varying vec4 vCol;
	void main() {
		vUV = aUV;
		vCol = aCol;
		gl_Position = Pmatrix * Vmatrix * Mmatrix * vec4(aPos, 1.0);
	}
`

const glyphFragSrc = `
	precision mediump float;
	uniform sampler2D uTex;
	varying vec2 vUV;
	varying vec4 vCol;
	void main(void) {
		gl_FragColor = vec4(vCol.rgb, vCol.a * texture2D(uTex, vUV).a);
	}
`

// glyphPipe draws textured, colored quads through the camera, added to what
// is already on screen like light.
type glyphPipe struct {
	program, buf             js.Value
	aPos, aUV, aCol          js.Value
	pmat, vmat, mmat, sample js.Value
	ready                    bool
	// The upload's JS side, kept and grown rather than made every frame: at
	// n = 24 a frame is megabytes.
	u8, f32 js.Value
	cap     int
	n       int         // the vertices last uploaded
	model   *mgl32.Mat4 // the matrix the quads are placed by: the view's, unless TURN holds the stack still
}

func (p *glyphPipe) init() {
	gl := glctx.GL
	vs := gl.Call("createShader", glctx.Types.VertexShader)
	gl.Call("shaderSource", vs, glyphVertSrc)
	gl.Call("compileShader", vs)
	fs := gl.Call("createShader", glctx.Types.FragmentShader)
	gl.Call("shaderSource", fs, glyphFragSrc)
	gl.Call("compileShader", fs)
	p.program = gl.Call("createProgram")
	gl.Call("attachShader", p.program, vs)
	gl.Call("attachShader", p.program, fs)
	gl.Call("linkProgram", p.program)
	p.aPos = gl.Call("getAttribLocation", p.program, "aPos")
	p.aUV = gl.Call("getAttribLocation", p.program, "aUV")
	p.aCol = gl.Call("getAttribLocation", p.program, "aCol")
	p.pmat = gl.Call("getUniformLocation", p.program, "Pmatrix")
	p.vmat = gl.Call("getUniformLocation", p.program, "Vmatrix")
	p.mmat = gl.Call("getUniformLocation", p.program, "Mmatrix")
	p.sample = gl.Call("getUniformLocation", p.program, "uTex")
	p.buf = gl.Call("createBuffer")
	p.ready = true
}

// glyphStride is the floats per vertex: position, texture coordinate, color
// with its alpha.
const glyphStride = 9

// draw draws the quads in v over what is on screen: as light, added to it,
// or as glass, over it.
func (p *glyphPipe) draw(v []float32, tex js.Value, light bool) {
	if !p.ready {
		p.init()
	}
	p.n = len(v) / glyphStride
	if p.n > 0 {
		glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, p.buf)
		if len(v) > p.cap {
			p.cap = len(v) + len(v)/2
			p.u8 = js.Global().Get("Uint8Array").New(p.cap * 4)
			p.f32 = js.Global().Get("Float32Array").New(p.u8.Get("buffer"), 0, p.cap)
		}
		js.CopyBytesToJS(p.u8, sliceToByteSlice(v))
		glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, p.f32.Call("subarray", 0, len(v)), glctx.Types.DynamicDraw)
	}
	p.redraw(tex, light)
}

// redraw draws what was last uploaded again, through the camera as it is now.
func (p *glyphPipe) redraw(tex js.Value, light bool) {
	n := p.n
	if !p.ready || n == 0 || tex.IsUndefined() {
		return
	}
	gl := glctx.GL
	gl.Call("useProgram", p.program)
	gl.Call("uniformMatrix4fv", p.pmat, false, texp.mat4ToTyped(&gpu.proj))
	gl.Call("uniformMatrix4fv", p.vmat, false, texp.mat4ToTyped(&view.viewMat))
	model := &view.modelMat
	if p.model != nil {
		model = p.model
	}
	gl.Call("uniformMatrix4fv", p.mmat, false, texp.mat4ToTyped(model))
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), tex)
	gl.Call("uniform1i", p.sample, 0)
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, p.buf)
	for _, a := range []struct {
		loc       js.Value
		size, off int
	}{{p.aPos, 3, 0}, {p.aUV, 2, 3}, {p.aCol, 4, 5}} {
		gl.Call("enableVertexAttribArray", a.loc)
		gl.Call("vertexAttribPointer", a.loc, a.size, glctx.Types.Float, false, glyphStride*4, a.off*4)
	}
	gl.Call("disable", glctx.Types.DepthTest)
	gl.Call("enable", gl.Get("BLEND"))
	if light {
		gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE"))
	} else {
		gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	}
	gl.Call("drawArrays", gl.Get("TRIANGLES"), 0, n)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
	gl.Call("enable", glctx.Types.DepthTest)
	gl.Call("disableVertexAttribArray", p.aUV)
	gl.Call("disableVertexAttribArray", p.aCol)
	// The attractor program's own state: its buffer and its program, which
	// the next model's draw expects to find bound.
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, gpu.vbuf)
	gl.Call("useProgram", gpu.program)
	gpu.staticDirty = true
}
