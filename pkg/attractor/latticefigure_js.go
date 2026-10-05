//go:build js && wasm

package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/lattice"
)

// Any model in the Lattice: ROWS's L.
//
// The model runs as it always does, and what it uploads — a trail, a mesh of
// lines, a cloud of dots — is recorded rather than drawn (the beam's capture,
// beam.capturing). That is a figure, which the lattice programs draw as they
// draw a solid, each its own slice: lit where the figure passes through a
// voxel, written as the way it runs across the sheet. Every program is handed
// the same figure (lattice.Program.Show), as a real one would read it with
// -figure, and it is voxelized once for all of them.
//
// The model keeps its own knobs in the bank, so it is played as ever, and the
// view turns as TURN says: BOTH the stack with the model in it, LATTICE the
// stack round a model held still, SOLID the model inside a stack held square.

// latticeNoFigure are the models that draw no figure a lattice could take:
// pictures and terminals, which are flat already.
var latticeNoFigure = map[string]bool{
	"terminal": true, "termanim": true, "hostterm": true, "desk": true,
}

// latticeDraws: this frame the model is drawn by the lattice.
func latticeDraws(mode string) bool {
	return lat.figOn && mode != "lattice" && !isTexturePlane(mode) && !latticeNoFigure[mode]
}

// latticeGenerate runs the model's generator with its uploads recorded, gives
// the lattice what it drew, and draws the lattice.
func latticeGenerate(fn func()) {
	beam.capturing = true
	fn()
	beam.capturing = false
	lat.takeFigure()
	lat.frame(true)
}

// figureEvery is how often the figure is handed on, in milliseconds: as often
// as the programs draw, since a figure handed on more often than that is
// voxelized and never shown.
const figureEvery = 1000.0 / latticeFPS

// takeFigure makes the figure from what the model last uploaded, scaled into
// the volume, if the programs are due another.
func (l *latticeModel) takeFigure() {
	if gpu.uploadSeq == l.figSeq || frameNowMs-l.figAt < figureEvery {
		return
	}
	l.figSeq, l.figAt = gpu.uploadSeq, frameNowMs
	v, stride := gpu.verts, gpu.stride
	if stride < 3 || len(v) < stride {
		l.fig.Set(nil)
		return
	}
	// The scale follows the model's size: out at once, back in slowly, so a
	// trail that grows and shrinks does not breathe in and out of the volume.
	most := float32(0)
	for i := 0; i+2 < len(v); i += stride {
		most = max(most, abs32(v[i]), abs32(v[i+1]), abs32(v[i+2]))
	}
	if most > l.figExt || l.figExt == 0 {
		l.figExt = most
	} else {
		l.figExt = max(most, l.figExt*0.99)
	}
	s := 0.92 / float64(max(l.figExt, 1e-6))
	pt := func(i int) lattice.Vec3 {
		return lattice.Vec3{float64(v[i]) * s, float64(v[i+1]) * s, float64(v[i+2]) * s}
	}
	// The programs read the figure only while drawing from it, inside the
	// frame it was set in, so the same storage serves every frame.
	l.figPts, l.figLens = l.figPts[:0], l.figLens[:0]
	add := func(from int) { l.figLens = append(l.figLens, len(l.figPts)-from) }
	t := gpu.lastTrace
	switch {
	case stride == 3 && len(gpu.indices) > 1: // a mesh: pairs of indices
		for k := 0; k+1 < len(gpu.indices); k += 2 {
			a, b := int(gpu.indices[k])*3, int(gpu.indices[k+1])*3
			if b+2 < len(v) && a+2 < len(v) {
				from := len(l.figPts)
				l.figPts = append(l.figPts, pt(a), pt(b))
				add(from)
			}
		}
	case stride == 4 && t.ok && t.mode.Equal(glctx.Types.LineStrip):
		from := len(l.figPts)
		for i := t.first; i < t.first+t.n && i*4+2 < len(v); i++ {
			l.figPts = append(l.figPts, pt(i*4))
		}
		add(from)
	case stride == 4 && t.ok && t.mode.Equal(glctx.Types.Lines):
		for i := t.first; i+1 < t.first+t.n && (i+1)*4+2 < len(v); i += 2 {
			from := len(l.figPts)
			l.figPts = append(l.figPts, pt(i*4), pt((i+1)*4))
			add(from)
		}
	case stride == 4 && t.ok: // dots, or anything else: its vertices
		for i := t.first; i < t.first+t.n && i*4+2 < len(v); i++ {
			from := len(l.figPts)
			l.figPts = append(l.figPts, pt(i*4))
			add(from)
		}
	}
	// Each line is a run of figPts, cut once append has stopped moving it.
	l.figLines = l.figLines[:0]
	at := 0
	for _, k := range l.figLens {
		l.figLines = append(l.figLines, l.figPts[at:at+k:at+k])
		at += k
	}
	l.fig.Set(l.figLines)
}

func abs32(x float32) float32 { return float32(math.Abs(float64(x))) }
