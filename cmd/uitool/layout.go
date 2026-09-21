// Subcommand layout: the control panel's own geometry, which nothing else here
// looks at.
//
// The monkey fuzzes and catches "the app broke". The golden oracle drives known
// states and checks the render is right — but it HIDES THE PANEL before every
// screenshot, deliberately, so the golden is the model and not the controls. So
// between them they can watch every pixel of the model and never notice that
// the controls have fallen apart.
//
// Which is exactly what happened: a generated list of switches went into a
// wrapper span, a span is inline, and twelve switches laid themselves out side
// by side instead of stacking. That took its switch column from 115 pixels wide
// to 958, and dragged the three sibling sections sharing the column with it, so
// the Console module went from 404 pixels to 1220 and shoved the rest of the
// rack off the panel. Every test passed.
//
// The invariants below are the ones that breakage violated, written so they
// hold at any interface scale rather than at the size it happened to be:
//
//   - the panel does not overflow itself horizontally
//
//   - no module is wider than the panel that holds it
//
//   - the switch columns are COLUMNS: none is wildly wider than its siblings,
//     which is what "something in here is laying out inline" looks like from
//     the outside
//
//     uitool layout             # measure the panel and check it
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// layoutProbe is evaluated in the page: it measures the panel, its modules and
// its switch columns, and hands back the numbers to judge.
const layoutProbe = `JSON.stringify((() => {
  const panel = document.getElementById('controls-panel');
  if (!panel) return {error: 'no #controls-panel'};
  const R = (el) => { const b = el.getBoundingClientRect();
    return {w: Math.round(b.width), h: Math.round(b.height)}; };
  const named = (el, sel) => { const h = el.querySelector(sel);
    return h ? h.textContent.trim() : '?'; };
  const vis = (el) => getComputedStyle(el).display !== 'none';
  return {
    panel: {...R(panel), scrollW: panel.scrollWidth, clientW: panel.clientWidth},
    // A DESCENDANT, not a child: modules live inside a unit's opening
    // now (.modules > .runit > .runit-open > .sect), and the child
    // selector this used to be quietly matched nothing at all — so the
    // "no module is wider than the panel" check had been passing on an
    // empty list since the rack grew its unit level.
    modules: [...panel.querySelectorAll('.modules .sect')].filter(vis)
      .map(s => ({name: named(s, '.sect-hdr'), ...R(s)})),
    swsecs: [...panel.querySelectorAll('.swsec')].filter(vis)
      .map(s => ({name: named(s, '.swsec-hdr'), ...R(s),
                  switches: s.querySelectorAll('input.sw').length})),
    cells: (() => {
      // The frame is drawn through a CSS transform when the window is
      // narrower than nineteen inches, so every rect comes back scaled.
      // Divide it out, or one footprint reads as two at 0.96.
      const f = document.querySelector('.rack-frame');
      let k = 1;
      if (f) { const m = new DOMMatrix(getComputedStyle(f).transform); if (m.a > 0) k = m.a; }
      const off = (el, r) => { const b = el.getBoundingClientRect();
        return [Math.round((b.left - r.left) / k), Math.round((b.top - r.top) / k)]; };
      return [...panel.querySelectorAll('.modules .pcell, .modules .punit')]
        .filter(e => e.offsetParent !== null)
        .map(e => {
          const r = e.getBoundingClientRect();
          const knob = e.querySelector('.knob, .knob-ring');
          const led  = e.querySelector('.numin');
          const o = {w: e.offsetWidth, h: e.offsetHeight,
                     cls: [...e.classList].filter(c => c !== 'pcell').sort().join('.') || '(bare)'};
          if (knob) { const b = knob.getBoundingClientRect();
            o.kx = Math.round((b.left + b.width / 2 - r.left) / k);
            o.ky = Math.round((b.top + b.height / 2 - r.top) / k); }
          if (led) { const p = off(led, r); o.lx = p[0]; o.ly = p[1]; }
          return o;
        });
    })(),
    // A module's knob grid: how many controls are on it, and how many
    // columns they stand in. Cells fill a column downwards, so the second
    // number follows from the first — see check 5.
    grids: [...panel.querySelectorAll('.modules .sect')].filter(vis).map(s => {
      const row = s.querySelector(':scope > .row.vmrow:not(.meterrow)');
      if (!row) return null;
      const cs = [...row.children].filter(e =>
        e.classList.contains('pcell') && e.offsetWidth > 0 && e.offsetHeight > 0);
      if (!cs.length) return null;
      return {name: named(s, '.sect-hdr'), cells: cs.length, w: s.offsetWidth,
              cols: new Set(cs.map(e => Math.round(e.getBoundingClientRect().left))).size};
    }).filter(Boolean),
  };
})())`

type layoutBox struct {
	Name     string
	W, H     int
	Switches int
}

func runLayout() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "layout: no tab —", err)
		os.Exit(2)
	}
	m := c.EvalJSON(layoutProbe)
	if m == nil {
		fmt.Fprintln(os.Stderr, "layout: the probe returned nothing (is the app loaded?)")
		os.Exit(2)
	}
	if e, bad := m["error"]; bad {
		fmt.Fprintln(os.Stderr, "layout:", e)
		os.Exit(2)
	}

	panel := numMap(m["panel"])
	modules := boxes(m["modules"])
	swsecs := boxes(m["swsecs"])
	cells := cellsOf(m["cells"])
	var fails []string

	// 1. The panel must not overflow itself. A slack of a few pixels absorbs
	//    sub-pixel rounding and the scrollbar gutter.
	if over := panel["scrollW"] - panel["clientW"]; over > 8 {
		fails = append(fails, fmt.Sprintf(
			"the panel overflows horizontally by %dpx (scrollWidth %d > clientWidth %d)",
			over, panel["scrollW"], panel["clientW"]))
	}

	// 2. Nothing in the rack may be wider than the rack.
	for _, mod := range modules {
		if mod.W > panel["w"]+8 {
			fails = append(fails, fmt.Sprintf(
				"module %q is %dpx wide, wider than the %dpx panel", mod.Name, mod.W, panel["w"]))
		}
	}

	// 3. The switch sections are columns of switches, so they are all about as
	//    wide as each other. One much wider means something inside is laying out
	//    along the wrong axis.
	//
	//    Measured against the NARROWEST, not the median or the mean: the failure
	//    this is here to catch took four of the seven columns with it, which
	//    moved the median to the broken width and made it agree that everything
	//    was fine. A statistic the fault can corrupt cannot be the thing that
	//    detects the fault. The narrowest column is whichever one is still
	//    behaving, and it takes only one.
	if len(swsecs) >= 3 {
		lo, loName := swsecs[0].W, swsecs[0].Name
		for _, s := range swsecs {
			if s.W < lo {
				lo, loName = s.W, s.Name
			}
		}
		for _, s := range swsecs {
			if lo > 0 && s.W > lo*3 {
				fails = append(fails, fmt.Sprintf(
					"switch column %q is %dpx wide against %q at %dpx — its %d switches are not stacking",
					s.Name, s.W, loName, lo, s.Switches))
			}
		}
	}

	// 4. THE CONTROL CELL. Every control on the panel occupies one
	//    footprint, and its knob and readout sit at one place inside it.
	//
	//    This is the rule Woodson & Conover make binding rather than tidy
	//    (Human Engineering Guide for Equipment Designers, 2nd ed. §2-132):
	//    having given rules for orienting a single group, they say that
	//    "for several groups on the same panel, use a consistent pointer
	//    position REGARDLESS of the above recommendations". Sameness across
	//    groups outranks what is locally best, because the panel is meant
	//    to be operated by position — you find a control by where it is,
	//    not by reading it.
	//
	//    It FAILS today, deliberately. The cell is declared in two places
	//    with different values (.vmcell and .axcol.axrot take --krow with
	//    !important; the bare cells take their own 116x160) and unifying
	//    them is a pass over the whole cell cascade. The counts below are
	//    the measurement that pass is against: they must fall to one and
	//    never rise.
	cellFoot := tally(cells, func(c layoutCell) string {
		return fmt.Sprintf("%dx%d", c.W, c.H)
	})
	if len(cellFoot) > 1 {
		fails = append(fails, fmt.Sprintf(
			"%d control-cell footprints, want 1: %s", len(cellFoot), rank(cellFoot)))
	}
	cellKnob := tally(cells, func(c layoutCell) string {
		if c.KX == 0 && c.KY == 0 {
			return "" // no knob in this cell
		}
		return fmt.Sprintf("%d,%d", c.KX, c.KY)
	})
	if len(cellKnob) > 1 {
		fails = append(fails, fmt.Sprintf(
			"%d knob positions within the cell, want 1: %s", len(cellKnob), rank(cellKnob)))
	}

	// 5. A MODULE IS AS NARROW AS ITS CONTROLS ALLOW. Cells fill a column
	//    downwards and start a new one when the column is full, so a
	//    module with N controls on it stands them in ceil(N/rows) columns
	//    and is milled that many slots wide. More columns than that is a
	//    panel carrying blank space it was charged a slot for.
	//
	//    This caught a module height two pixels short of three rows. The
	//    content row reserved the panel less a flat header and then spent
	//    the remainder on a bottom margin and a row gap, so every module
	//    with three controls on it — Style, the three generators, Test,
	//    Counter, Envelope, Layers, Position, Model Out — stood two and
	//    one, was milled two slots wide, and left the bottom third of both
	//    columns blank. Fifteen slots of the rack, and a whole 84 HP row.
	rows := rackspec.RowsPerPanel()
	for _, g := range gridsOf(m["grids"]) {
		want := (g.Cells + rows - 1) / rows
		if g.Cols > want {
			fails = append(fails, fmt.Sprintf(
				"module %q stands its %d controls in %d columns, want %d — %dpx of panel for %d blank positions",
				g.Name, g.Cells, g.Cols, want, g.W, g.Cols*rows-g.Cells))
		}
	}

	{
		fmt.Printf("panel %dx%d (scrollW %d, clientW %d)\n",
			panel["w"], panel["h"], panel["scrollW"], panel["clientW"])
		fmt.Println("modules:")
		for _, b := range modules {
			fmt.Printf("  %-14s %4dx%-4d\n", b.Name, b.W, b.H)
		}
		fmt.Printf("switch columns (narrowest %dpx):\n", minW(swsecs))
		for _, b := range swsecs {
			fmt.Printf("  %-14s %4dx%-4d  %2d switches\n", b.Name, b.W, b.H, b.Switches)
		}
	}
	if len(fails) > 0 {
		fmt.Fprintln(os.Stderr, "\nlayout: FAILED")
		for _, f := range fails {
			fmt.Fprintln(os.Stderr, "  -", f)
		}
		os.Exit(1)
	}
	fmt.Printf("layout: ok — %d modules, %d switch columns, no overflow\n", len(modules), len(swsecs))
}

func numMap(v any) map[string]int {
	out := map[string]int{}
	if m, ok := v.(map[string]any); ok {
		for k, n := range m {
			if f, ok := n.(float64); ok {
				out[k] = int(f)
			}
		}
	}
	return out
}

func boxes(v any) []layoutBox {
	arr, _ := v.([]any)
	out := make([]layoutBox, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		b := layoutBox{}
		if s, ok := m["name"].(string); ok {
			b.Name = s
		}
		if f, ok := m["w"].(float64); ok {
			b.W = int(f)
		}
		if f, ok := m["h"].(float64); ok {
			b.H = int(f)
		}
		if f, ok := m["switches"].(float64); ok {
			b.Switches = int(f)
		}
		out = append(out, b)
	}
	return out
}

func minW(bs []layoutBox) int {
	if len(bs) == 0 {
		return 0
	}
	lo := bs[0].W
	for _, b := range bs {
		if b.W < lo {
			lo = b.W
		}
	}
	return lo
}

// layoutCell is one control's footprint, and where its knob and readout sit
// inside it.
//
// Woodson & Conover §2-131 define the footprint a panel is laid out on: not
// the knob, but "the exterior dimension of such items as dial face or
// control knob, AND the interior dimension of the physical structure of the
// control mechanism that will limit the proximity of adjacent items." A
// control's box is the knob plus whatever stops the next one coming closer,
// which is what these numbers measure.
type layoutCell struct {
	W, H   int
	KX, KY int
	LX, LY int
	Cls    string
}

// cellsOf reads the cells out of the probe's JSON.
func cellsOf(v any) []layoutCell {
	arr, _ := v.([]any)
	out := make([]layoutCell, 0, len(arr))
	for _, e := range arr {
		m, _ := e.(map[string]any)
		if m == nil {
			continue
		}
		num := func(k string) int {
			f, _ := m[k].(float64)
			return int(f)
		}
		s, _ := m["cls"].(string)
		out = append(out, layoutCell{
			W: num("w"), H: num("h"),
			KX: num("kx"), KY: num("ky"),
			LX: num("lx"), LY: num("ly"),
			Cls: s,
		})
	}
	return out
}

// tally counts the cells by whatever key is asked for, skipping the ones the
// key does not apply to (a cell with no knob has no knob position).
func tally(cells []layoutCell, key func(layoutCell) string) map[string]int {
	out := map[string]int{}
	for _, c := range cells {
		if k := key(c); k != "" {
			out[k]++
		}
	}
	return out
}

// rank renders a tally commonest-first, which is the order that says which
// value is the majority and which are the strays to be brought into line.
func rank(m map[string]int) string {
	type kv struct {
		k string
		n int
	}
	list := make([]kv, 0, len(m))
	for k, n := range m {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].k < list[j].k
	})
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s x%d", e.k, e.n))
	}
	return strings.Join(parts, ", ")
}

// layoutGrid is one module's knob grid: the controls on it, and the columns
// they stand in.
type layoutGrid struct {
	Name  string
	Cells int
	Cols  int
	W     int
}

func gridsOf(v any) []layoutGrid {
	arr, _ := v.([]any)
	out := make([]layoutGrid, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		g := layoutGrid{}
		if s, ok := m["name"].(string); ok {
			g.Name = s
		}
		for _, f := range []struct {
			k string
			p *int
		}{{"cells", &g.Cells}, {"cols", &g.Cols}, {"w", &g.W}} {
			if n, ok := m[f.k].(float64); ok {
				*f.p = int(n)
			}
		}
		out = append(out, g)
	}
	return out
}
