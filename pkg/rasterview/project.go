package rasterview

import (
	"image"
	"math"
)

// projector places model points in a frame: a View's pose, perspective,
// pan and scale, for a frame of a size and a model of a reach.
type projector struct {
	v                      View
	cx, cy, r, aspect      float64
	sinX, cosX, sinY, cosY float64
	sinZ, cosZ             float64
}

// projector frames a model reaching maxLen from the origin in a pw by ph
// frame: it fills Scale of the half-frame.
func (v View) projector(pw, ph int, maxLen float64) projector {
	scale := v.Scale
	if scale == 0 {
		scale = 0.85
	}
	p := projector{v: v, cx: float64(pw) / 2, cy: float64(ph) / 2, aspect: v.PixelAspect}
	// r is in the height's pixels; a pixel's width is aspect of that.
	if p.aspect <= 0 {
		p.aspect = 1
	}
	p.r = math.Min(p.cx*p.aspect, p.cy) * scale / maxLen
	p.sinX, p.cosX = math.Sin(v.AngleX), math.Cos(v.AngleX)
	p.sinY, p.cosY = math.Sin(v.AngleY), math.Cos(v.AngleY)
	p.sinZ, p.cosZ = math.Sin(v.AngleZ), math.Cos(v.AngleZ)
	return p
}

// at is where a model point lands in the frame, and its depth toward the
// viewer.
func (p projector) at(x0, y0, z0 float64) (sx, sy, z float64) {
	// Rz, then Ry, then Rx — Rx·Ry·Rz applied to a column vector.
	x1, y1 := x0*p.cosZ-y0*p.sinZ, x0*p.sinZ+y0*p.cosZ
	x2, z2 := x1*p.cosY+z0*p.sinY, -x1*p.sinY+z0*p.cosY
	y3, z3 := y1*p.cosX-z2*p.sinX, y1*p.sinX+z2*p.cosX

	persp := 1.0
	if p.v.Dist > 0 {
		persp = p.v.Dist / (p.v.Dist - z3)
	}
	return p.cx + (x2*persp+p.v.PanX)*p.r/p.aspect, p.cy - (y3*persp+p.v.PanY)*p.r, z3
}

// Quad is a picture on a flat quad in model space.
type Quad struct {
	// BL, BR and TL are its bottom-left, bottom-right and top-left corners;
	// the picture's bottom row lies along BL–BR.
	BL, BR, TL [3]float64
	Pic        *image.RGBA
	// Shift scrolls the picture left by that fraction of its width, wrapping
	// round: what was at Shift is at the quad's left edge, as the page's
	// textured shader offsets a scrolling texture.
	Shift float64
}

// RenderQuad draws q, posed and framed as Render poses and frames a model
// reaching maxLen from the origin: the quad's corners land where Render would
// put them as vertices. Each pixel takes the nearest of pic's, as the page samples a
// texture of yes-or-no cells. A quad seen from behind is drawn mirrored, as
// WebGL draws it with no culling.
//
// The quad is mapped as a parallelogram, which is exact for an orthographic
// view (Dist 0, every native scene's) and close for a distant camera.
func (v View) RenderQuad(dst *image.RGBA, q Quad, maxLen float64) {
	bl, br, tl, pic := q.BL, q.BR, q.TL, q.Pic
	if pic == nil {
		return
	}
	b := dst.Bounds()
	pw, ph := b.Dx(), b.Dy()
	tw, th := pic.Rect.Dx(), pic.Rect.Dy()
	if pw == 0 || ph == 0 || tw == 0 || th == 0 {
		return
	}
	if maxLen <= 0 {
		maxLen = 1
	}
	p := v.projector(pw, ph, maxLen)
	ox, oy, _ := p.at(bl[0], bl[1], bl[2])
	ux, uy, _ := p.at(br[0], br[1], br[2])
	vx, vy, _ := p.at(tl[0], tl[1], tl[2])
	ux, uy, vx, vy = ux-ox, uy-oy, vx-ox, vy-oy
	det := ux*vy - uy*vx
	if math.Abs(det) < 1e-9 {
		return // edge-on: a line of no width
	}
	x0 := math.Min(ox, math.Min(ox+ux, math.Min(ox+vx, ox+ux+vx)))
	x1 := math.Max(ox, math.Max(ox+ux, math.Max(ox+vx, ox+ux+vx)))
	y0 := math.Min(oy, math.Min(oy+uy, math.Min(oy+vy, oy+uy+vy)))
	y1 := math.Max(oy, math.Max(oy+uy, math.Max(oy+vy, oy+uy+vy)))
	for y := max(int(math.Floor(y0)), 0); y < min(int(math.Ceil(y1)), ph); y++ {
		for x := max(int(math.Floor(x0)), 0); x < min(int(math.Ceil(x1)), pw); x++ {
			// The pixel's center as s along bl–br and t along bl–tl.
			dx, dy := float64(x)+0.5-ox, float64(y)+0.5-oy
			s := (dx*vy - dy*vx) / det
			t := (ux*dy - uy*dx) / det
			if s < 0 || s >= 1 || t < 0 || t >= 1 {
				continue
			}
			u := s + q.Shift
			u -= math.Floor(u)
			c := pic.RGBAAt(pic.Rect.Min.X+min(int(u*float64(tw)), tw-1), pic.Rect.Min.Y+th-1-int(t*float64(th)))
			dst.SetRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
}
