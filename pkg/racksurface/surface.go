package racksurface

// The surface: the rack drawn at a size that is the RACK's, not the viewer's.
//
// Everything here is in character cells, and none of it depends on how big
// the terminal is. A module two slots wide is always SlotCols*2 cells wide; a
// bay is always BayRows tall; bay three always begins at the same y. The
// viewer's size decides only how much of it can be seen at once, which is a
// separate question answered by a View.
//
// This is the inversion that makes the terminal and the page agree. Reflowing
// to the terminal meant the rack had one shape in the browser and another in
// a shell, and the second one had no bays in it — a module's neighbors were
// whatever the wrap happened to put beside it. A physical rack does not get
// wider when you step back from it.

// Metrics is the scale: how many cells a slot and a bay are worth.
//
// One place, because every one of these numbers is load-bearing twice. SlotCols
// sets both how wide a module is drawn and how wide the whole surface is, so a
// caller cannot change the panel size without changing the rack — which is
// true of a real rack too, and is the point.
type Metrics struct {
	// SlotCols is the width of one slot. The rack's own unit: a 2-slot module
	// is 2*SlotCols cells across.
	SlotCols int
	// PanelRows is the height of a module's panel, the drawable part of a bay.
	PanelRows int
	// HeadRows is the label strip above each bay, carrying the section names.
	HeadRows int
	// GutterRows is the blank between one bay and the next.
	GutterRows int
}

// DefaultMetrics is a scale at which a knob is still a knob.
//
// 16 cells per slot is set by the dial, not the other way round: a
// half-block knob reads as a knob from about 8 columns and is coarse until
// about 12, and a 1-slot module has to hold one inside its frame with a label
// over it. 12 + a border and a pad on each side is 16.
var DefaultMetrics = Metrics{SlotCols: 16, PanelRows: 22, HeadRows: 2, GutterRows: 1}

// Panel is where one module sits on the surface, in cells.
type Panel struct {
	Item int // index into the items Build was given
	Bay  int // index into Surface.Bays
	Slot int // the slot this module starts at within its bay
	X, Y int
	W, H int
}

// Bay is one row of the rack: a strip of the surface, with the sections that
// share it and where each of those sections starts and ends.
type Bay struct {
	Index int
	Y, H  int
	// Runs are the sections in this bay, with From/Count over the bay's OWN
	// module list. SpanX and SpanW give the same runs as cells, so a caller
	// can draw the label over exactly the modules it names.
	Runs   []Run
	SpanX  []int
	SpanW  []int
	Panels []int // indices into Surface.Panels, in slot order
	// Used and Capacity are slots, so a caller can show how much of the bay
	// is blank panel without re-deriving it.
	Used, Capacity int
}

// Surface is the whole rack, laid out once.
type Surface struct {
	Cols, Rows int
	Metrics    Metrics
	Bays       []Bay
	Panels     []Panel
	// PanelOf maps an item index to its panel, or -1 for an item that was
	// packed into nothing (zero slots — a module switched out).
	PanelOf []int
}

// Build packs the items and gives every one of them a rectangle.
//
// capacity is the rack's width in slots — the property of the frame, not of
// the terminal. monitor is as Pack takes it.
func Build(items []Item, capacity int, monitor map[string]int, m Metrics) Surface {
	if capacity < 1 {
		capacity = 1
	}
	if m.SlotCols < 1 {
		m.SlotCols = 1
	}
	if m.PanelRows < 1 {
		m.PanelRows = 1
	}
	s := Surface{
		Cols:    capacity * m.SlotCols,
		Metrics: m,
		PanelOf: make([]int, len(items)),
	}
	for i := range s.PanelOf {
		s.PanelOf[i] = -1
	}

	y := 0
	for bi, unit := range Pack(items, capacity, monitor) {
		// The bay is as tall as the tallest thing in it, which is what a
		// shelf of chassis of different heights does.
		panelRows := m.PanelRows
		for _, it := range unit {
			if r := items[it].Rows; r > panelRows {
				panelRows = r
			}
		}
		bay := Bay{
			Index:    bi,
			Y:        y,
			H:        m.HeadRows + panelRows,
			Runs:     Runs(items, unit),
			Capacity: capacity,
		}
		// A module's x is the sum of the slots before it, NOT its position in
		// the bay's list: the modules are different widths, and a bay that
		// opened with a monitor has slots charged to it that no module in the
		// list owns.
		slot := monitorSlots(monitor, SectionOf(items, unit), capacity)
		for _, it := range unit {
			w := max(items[it].Slots, 0)
			p := Panel{
				Item: it,
				Bay:  bi,
				Slot: slot,
				X:    slot * m.SlotCols,
				Y:    y + m.HeadRows,
				W:    w * m.SlotCols,
				H:    panelRows,
			}
			bay.Panels = append(bay.Panels, len(s.Panels))
			s.PanelOf[it] = len(s.Panels)
			s.Panels = append(s.Panels, p)
			slot += w
		}
		bay.Used = slot
		// The runs as cells, now that every module's slot is known.
		for _, r := range bay.Runs {
			x, w := 0, 0
			for n := r.From; n < r.From+r.Count && n < len(bay.Panels); n++ {
				p := s.Panels[bay.Panels[n]]
				if n == r.From {
					x = p.X
				}
				w += p.W
			}
			bay.SpanX = append(bay.SpanX, x)
			bay.SpanW = append(bay.SpanW, w)
		}
		s.Bays = append(s.Bays, bay)
		y += bay.H + m.GutterRows
	}
	if len(s.Bays) > 0 {
		y -= m.GutterRows // no gutter under the last bay
	}
	s.Rows = y
	return s
}

// monitorSlots is Pack's own clamp, which Build has to repeat because the
// monitor's slots are charged to the bay and owned by no module in it: the
// first panel starts after them, and only this knows how far after.
func monitorSlots(monitor map[string]int, section string, capacity int) int {
	w := monitor[section]
	if w < 0 {
		return 0
	}
	if w >= capacity {
		return capacity - 1
	}
	return w
}

// Blank is how many slots of the surface carry no module, and the share of
// the rack that is. A rack is judged by this: blank panel is wasted frame.
func (s Surface) Blank() (slots int, share float64) {
	total := 0
	for _, b := range s.Bays {
		total += b.Capacity
		slots += b.Capacity - b.Used
	}
	if total == 0 {
		return 0, 0
	}
	return slots, float64(slots) / float64(total)
}
