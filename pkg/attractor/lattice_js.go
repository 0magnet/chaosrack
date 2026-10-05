//go:build js && wasm

package attractor

import (
	"io"
	"slices"
	"strconv"
	"syscall/js"
	"time"
	"unicode"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/lattice"
	"github.com/0magnet/xterm-go/vt"
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

	sheets []latticeSheet
	sig    string  // the knobs the sheets were started with
	nextMs float64 // when the programs next draw
	fitted bool

	atlas glyphAtlas
	pipe  glyphPipe
	verts []float32
}

// latticeSheet is one terminal and the program drawing in it.
type latticeSheet struct {
	prog *lattice.Program
	term *vt.Terminal
	sh   lattice.Sheet
}

var lat = latticeModel{shapeF: latticeSphere, nF: 12, spinF: 20, tiltF: 20}

// latticeSphere is the sphere's place on the shape knob, which is where it starts.
var latticeSphere = float32(max(0, slices.Index(lattice.ShapeNames(), "sphere")))

// latticeStyles are the styles in the order the knob turns through them.
var latticeStyles = []string{"lines", "ascii", "shade", "solid"}

func init() {
	registerGenerate("lattice", lat.generate)
	attractorParams["lattice"] = []paramDef{
		{"lattice-shape", "shp", &lat.shapeF, latticeSphere, 0, float32(len(lattice.ShapeNames()) - 1), 1},
		{"lattice-style", "styl", &lat.styleF, 0, 0, float32(len(latticeStyles) - 1), 1},
		{"lattice-n", "n", &lat.nF, 12, 6, 24, 1},
		{"lattice-spin", "spin", &lat.spinF, 20, -90, 90, 1},
		{"lattice-tilt", "tilt", &lat.tiltF, 20, -90, 90, 1},
	}
	paramLabels["lattice-shape"] = lattice.ShapeNames()
	paramLabels["lattice-style"] = latticeStyles
}

// args are the command line each sheet's program is started with.
func (l *latticeModel) args() (n int, args []string) {
	n = max(2, int(l.nF+0.5))
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }
	return n, []string{
		"-n", strconv.Itoa(n),
		"-shape", lattice.ShapeNames()[pick(l.shapeF, len(lattice.ShapeNames()))],
		"-style", latticeStyles[pick(l.styleF, len(latticeStyles))],
		"-spin", f(l.spinF),
		"-pitch", f(l.tiltF),
		"-fps", "30",
	}
}

// vtWriter is a terminal's input, as the program sees its terminal.
type vtWriter struct{ t *vt.Terminal }

func (w vtWriter) Write(p []byte) (int, error) {
	w.t.Write(p)
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
			if err := prog.Enter(vtWriter{term}); err != nil {
				continue
			}
			l.sheets = append(l.sheets, latticeSheet{prog: prog, term: term, sh: prog.Sheet()})
		}
	}
	l.nextMs = 0
}

// generate is the model's frame: the programs draw when their interval is up,
// and every sheet's screen is drawn as it stands.
func (l *latticeModel) generate() {
	n, args := l.args()
	if sig := joinArgs(args); sig != l.sig {
		l.sig = sig
		l.start(n, args)
	}
	if frameNowMs >= l.nextMs {
		now := time.Now()
		for _, s := range l.sheets {
			_ = s.prog.Frame(now) //nolint:errcheck // vtWriter cannot fail
		}
		l.nextMs = frameNowMs + float64(time.Second/30)/1e6
	}
	if !l.fitted {
		l.fitted = true
		const ext = 1.4 // the volume's corners, nearly: it turns inside a cube of ±1
		view.fitOverride, view.fitExtent = ext, ext
		dist := fitDistFor(ext)
		view.initDist, view.defaultDist = dist, dist
		view.updateViewMatrix()
	}
	l.draw(n)
}

func joinArgs(a []string) string {
	s := ""
	for _, x := range a {
		s += x + " "
	}
	return s
}

// armFit makes the next frame frame the camera on the volume.
func (l *latticeModel) armFit() { l.fitted = false }

// draw puts every sheet's glyphs in the scene. Each family is weighted by how
// squarely it faces the camera — the square of the cosine, which over the
// three families always adds up to one — so a voxel, drawn once in each of
// its three sheets, is as bright from any side.
func (l *latticeModel) draw(n int) {
	m := view.modelMat
	look := lattice.Vec3{float64(m[2]), float64(m[6]), float64(m[10])} // the view direction, in the model's frame
	l.verts = l.verts[:0]
	hw, hh := float32(1)/float32(2*n), float32(1)/float32(n) // half a character, across and up
	for _, s := range l.sheets {
		b := lattice.BasisOf(s.sh.Axis)
		c := look.Dot(b.Normal)
		w := float32(c * c)
		if w < 0.01 {
			continue
		}
		buf := s.term.Buffer()
		depth := b.Normal.Scale(cellCenter(s.sh.Slice, n))
		for r := range n {
			if buf.YDisp+r >= buf.Lines.Length() {
				break
			}
			line := buf.Lines.Get(buf.YDisp + r)
			if line == nil {
				continue
			}
			vc := float32(1 - (float64(r)+0.5)/float64(n)*2)
			for col := range min(2*n, line.GetTrimmedLength()) {
				cp := line.GetCodePoint(col)
				if cp <= ' ' || cp > unicode.MaxRune {
					continue
				}
				u0, v0, u1, v1, ok := l.atlas.slot(rune(cp)) //nolint:gosec // bounded by unicode.MaxRune just above
				if !ok {
					continue
				}
				uc := float32((float64(col)+0.5)/float64(2*n)*2 - 1)
				cr, cg, cb := vtColor(line.GetFg(col))
				l.quad(depth, b, uc, vc, hw, hh, u0, v0, u1, v1, cr*w, cg*w, cb*w)
			}
		}
	}
	l.atlas.upload()
	l.pipe.draw(l.verts, l.atlas.tex)
}

// cellCenter is the coordinate of the middle of slice i of n, as the program
// places it.
func cellCenter(i, n int) float64 { return (float64(i)+0.5)/float64(n)*2 - 1 }

// quad adds one glyph's two triangles: centered at (u, v) on the sheet whose
// plane is at depth, the texture's top at the sheet's up.
func (l *latticeModel) quad(depth lattice.Vec3, b lattice.Basis, u, v, hw, hh, u0, v0, u1, v1, r, g, bl float32) {
	at := func(du, dv float32) [3]float32 {
		p := depth.Add(b.Right.Scale(float64(u + du))).Add(b.Up.Scale(float64(v + dv)))
		return [3]float32{float32(p[0]), float32(p[1]), float32(p[2])}
	}
	tl, tr, bl0, br := at(-hw, hh), at(hw, hh), at(-hw, -hh), at(hw, -hh)
	put := func(p [3]float32, s, t float32) {
		l.verts = append(l.verts, p[0], p[1], p[2], s, t, r, g, bl)
	}
	put(tl, u0, v0)
	put(bl0, u0, v1)
	put(tr, u1, v0)
	put(tr, u1, v0)
	put(bl0, u0, v1)
	put(br, u1, v1)
}

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
	attribute vec3 aCol;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	varying vec2 vUV;
	varying vec3 vCol;
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
	varying vec3 vCol;
	void main(void) {
		gl_FragColor = vec4(vCol, texture2D(uTex, vUV).a);
	}
`

// glyphPipe draws textured, colored quads through the camera, added to what
// is already on screen like light.
type glyphPipe struct {
	program, buf             js.Value
	aPos, aUV, aCol          js.Value
	pmat, vmat, mmat, sample js.Value
	ready                    bool
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

// glyphStride is the floats per vertex: position, texture coordinate, color.
const glyphStride = 8

func (p *glyphPipe) draw(v []float32, tex js.Value) {
	n := len(v) / glyphStride
	if n == 0 || tex.IsUndefined() {
		return
	}
	if !p.ready {
		p.init()
	}
	gl := glctx.GL
	gl.Call("useProgram", p.program)
	gl.Call("uniformMatrix4fv", p.pmat, false, texp.mat4ToTyped(&gpu.proj))
	gl.Call("uniformMatrix4fv", p.vmat, false, texp.mat4ToTyped(&view.viewMat))
	gl.Call("uniformMatrix4fv", p.mmat, false, texp.mat4ToTyped(&view.modelMat))
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), tex)
	gl.Call("uniform1i", p.sample, 0)
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, p.buf)
	gl.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(v), glctx.Types.DynamicDraw)
	for _, a := range []struct {
		loc       js.Value
		size, off int
	}{{p.aPos, 3, 0}, {p.aUV, 2, 3}, {p.aCol, 3, 5}} {
		gl.Call("enableVertexAttribArray", a.loc)
		gl.Call("vertexAttribPointer", a.loc, a.size, glctx.Types.Float, false, glyphStride*4, a.off*4)
	}
	gl.Call("disable", glctx.Types.DepthTest)
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE"))
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
