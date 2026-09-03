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

	"github.com/0magnet/chaosrack/internal/cdp"
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
    modules: [...panel.querySelectorAll('.modules > .sect')].filter(vis)
      .map(s => ({name: named(s, '.sect-hdr'), ...R(s)})),
    swsecs: [...panel.querySelectorAll('.swsec')].filter(vis)
      .map(s => ({name: named(s, '.swsec-hdr'), ...R(s),
                  switches: s.querySelectorAll('input.sw').length})),
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
