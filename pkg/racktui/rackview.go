package racktui

import (
	"sort"
	"strings"
)

// The rack, as panels rather than as a list.
//
// Controls come off the registry flat, in the order the rack wired them, with
// the module each is mounted in. Grouping them by that and drawing each group
// as a bordered panel is the whole of what makes this look like the
// instrument instead of like its settings: a knob in a panel, under the name
// silkscreened on it, beside the reading it produces.
//
// Panels are laid side by side until the row is full and then wrapped, which
// is what the rack's own packer does with slots — here the unit is a
// character cell rather than an HP, so the arithmetic is different but the
// rule is the same.

// panelWidth is how wide one module's panel is drawn. Wide enough for a
// five-cell dial, a label and a reading beside it, and narrow enough that
// three fit across an eighty-column terminal.
const panelWidth = 26

// modulePanel is one module's controls, in the order the rack wired them.
type modulePanel struct {
	Name string
	Ctls []Control
}

// groupByModule collects the surface into panels, keeping the registry's
// order both within a panel and between them — that order is the order the
// rack was built in, which is the order it reads.
// The second return says, for each control in ctls, which panel it landed in
// and where — so a cursor kept on the flat list can be drawn on the rack.
func groupByModule(ctls []Control) ([]modulePanel, [][2]int) {
	var out []modulePanel
	where := make([][2]int, len(ctls))
	at := map[string]int{}
	for n, c := range ctls {
		name := c.Module
		if name == "" {
			name = "console"
		}
		i, ok := at[name]
		if !ok {
			at[name] = len(out)
			out = append(out, modulePanel{Name: name})
			i = len(out) - 1
		}
		where[n] = [2]int{i, len(out[i].Ctls)}
		out[i].Ctls = append(out[i].Ctls, c)
	}
	return out, where
}

// drawPanel renders one module as lines of exactly panelWidth cells.
//
// The rows each control occupies are reported so the caller can highlight it —

// tcell draws the highlight, this decides the shape.
func drawPanel(p modulePanel) (lines []string, rowOf []int) {
	name := strings.ToUpper(p.Name)
	head := "┌─ " + clip(name, panelWidth-6) + " "
	lines = append(lines, head+strings.Repeat("─", panelWidth-len([]rune(head))-1)+"┐")
	rowOf = append(rowOf, -1)

	for i, c := range p.Ctls {
		for _, l := range drawControl(c) {
			lines = append(lines, "│"+padRunes(l, panelWidth-2)+"│")
			rowOf = append(rowOf, i)
		}
	}
	lines = append(lines, "└"+strings.Repeat("─", panelWidth-2)+"┘")
	rowOf = append(rowOf, -1)
	return lines, rowOf
}

// drawControl is one control's rows inside a panel.
//
// A switch with two states is a lamp, a switch with a handful is its detents,
// and anything with a range is a dial with its reading beside it. That is the
// same rule the rack's own panels follow, and for the same reason: a control
// should look like the thing it is.
func drawControl(c Control) []string {
	label := clip(c.Label, 10)
	if c.IsSelect && len(c.Options) == 2 {
		on := c.Value == c.Options[1]
		return []string{drawLamp(on, label)}
	}
	if c.IsSelect && len(c.Options) > 0 {
		return []string{
			label,
			"  " + drawDetents(c.Options, c.Value, panelWidth-5),
		}
	}
	k := drawKnob(fracOf(c))
	val := shortNum(c.Value, 7)
	return []string{
		k[0] + "  " + label,
		k[1] + "  " + ledText(val, 7),
		k[2],
	}
}

// layoutRack lays the panels across a terminal of the given width, wrapping
// when a row is full, and reports where every control landed.
//
// spot is a control's place on the screen: which panel, which index in it,
// and the rows it occupies — everything a cursor needs to be drawn and moved.
type spot struct {
	Panel int
	Index int
	Y0    int
	Y1    int
	X     int
}

func layoutRack(panels []modulePanel, width int) (lines []string, spots []spot) {
	perRow := width / (panelWidth + 1)
	if perRow < 1 {
		perRow = 1
	}
	for start := 0; start < len(panels); start += perRow {
		end := start + perRow
		if end > len(panels) {
			end = len(panels)
		}
		var blocks [][]string
		var rowOfs [][]int
		tall := 0
		for i := start; i < end; i++ {
			b, r := drawPanel(panels[i])
			blocks = append(blocks, b)
			rowOfs = append(rowOfs, r)
			if len(b) > tall {
				tall = len(b)
			}
		}
		base := len(lines)
		for y := 0; y < tall; y++ {
			var row strings.Builder
			for _, b := range blocks {
				if y < len(b) {
					row.WriteString(b[y])
				} else {
					row.WriteString(strings.Repeat(" ", panelWidth))
				}
				row.WriteString(" ")
			}
			lines = append(lines, row.String())
		}
		// Where each control ended up, for the cursor.
		for bi, ro := range rowOfs {
			seen := map[int]*spot{}
			for y, idx := range ro {
				if idx < 0 {
					continue
				}
				s, ok := seen[idx]
				if !ok {
					seen[idx] = &spot{Panel: start + bi, Index: idx, Y0: base + y, Y1: base + y, X: bi * (panelWidth + 1)}
					continue
				}
				s.Y1 = base + y
			}
			keys := make([]int, 0, len(seen))
			for k := range seen {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				spots = append(spots, *seen[k])
			}
		}
	}
	return lines, spots
}
