package meshstl

import (
	"math"
	"sort"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// The rack itself, as something you could hold: a module is a front panel
// with a body behind it that slides into a frame, and controls standing off
// the front. Dimensions come from pkg/rackspec, so these models and the
// on-screen panel are the same object drawn twice.

// PanelThickness is the aluminum front panel. BodyInset keeps the case
// narrower than the panel, so that neighboring panels butt up against each
// other and not their cases.
//
// The depth comes from rackspec: a module is a Eurocard, so it runs 160 mm
// back from a 128.5 mm panel and is deeper than it is tall. An earlier
// version of this file had a 40 mm "body", which is the depth of the
// potentiometer behind the panel rather than the depth of the card the
// potentiometer is soldered to.
const (
	PanelThickness = 2.0
	BodyInset      = 3.0
)

// Control is something standing off the front of a panel: a knob, a pin, an
// LED, a button. Position is in panel coordinates — X across from the left
// edge, Y up from the bottom, both in millimeters.
type Control struct {
	X      float64     `json:"x"`
	Y      float64     `json:"y"`
	Kind   ControlKind `json:"kind"`
	Diam   float64     `json:"diam,omitempty"`   // the part's diameter, or its width if square
	Height float64     `json:"height,omitempty"` // how far it stands off the panel; 0 takes the kind's default
	// Lift raises the control off the panel face, for the upper stages of a
	// concentric stack. Zero means it sits on the panel.
	Lift float64 `json:"lift,omitempty"`
}

// ControlKind picks the shape a control is modeled as.
type ControlKind int

const (
	// KnobControl is a shallow truncated cone with a pointer flat — a collet
	// knob seen from the front.
	KnobControl ControlKind = iota
	// PinControl is a matrix pin: a short cylinder, barely proud of the panel.
	PinControl
	// LEDControl is a domed cylinder.
	LEDControl
	// ButtonControl is a square tactile-switch cap.
	ButtonControl
	// ToggleControl is a sub-miniature toggle: a panel bushing with a bat
	// lever leaning out of it.
	ToggleControl
	// JackControl is a hex-nutted socket — unused by the pin matrix, here so
	// a jackfield version can be modeled without inventing a shape for it.
	JackControl
)

// PanelOptions describes one module's panel.
type PanelOptions struct {
	HP       int       // width in HP; 0 means one slot (rackspec.ModuleHP)
	Controls []Control // what stands off the front
	Body     bool      // include the body behind the panel
	Seg      int       // segments per revolution on round parts; 0 means 24
}

// ModulePanel builds one rack module: a 3U panel of the given width, milled
// narrow for the seam exactly as the on-screen module is, optionally with the
// body behind it that gives it something to slide into a rack with.
//
// The panel lies in the XY plane with its lower-left corner at the origin and
// its front face at Z = PanelThickness, so several of them can be laid side
// by side by translating in X by whole HP.
func ModulePanel(o PanelOptions) Mesh {
	hp := o.HP
	if hp <= 0 {
		hp = rackspec.ModuleHP
	}
	seg := o.Seg
	if seg <= 0 {
		seg = 24
	}
	// The milled width: nominal HP less the seam, so that panel plus seam is
	// a whole number of HP and neighbors butt up on the grid.
	w := float64(hp)*rackspec.HP - rackspec.Seam
	h := rackspec.PanelHeight3U

	var m Mesh
	m.Append(Box(V3{0, 0, 0}, V3{w, h, PanelThickness}))

	if o.Body {
		m.Append(cardBody(w, h, seg))
	}

	for _, c := range o.Controls {
		m.Append(control(c, seg))
	}
	return m
}

// cardBody is what is actually behind the panel: a case wrapped around a
// Eurocard, with the board protruding at the back to meet the backplane.
//
// The case is built as four walls rather than a solid block, because a solid
// block is not a case — it has no inside for the board to be in, and it reads
// as a lump when the model is sectioned or printed.
func cardBody(w, h float64, seg int) Mesh {
	var m Mesh

	// The board sits centered on the panel's height, on the card guides.
	boardY0 := (h - rackspec.PCBHeight3U) / 2
	boardY1 := boardY0 + rackspec.PCBHeight3U

	x0, x1 := BodyInset, w-BodyInset
	y0, y1 := boardY0-rackspec.CaseWall*2, boardY1+rackspec.CaseWall*2
	zBack := -rackspec.CaseDepth

	// Four walls, open front and back: the front is the panel and the back is
	// where the board comes out.
	m.Append(Box(V3{x0, y0, zBack}, V3{x1, y0 + rackspec.CaseWall, 0}))
	m.Append(Box(V3{x0, y1 - rackspec.CaseWall, zBack}, V3{x1, y1, 0}))
	m.Append(Box(V3{x0, y0, zBack}, V3{x0 + rackspec.CaseWall, y1, 0}))
	m.Append(Box(V3{x1 - rackspec.CaseWall, y0, zBack}, V3{x1, y1, 0}))

	// The board itself, on the case's centerline, running the full Eurocard
	// depth — so it stands proud of the case by rackspec.CardEdge.
	zMid := (x0 + x1) / 2
	m.Append(Box(
		V3{zMid - rackspec.PCBThickness/2, boardY0, -rackspec.PCBDepth},
		V3{zMid + rackspec.PCBThickness/2, boardY1, -2},
	))

	// The edge connector: contacts on both faces of the protruding tongue, on
	// the 0.1 in pitch. Modeled as raised pads, which is what you would see.
	m.Append(edgeFingers(zMid, boardY0, boardY1))

	// A pair of card guides' worth of rail on the case, so the model shows
	// which way it slides in.
	for _, y := range []float64{y0, y1 - rackspec.CaseWall} {
		m.Append(Box(
			V3{x0 - 1, y, zBack + 6},
			V3{x0, y + rackspec.CaseWall, -6},
		))
		m.Append(Box(
			V3{x1, y, zBack + 6},
			V3{x1 + 1, y + rackspec.CaseWall, -6},
		))
	}
	_ = seg
	return m
}

// edgeFingers lays contacts along the protruding board edge. They stop short
// of the board's top and bottom, as a real connector's do.
func edgeFingers(xMid, y0, y1 float64) Mesh {
	var m Mesh
	const (
		fingerLen   = 8.0 // how far up the tongue a contact runs
		fingerProud = 0.1 // plating, barely proud of the board
		margin      = 6.0 // clear of the board edges
	)
	fingerW := rackspec.EdgeFingerPitch * 0.6
	z0 := -rackspec.PCBDepth
	z1 := z0 + fingerLen
	for y := y0 + margin; y+fingerW <= y1-margin; y += rackspec.EdgeFingerPitch {
		for _, s := range []float64{1, -1} {
			x := xMid + s*rackspec.PCBThickness/2
			lo, hi := x, x+s*fingerProud
			if lo > hi {
				lo, hi = hi, lo
			}
			m.Append(Box(V3{lo, y, z0}, V3{hi, y + fingerW, z1}))
		}
	}
	return m
}

// control models one part standing off the front face.
func control(c Control, seg int) Mesh {
	base := V3{c.X, c.Y, PanelThickness + c.Lift}
	d := c.Diam
	if d <= 0 {
		d = rackspec.KnobLarge
	}
	hgt := c.Height
	switch c.Kind {
	case KnobControl:
		if hgt <= 0 {
			hgt = 14 // a collet knob's body
		}
		var m Mesh
		// Slightly tapered, as a knob is, with a small skirt at the panel.
		m.Append(Cone(base, d/2, d/2*0.86, hgt, seg))
		m.Append(Cylinder(base, d/2*knobSkirt, 1.2, seg))
		// The pointer: a fin down one side, so the model shows which way the
		// knob is turned rather than being a featureless cylinder.
		// Measured from the knob's own base, not the panel's: on the upper
		// stage of a concentric stack those are different, and anchoring to
		// the panel drove the fin straight down through the knob below it.
		fin := d / 2 * 0.12
		m.Append(Box(
			V3{c.X - fin, c.Y, base[2]},
			V3{c.X + fin, c.Y + d/2*0.92, base[2] + hgt + 0.4},
		))
		return m
	case PinControl:
		if hgt <= 0 {
			hgt = 2.5
		}
		if c.Diam <= 0 {
			d = rackspec.PinHead
		}
		return Cylinder(base, d/2, hgt, seg)
	case LEDControl:
		if hgt <= 0 {
			hgt = 2.0
		}
		if c.Diam <= 0 {
			d = rackspec.LEDIndicator
		}
		var m Mesh
		m.Append(Cylinder(base, d/2, hgt, seg))
		m.Append(UVSphere(base.Add(V3{0, 0, hgt}), d/2, seg/2, seg))
		return m
	case ButtonControl:
		if hgt <= 0 {
			hgt = 1.6
		}
		if c.Diam <= 0 {
			d = rackspec.TactSwitch
		}
		return Box(
			V3{c.X - d/2, c.Y - d/2, PanelThickness},
			V3{c.X + d/2, c.Y + d/2, PanelThickness + hgt},
		)
	case ToggleControl:
		if hgt <= 0 {
			hgt = 10
		}
		if c.Diam <= 0 {
			d = rackspec.ToggleBushing
		}
		var m Mesh
		// The bushing, then the lever leaning off center — a toggle drawn
		// symmetrically is a toggle with no position, which is the one thing a
		// switch has to show.
		m.Append(Cylinder(base, d/2, 2, seg))
		lean := d * 0.35
		m.Append(Cone(base.Add(V3{lean / 3, 0, 2}), d/2*0.45, d/2*0.3, hgt-2, seg))
		return m
	case JackControl:
		if hgt <= 0 {
			hgt = 3.0
		}
		if c.Diam <= 0 {
			d = rackspec.JackHole35 + 2 // the nut, wider than the hole
		}
		var m Mesh
		m.Append(Cylinder(base, d/2, hgt, 6)) // six sides: a hex nut
		m.Append(Cylinder(base, rackspec.JackHole35/2*0.8, hgt+0.2, seg))
		return m
	}
	return Mesh{}
}

// PinMatrix lays out a grid of matrix pins on the rackspec pitch, centered on
// (cx, cy) — the patchbay, modeled.
func PinMatrix(cx, cy float64, cols, rows int) []Control {
	var out []Control
	w := float64(cols-1) * rackspec.PinPitch
	h := float64(rows-1) * rackspec.PinPitch
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			out = append(out, Control{
				X:    cx - w/2 + float64(c)*rackspec.PinPitch,
				Y:    cy + h/2 - float64(r)*rackspec.PinPitch,
				Kind: PinControl,
			})
		}
	}
	return out
}

// RailHeight is the frame member above and below a 3U opening.
const RailHeight = 8.0

// Rack builds a 19-inch card cage: rails a 3U opening apart with ears out to
// the full nineteen inches, card guides running back on the slot pitch, and a
// backplane across the rear carrying one connector per slot.
//
// It is the piece that shows what the numbers mean — the panel row is 84 HP,
// the frame around it is the rest of the nineteen inches, and the reason a
// module is deeper than it is tall is that it has to reach the back.
func Rack(rows int) Mesh { return RackWithHandles(rows, true) }

// RackWithHandles is Rack with the grab handles on its ears made optional.
func RackWithHandles(rows int, handles bool) Mesh {
	if rows < 1 {
		rows = 1
	}
	const railDepth = 12.0
	var m Mesh
	w := rackspec.PanelWidth19
	inset := (w - rackspec.RowWidth()) / 2 // the frame either side of the row
	bayH := 2*RailHeight + rackspec.PanelHeight3U

	for r := 0; r < rows; r++ {
		y := float64(r) * bayH

		// Bottom and top rails, spanning the full 19 inches.
		m.Append(Box(V3{0, y, -railDepth}, V3{w, y + RailHeight, 0}))
		m.Append(Box(
			V3{0, y + RailHeight + rackspec.PanelHeight3U, -railDepth},
			V3{w, y + bayH, 0},
		))
		// The side members, which are what makes the usable row 84 HP and not
		// the full width.
		m.Append(Box(V3{0, y, -railDepth}, V3{inset, y + bayH, 0}))
		m.Append(Box(V3{w - inset, y, -railDepth}, V3{w, y + bayH, 0}))

		// Card guides: a pair per slot, running the depth of the cage. These
		// are what a module actually rides in.
		boardY0 := y + RailHeight + (rackspec.PanelHeight3U-rackspec.PCBHeight3U)/2
		for i := 0; i <= rackspec.SlotsPerRow(); i++ {
			x := inset + float64(i)*rackspec.SlotPitch
			if x+2 > w-inset {
				break
			}
			for _, gy := range []float64{boardY0 - 3, boardY0 + rackspec.PCBHeight3U + 1} {
				m.Append(Box(
					V3{x - 1, gy, -rackspec.CaseDepth},
					V3{x + 1, gy + 2, -8},
				))
			}
		}

		// The backplane, and a connector body per slot for the cards to land
		// in. It sits just behind where the boards end.
		bpZ := -rackspec.PCBDepth - 2
		m.Append(Box(
			V3{inset, y + RailHeight, bpZ - rackspec.PCBThickness},
			V3{w - inset, y + RailHeight + rackspec.PanelHeight3U, bpZ},
		))
		for i := 0; i < rackspec.SlotsPerRow(); i++ {
			x := inset + float64(i)*rackspec.SlotPitch + rackspec.SlotPitch/2
			m.Append(Box(
				V3{x - 4, boardY0 + 6, bpZ},
				V3{x + 4, boardY0 + rackspec.PCBHeight3U - 6, bpZ + 10},
			))
		}

		// Grab handles on the ears, outboard of the 84 HP row — which is
		// what the leftover between the row and the 19 inches is FOR.
		if handles {
			m.Append(RackHandle(inset/2, y+bayH/2, bayH*0.72))
			m.Append(RackHandle(w-inset/2, y+bayH/2, bayH*0.72))
		}
	}
	return m
}

// PanelLayout is a module's real control layout, measured rather than
// invented: what is on the panel, where, and how big — in millimeters, with
// the origin at the panel's lower-left corner.
//
// It exists because a hand-placed "plausible" panel is not a model of
// anything. `uitool modules -layout` measures the running rack and writes
// these out, and ExactPanel turns one into the module it measured.
type PanelLayout struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	HP       int       `json:"hp"`
	Controls []Control `json:"controls"`
}

// ExactPanel builds the module a PanelLayout describes.
//
// Controls are clamped to the panel first. A measured layout can put one
// slightly over an edge — an element whose box overhangs its module's box by
// a millimeter, which in a browser is nothing and on a panel is a part
// hanging off the side. The counter module came out 129.8 mm tall that way,
// against a panel that is 128.5.
func ExactPanel(l PanelLayout, body bool, seg int) Mesh {
	w := float64(l.HP)*rackspec.HP - rackspec.Seam
	h := rackspec.PanelHeight3U
	ctl := make([]Control, 0, len(l.Controls))
	for _, c := range l.Controls {
		r := controlFootprint(c)
		if 2*r > w || 2*r > h {
			continue // too big for this panel to carry; not a real part
		}
		c.X = clamp(c.X, r, w-r)
		c.Y = clamp(c.Y, r, h-r)
		ctl = append(ctl, c)
	}
	return ModulePanel(PanelOptions{HP: l.HP, Controls: stackConcentric(ctl), Body: body, Seg: seg})
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// knobSkirt is how much wider than its nominal diameter a knob is drawn — the
// flange where it meets the panel.
const knobSkirt = 1.04

// controlFootprint is the largest radius a control actually DRAWS, which is
// what has to fit on the panel. It is not always the nominal diameter: a knob
// has a skirt a few percent wider, and clamping by the nominal figure left
// the skirt hanging over the edge by 0.38 mm.
func controlFootprint(c Control) float64 {
	d := c.Diam
	if d <= 0 {
		switch c.Kind {
		case PinControl:
			d = rackspec.PinHead
		case LEDControl:
			d = rackspec.LEDIndicator
		case ButtonControl:
			d = rackspec.TactSwitch
		case JackControl:
			d = rackspec.JackHole35 + 2
		default:
			d = rackspec.KnobLarge
		}
	}
	if c.Kind == KnobControl {
		return d / 2 * knobSkirt
	}
	return d / 2
}

// stackConcentric turns knobs that share a position into a concentric stack.
//
// Half this panel's knobs are concentric — a selector ring with a smaller
// knob in the middle of it — and the measured layout reports them as what
// they are: two controls at the same X and Y with different diameters. Built
// as-is that is two knobs occupying the same space, the smaller one buried
// inside the larger, which models neither part.
//
// Sorted big to small, each one starts where the one under it ended, so a
// 15 mm ring carries a 9 mm knob on its face the way the real ones do.
func stackConcentric(in []Control) []Control {
	const tol = 0.75 // mm; measured centers of a concentric pair are not identical

	used := make([]bool, len(in))
	out := make([]Control, 0, len(in))
	for i, c := range in {
		if used[i] {
			continue
		}
		group := []int{i}
		if c.Kind == KnobControl {
			for j := i + 1; j < len(in); j++ {
				if used[j] || in[j].Kind != KnobControl {
					continue
				}
				if math.Abs(in[j].X-c.X) <= tol && math.Abs(in[j].Y-c.Y) <= tol {
					group = append(group, j)
					used[j] = true
				}
			}
		}
		used[i] = true
		if len(group) == 1 {
			out = append(out, c)
			continue
		}
		// Widest first: it is the one sitting on the panel.
		sort.Slice(group, func(a, b int) bool {
			return controlFootprint(in[group[a]]) > controlFootprint(in[group[b]])
		})
		lift := 0.0
		for _, gi := range group {
			g := in[gi]
			// Share the position of the outermost, so a millimeter of
			// measurement noise does not leave the stack leaning.
			g.X, g.Y = in[group[0]].X, in[group[0]].Y
			g.Height = knobHeightFor(g.Diam)
			g.Lift = lift
			lift += g.Height
			out = append(out, g)
		}
	}
	return out
}

// knobHeightFor is how tall a knob of a given diameter stands. A small knob
// on top of a large one is not as tall as the one under it — a concentric
// stack that ignored this came out as a tower.
func knobHeightFor(d float64) float64 {
	if d <= 0 {
		d = rackspec.KnobLarge
	}
	h := d * 0.7
	if h < 4 {
		h = 4
	}
	if h > 14 {
		h = 14
	}
	return h
}

// Handle geometry: the U-shaped grab handles bolted to the ears of a rack.
const (
	HandleDepth = 34.0 // how far it stands off the front
	HandleWidth = 26.0 // across the ear
	HandleBar   = 7.0  // stock thickness
)

// RackHandle is one handle, standing off the front face at x, centered on y.
//
// A real one is a strap bent twice: a foot bolted flat to the ear, a leg out
// to the front, the grip across, and back again. Built as four boxes, which
// is what a bent strap is.
func RackHandle(x, y, height float64) Mesh {
	var m Mesh
	half := height / 2
	x0, x1 := x-HandleWidth/2, x+HandleWidth/2
	// The two feet, flat against the panel.
	m.Append(Box(V3{x0, y - half, 0}, V3{x1, y - half + HandleBar, HandleBar}))
	m.Append(Box(V3{x0, y + half - HandleBar, 0}, V3{x1, y + half, HandleBar}))
	// The legs standing out from them.
	m.Append(Box(V3{x0, y - half, HandleBar}, V3{x1, y - half + HandleBar, HandleDepth}))
	m.Append(Box(V3{x0, y + half - HandleBar, HandleBar}, V3{x1, y + half, HandleDepth}))
	// The grip across the front.
	m.Append(Box(
		V3{x0, y - half, HandleDepth - HandleBar},
		V3{x1, y + half, HandleDepth},
	))
	return m
}
