// Package rackspec is the physical dimension system the control surface is
// drawn to: real millimeters, from real standards, in one place.
//
// The panel was already drawn on a 4 px per millimeter grid — almost every
// size in panel.css is a whole number of millimeters at that scale (56px =
// 14mm, 96px = 24mm, 24px = 6mm, 8px = 2mm). It had simply never been written
// down, so nothing checked it and a few controls had drifted off any real
// part: the patch matrix's pins were on a 4.5 mm pitch, which is not a grid
// anybody makes, and the modules were 33 × 131 mm, which is not a panel
// anybody cuts.
//
// # The standards
//
// The frame is the 19-inch rack of EIA-310, and the card cage inside it is
// IEC 60297-3 (DIN 41494) — which is where Eurorack gets its mechanics, so
// the two agree:
//
//   - The panel/flange width is 19 in (482.6 mm). Between the rails the
//     opening is about 450 mm (17.7 in) — the "17 something" of a rack — and
//     of that, 84 HP = 426.72 mm is the usable panel width the standard
//     defines for plug-in units. The remainder goes to the card guides, the
//     side members and the mounting ears, which is why the row is 84 HP and
//     not simply as many HP as the bare opening would take.
//   - Horizontal pitch (HP) is 0.2 in = 5.08 mm. Module widths are whole
//     multiples of it.
//   - A rack unit (U) is 1.75 in = 44.45 mm, so a 3U opening is 133.35 mm; a
//     3U *panel* is cut to 128.5 mm, the rest being clearance and rails.
//
// # The parts
//
// Sizes here are the ones real components come in, so that a knob on screen
// is a knob you could buy. Where a control had drifted, it is moved to the
// nearest real part rather than to the nearest round number.
package rackspec

// Everything in this package is millimeters unless the name says otherwise.

// The rack.
const (
	// PanelWidth19 is the full width of a 19-inch panel, flange to flange.
	PanelWidth19 = 482.6

	// RailOpening is roughly the clear width between the rails of a 19-inch
	// frame. It is approximate by nature — it varies with the frame — and is
	// here to say why RowHP is what it is.
	RailOpening = 450.0

	// HP is the horizontal pitch: 0.2 in, the unit module widths are counted
	// in. Also, not coincidentally, a standard grid pitch for matrix pins.
	HP = 5.08

	// U is one rack unit, 1.75 in.
	U = 44.45

	// RowHP is the standard 3U row of a 19-inch frame: 84 × 5.08 = 426.72 mm
	// of plug-in unit, the rest of the 19 inches being frame.
	RowHP = 84

	// PanelHeight3U is the height a 3U panel is cut to. The 3U *opening* is
	// 3 × U = 133.35 mm; the panel is shorter, so it clears the rails.
	PanelHeight3U = 128.5
)

// The plug-in unit behind the panel.
//
// A 19-inch subrack is a card cage: what goes into it is a Eurocard riding a
// pair of card guides into a connector on a backplane at the rear. The front
// panel is the visible end of that card, not a box in its own right — which
// is why a module is DEEPER THAN IT IS TALL, and why the board sticks out
// past the back of any case to reach the connector.
const (
	// PCBHeight3U is a 3U Eurocard's board height. Note that it is not the
	// panel height: the panel is 128.5 mm and the board is 100 mm, the
	// difference being the card guides and the clearance around them.
	PCBHeight3U = 100.0

	// PCBDepth is the standard Eurocard depth, measured back from the panel.
	// 220, 280 and 340 mm are the other standard depths.
	PCBDepth = 160.0

	// PCBThickness is standard FR-4.
	PCBThickness = 1.6

	// CardEdge is how far the board protrudes past the back of the case, so
	// its connector can reach the backplane.
	CardEdge = 12.0

	// EdgeFingerPitch is the pitch of the contacts on that protruding edge.
	// 2.54 mm is 0.1 in — the same grid everything else here is built on, and
	// the pitch DIN 41612 (the connector this form factor actually uses)
	// spaces its rows of pins at.
	EdgeFingerPitch = 2.54

	// CaseWall is the thickness of the case wrapped around the board.
	CaseWall = 1.5
)

// CaseDepth is how deep the case itself runs: the board's depth less the part
// left sticking out at the back.
const CaseDepth = PCBDepth - CardEdge

// The modules.
const (
	// ModuleHP is how wide one slot of the rack is. Seven, because the
	// content column the panel is laid out on is 29 mm and the module's own
	// padding adds 2 mm: 31 mm fits inside 7 HP (35.56 mm) and does not fit
	// inside 6 HP (30.48 mm). The layout picked the width; this only names it.
	ModuleHP = 7

	// Seam is the visible line between two adjacent panels. Real panels are
	// milled narrower than their nominal width for exactly this reason —
	// Eurorack takes 0.2 mm off — so a module is SlotWidth wide and the rack
	// pitch is a whole ModuleHP, seam included.
	Seam = 0.5
)

// SlotWidth is the milled width of a one-slot panel: its nominal HP width
// less the seam, so that panel-plus-seam — the pitch — is a whole ModuleHP.
//
// An N-slot module is therefore N×SlotPitch − Seam wide: it spans the N-1
// seams it covers, but not the one after it, which belongs to its neighbor.
const SlotWidth = ModuleHP*HP - Seam

// SlotPitch is center-to-center spacing of adjacent slots — a whole number of
// HP, which is the property that makes module edges line up down a rack.
const SlotPitch = ModuleHP * HP

// The components. These are catalog sizes: a 19 mm knob, a 6 mm tactile
// switch, a 0.2 in seven-segment digit are all things that exist.
const (
	KnobLarge  = 19.0 // the big parameter knobs
	KnobMedium = 15.0 // concentric outer rings
	KnobRing   = 14.0 // selector rings
	KnobSmall  = 9.0  // concentric inner knobs
	KnobTiny   = 8.0  // fine-trim discs

	// LEDIndicator and LEDIndicatorLarge are the two standard through-hole
	// LED diameters.
	LEDIndicator      = 3.0
	LEDIndicatorLarge = 5.0

	// DigitHeight is a seven-segment display's digit height. 0.2 in and
	// 0.3 in are the two common small sizes; the readouts use the smaller.
	DigitHeight = 5.08

	// TactSwitch is the ubiquitous 6 × 6 mm tactile switch, which is what the
	// program-bank buttons are.
	TactSwitch = 6.0

	// ToggleBushing is a sub-miniature toggle switch's panel bushing: 1/4 in.
	ToggleBushing = 6.35

	// PinHead and PinPitch are the patch matrix. The pitch is 0.2 in — the
	// same grid as HP, and a standard pitch for a pin matrix — and the head
	// is sized to leave a finger-visible gap between neighbors.
	//
	// This is a PIN MATRIX, in the EMS Synthi sense, and not a jackfield: the
	// holes are not sockets and nothing is supposed to take a cable. That is
	// worth saying because the pins had been drawn at 3.25 mm on a 4.5 mm
	// pitch, which reads as an undersized jack rather than as a pin.
	PinHead  = 3.5
	PinPitch = HP

	// JackHole35 is what a 3.5 mm mono jack needs in a panel, for the day the
	// matrix becomes a jackfield. The visible nut is a little wider again,
	// and Eurorack spaces them no closer than about 12.5 mm center to center
	// — roughly two and a half times the pin pitch, which is why swapping one
	// for the other is a layout change and not a size change.
	JackHole35          = 6.0
	JackPitch35         = 12.5
	JackHoleBanana      = 8.0
	JackHoleQuarterInch = 9.5
)

// PxPerMM is the screen scale at interface scale 1: one millimeter is four
// CSS pixels, so one pixel is a quarter of a millimeter.
const PxPerMM = 4.0

// Px converts millimeters to CSS pixels at interface scale 1.
func Px(mm float64) float64 { return mm * PxPerMM }

// MM converts CSS pixels at interface scale 1 back to millimeters.
func MM(px float64) float64 { return px / PxPerMM }

// RowWidth is the usable panel width of a standard row.
func RowWidth() float64 { return RowHP * HP }

// SlotsPerRow is how many whole modules fit across a standard 84 HP row.
func SlotsPerRow() int { return RowHP / ModuleHP }
