package racktui

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Tying the controls to the modules they are mounted in.
//
// The registry hands the surface over flat, with the module each control
// belongs to written on it. The rack hands over the modules and their widths.
// Neither knows about the other, and this is the join: for every module on the
// surface, the controls that sit on it, in the order the rack wired them.
//
// A control whose module is not on the surface is not dropped. It goes in a
// spare panel at the end, because a control the rack reported and the layout
// cannot place is a thing to SEE — silently discarding it would hide exactly
// the disagreement this join exists to expose.

// Module is one module as the rack describes it: enough to lay out, and no
// more. A Source produces these; the surface is built from them.
type Module struct {
	Key   string
	Cat   string // a model card's category, for the bay it belongs to
	Slots int
}

// moduleCtls is a module with its controls attached.
type moduleCtls struct {
	Name string
	Key  string
	Ctls []Control
}

// ctlAt names one control by where it is: which module on the surface, and
// which control within it. The cursor is one of these.
type ctlAt struct {
	Module int
	Index  int
}

// attach groups the controls by module and puts them in the order the surface
// items are in, so an item index is a moduleCtls index.
//
// Matching is by key, case-folded, because the registry records the module
// name as the panel spells it and the frame records the key as the layout
// spells it, and those differ by capitalization more often than not.
func attach(items []racksurface.Item, ctls []Control) []moduleCtls {
	out := make([]moduleCtls, len(items))
	at := make(map[string]int, len(items))
	for i, it := range items {
		name := it.Title
		if name == "" {
			name = it.Key
		}
		out[i] = moduleCtls{Name: name, Key: it.Key}
		at[fold(it.Key)] = i
	}
	var orphans []Control
	for _, c := range ctls {
		if i, ok := at[fold(c.Module)]; ok {
			out[i].Ctls = append(out[i].Ctls, c)
			continue
		}
		orphans = append(orphans, c)
	}
	if len(orphans) > 0 {
		out = append(out, moduleCtls{Name: "unplaced", Key: "", Ctls: orphans})
	}
	return out
}

func fold(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// cursorOf finds a control on the surface by its id, so the cursor survives a
// reload that renumbers things.
func cursorOf(mods []moduleCtls, id string) (ctlAt, bool) {
	for mi, m := range mods {
		for ci, c := range m.Ctls {
			if c.ID == id {
				return ctlAt{Module: mi, Index: ci}, true
			}
		}
	}
	return ctlAt{}, false
}

// step moves the cursor n controls along, running off the end of one module
// into the next — the modules are in rack order, so this walks the rack the
// way a hand would.
func (a ctlAt) step(mods []moduleCtls, n int) ctlAt {
	flat := flatten(mods)
	if len(flat) == 0 {
		return a
	}
	i := 0
	for j, c := range flat {
		if c == a {
			i = j
			break
		}
	}
	i += n
	if i < 0 {
		i = 0
	}
	if i >= len(flat) {
		i = len(flat) - 1
	}
	return flat[i]
}

// flatten is every control on the surface in rack order.
func flatten(mods []moduleCtls) []ctlAt {
	var out []ctlAt
	for mi, m := range mods {
		for ci := range m.Ctls {
			out = append(out, ctlAt{Module: mi, Index: ci})
		}
	}
	return out
}

// detentsOf is how many positions a control has, for the ticks round its dial.
// A continuous control has none.
func detentsOf(c Control) int {
	if c.IsSelect {
		return len(c.Options)
	}
	return 0
}

// shortLabel is a control's name, trimmed to fit over its dial.
func shortLabel(c Control) string {
	s := c.Label
	if s == "" {
		s = c.ID
	}
	return strings.ToUpper(s)
}

// readingOf is what the LED under a control says.
func readingOf(c Control) string {
	if c.IsSelect {
		return c.Value
	}
	return shortNum(c.Value, knobCols)
}
