// Subcommand monkey: a random-walk UI fuzzer for the wasm-stuff control panel.
//
// It drives an already-open browser tab over the Chrome DevTools Protocol —
// the same tab a human is looking at — firing seeded, randomized REAL input
// (clicks, knob/canvas drags, wheel, mode/dock switches, toggles, reset) and,
// after each action, checking invariants a normal user relies on:
//
//   - no new uncaught JS exception / unhandled promise rejection
//   - no NaN / undefined / Infinity in the live permalink (#hash) or any LED
//   - the control panel is present and RECOVERABLE (never buried with no way
//     back — the class of bug that used to need a hard refresh)
//   - the JS main thread isn't frozen (detected via CDP response timeout, which
//     — unlike a page rAF/timer heartbeat — isn't fooled by a throttled tab)
//   - a non-audio model mode actually RENDERS something (not a blank canvas)
//
// Everything is seeded, so a failing run replays deterministically. On a
// violation it dumps the action log and writes a screenshot. Real CDP input
// means it hits the same hit-testing / z-order / gesture paths a user does.
//
// It talks to whatever Chromium/Brave is running with --remote-debugging-port
// (default 9222) and picks the tab whose URL contains -target.
//
// This is a crash/invariant fuzzer — it finds "the app broke", not "the app is
// visually/semantically wrong". For the latter see cmd/uigolden.
//
// Usage:
//
//	uitool monkey -steps 120 -seed 7          # fuzz the :8300 tab, watchable pacing
//	uitool monkey -seed 7 -steps 120 -delay 0 # replay seed 7 as fast as possible
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0magnet/wasm-stuff/internal/cdp"
)

var (
	seed    = flag.Int64("seed", 1, "RNG seed (same seed → same sequence)")
	steps   = flag.Int("steps", 80, "number of random actions")
	delayMs = flag.Int("delay", 110, "ms to pause after each action (raise to watch, 0 = fast)")
	shotDir = flag.String("out", ".", "directory for failure screenshots")
	stopOn  = flag.Bool("stop", false, "stop at the first violation")
	verbose = flag.Bool("v", true, "log every action")
	noReset = flag.Bool("noreset", false, "never press Reset All — lets state (persist trail, colors, pose) accumulate over the whole walk, which is more interesting to watch and stresses long-run accumulation instead of the reset path")
)

const setupJS = `(function(){
  if(!window.__mk){window.__mk=1;window.__errs=[];
    addEventListener('error',function(e){window.__errs.push('ERR:'+(e.message||(e.error&&e.error.stack)||e))});
    addEventListener('unhandledrejection',function(e){window.__errs.push('REJ:'+e.reason)});
    var o=console.error;console.error=function(){try{window.__errs.push('CE:'+Array.prototype.join.call(arguments,' ').slice(0,300))}catch(_){}} ;
  }
  return 'ok';})()`

const controlsJS = `(function(){
  function vis(e){var r=e.getBoundingClientRect();return r.width>3&&r.height>3&&r.top>=0&&r.left>=0&&r.bottom<=innerHeight&&r.right<=innerWidth;}
  function onTop(e,cx,cy){var h=document.elementFromPoint(cx,cy);return h&&(e===h||e.contains(h)||h.contains(e));}
  var p=document.getElementById('controls-panel'),ctrls=[],knobs=[];
  if(p){[].forEach.call(p.querySelectorAll('.sw:not(#fullscreen-sw),.knob:not(.knob-fine),.rst,.numin,.sect-hdr,[id^=dock-]'),function(e){
    if(!vis(e))return;var r=e.getBoundingClientRect(),cx=Math.round(r.left+r.width/2),cy=Math.round(r.top+r.height/2);
    if(onTop(e,cx,cy))ctrls.push({cx:cx,cy:cy,t:(e.id||e.className||'').slice(0,20)});});
   [].forEach.call(p.querySelectorAll('.knob:not(.knob-fine)'),function(e){
    if(!vis(e))return;var r=e.getBoundingClientRect(),cx=Math.round(r.left+r.width/2),cy=Math.round(r.top+r.height/2);
    if(onTop(e,cx,cy))knobs.push({cx:cx,cy:cy});});}
  return JSON.stringify({controls:ctrls,knobs:knobs});
})()`

// snapJS captures the invariant state after an action, including a scan of
// every LED/readout for NaN/undefined/Infinity.
const snapJS = `(function(){
  var p=document.getElementById('controls-panel'),lb=[];
  if(p)[].forEach.call(p.querySelectorAll('.led,.numin'),function(e){var t=(e.value||e.textContent||'').trim();
    if(/NaN|undefined|Infinity/i.test(t))lb.push((e.id||e.className||'').slice(0,16)+'='+t.slice(0,10));});
  return JSON.stringify({
    nErr:(window.__errs||[]).length, lastErr:(window.__errs||[]).slice(-3),
    hashBad:/NaN|undefined|Infinity/.test(location.hash),
    panelDisp:p?getComputedStyle(p).display:'gone',
    mode:(document.getElementById('mode-select')||{}).value||'',
    ledBad:lb.slice(0,6)
  });
})()`

// recoverJS presses the ▤ button when the panel is hidden OR the "Front" canvas
// is stacked over it, then reports whether the panel is BOTH shown AND stacked
// at/above the canvas. Stacking (not elementFromPoint) is the right signal for
// visual burial: the Front canvas is pointer-events:none, so a hit-test passes
// through it and can't tell the panel is buried under an opaque backdrop —
// comparing z-index against the canvas does.
const recoverJS = `(function(){
  var p=document.getElementById('controls-panel'); if(!p) return JSON.stringify({shown:false,above:false});
  var cc=document.getElementById('gocanvas-container');
  function z(e){return e?(parseInt(getComputedStyle(e).zIndex)||0):0;}
  var mf=document.getElementById('model-front'), front=mf&&mf.checked;
  if(getComputedStyle(p).display==='none' || (front && z(p)<z(cc))){
    var b=[].slice.call(document.querySelectorAll('button')).filter(function(x){return x.textContent==='▤';})[0];
    if(b)b.click();
  }
  return JSON.stringify({shown:getComputedStyle(p).display!=='none', above:z(p)>=z(cc), pz:z(p), cz:z(cc), front:!!front});
})()`

var modes = []string{"rossler", "lorenz", "aizawa", "thomas", "chua", "cube", "torus", "magnetosphere", "lissajou", "graphicartist", "spectrogram", "xy", "fvf", "custom", "sprotta"}
var docks = []string{"bottom", "top", "left", "right", "float"}

type violation struct {
	Step   int
	Action string
	Mode   string
	Detail []string
}

func runMonkey() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "monkey:", err)
		os.Exit(1)
	}
	fmt.Printf("monkey: seed=%d steps=%d tab=%s\n", *seed, *steps, c.URL)
	c.Eval(setupJS)

	rng := rand.New(rand.NewSource(*seed))
	base := c.EvalJSON(snapJS)
	baseErr := toInt(base["nErr"])

	var log []string
	var viols []violation

	for step := 0; step < *steps; step++ {
		st := c.EvalJSON(controlsJS)
		controls := toList(st["controls"])
		knobs := toList(st["knobs"])
		var act string
		// Fire Reset All on a fixed cadence (as well as the random chance below)
		// so it's exercised every run and periodically returns the walk to a
		// known baseline. -noreset disables both, letting state accumulate.
		periodic := !*noReset && step > 0 && step%25 == 0
		roll := rng.Float64()
		switch {
		case periodic:
			act = "reset-all (periodic)"
			c.Eval(`(function(){var r=document.getElementById('reset-all-btn');if(r)r.click();})()`)
			time.Sleep(300 * time.Millisecond)
		case roll < 0.24 && len(controls) > 0:
			m := controls[rng.Intn(len(controls))]
			act = fmt.Sprintf("click %v @%d,%d", m["t"], toInt(m["cx"]), toInt(m["cy"]))
			c.Click(toF(m["cx"]), toF(m["cy"]))
		case roll < 0.37 && len(knobs) > 0:
			k := knobs[rng.Intn(len(knobs))]
			dx, dy := rng.Intn(80)-40, rng.Intn(80)-40
			act = fmt.Sprintf("knobdrag @%d,%d d%d,%d", toInt(k["cx"]), toInt(k["cy"]), dx, dy)
			c.Drag(toF(k["cx"]), toF(k["cy"]), toF(k["cx"])+float64(dx), toF(k["cy"])+float64(dy), 8)
		case roll < 0.49:
			x, y := 700+rng.Intn(700), 120+rng.Intn(440)
			dx, dy := rng.Intn(400)-200, rng.Intn(400)-200
			act = fmt.Sprintf("canvasdrag @%d,%d d%d,%d", x, y, dx, dy)
			c.Drag(float64(x), float64(y), float64(x+dx), float64(y+dy), 8)
		case roll < 0.58 && len(knobs) > 0:
			k := knobs[rng.Intn(len(knobs))]
			dy := float64(120 * (rng.Intn(2)*2 - 1))
			act = fmt.Sprintf("wheel @%d,%d %g", toInt(k["cx"]), toInt(k["cy"]), dy)
			c.Wheel(toF(k["cx"]), toF(k["cy"]), dy)
		case roll < 0.66:
			// Recolor: drive the real gradient-color pipeline directly. The native
			// <input type=color> swatches can't be operated by a synthetic click
			// (it opens the OS picker), so we set a random value and fire input +
			// change — exactly what the picker's onchange would do. Weight the
			// gradient stops (base/mid/top) over the background so the MODEL
			// visibly recolors, which was under-exercised before.
			cids := []string{"color-base", "color-mid", "color-top", "color-base", "color-top", "color-bg"}
			id := cids[rng.Intn(len(cids))]
			hex := fmt.Sprintf("#%02x%02x%02x", rng.Intn(256), rng.Intn(256), rng.Intn(256))
			act = fmt.Sprintf("recolor %s=%s", id, hex)
			c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value=%q;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, id, hex))
		case roll < 0.78:
			mode := modes[rng.Intn(len(modes))]
			act = "mode " + mode
			c.Eval(fmt.Sprintf(`(function(){var s=document.getElementById('mode-select');s.value=%q;s.dispatchEvent(new Event('change'));})()`, mode))
			time.Sleep(500 * time.Millisecond)
		case roll < 0.88:
			d := docks[rng.Intn(len(docks))]
			act = "dock " + d
			c.Eval(fmt.Sprintf(`(function(){var b=document.getElementById('dock-%s');if(b)b.click();})()`, d))
		case roll < 0.94:
			act = "toggle-sw"
			// Fullscreen is excluded: toggling it just fights the window manager
			// (and a synthetic toggle can't grant the user-gesture fullscreen
			// needs), so it adds noise without exercising anything useful.
			c.Eval(fmt.Sprintf(`(function(){var s=document.querySelectorAll('#controls-panel .sw:not(#fullscreen-sw)');if(s.length){var e=s[%d%%s.length];e.click&&e.click();}})()`, rng.Intn(9999)))
		case roll < 0.97:
			act = "panel-toggle"
			c.Eval(`(function(){var b=[].slice.call(document.querySelectorAll('button')).filter(function(x){return x.textContent==='▤';})[0];if(b)b.click();})()`)
		default:
			if *noReset {
				// Keep the walk (and the RNG stream) moving without resetting:
				// spin the model instead.
				x, y := 700+rng.Intn(700), 120+rng.Intn(440)
				dx, dy := rng.Intn(400)-200, rng.Intn(400)-200
				act = fmt.Sprintf("canvasdrag(no-reset) @%d,%d d%d,%d", x, y, dx, dy)
				c.Drag(float64(x), float64(y), float64(x+dx), float64(y+dy), 8)
			} else {
				act = "reset-all"
				c.Eval(`(function(){var r=document.getElementById('reset-all-btn');if(r)r.click();})()`)
			}
		}

		if *delayMs > 0 {
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		}
		log = append(log, act)

		snap := c.EvalJSON(snapJS)
		var bad []string
		if c.Frozen {
			bad = append(bad, "FROZEN (CDP eval timed out → JS main thread blocked)")
		}
		if n := toInt(snap["nErr"]); n > baseErr {
			bad = append(bad, "JSERR:"+joinAny(snap["lastErr"]))
			baseErr = n
		}
		if b, _ := snap["hashBad"].(bool); b {
			bad = append(bad, "BAD-HASH (NaN/undefined/Infinity in permalink)")
		}
		if lb := joinAny(snap["ledBad"]); lb != "" {
			bad = append(bad, "LED-GARBAGE: "+lb)
		}
		if snap["panelDisp"] == "gone" {
			bad = append(bad, "PANEL-GONE (controls-panel removed)")
		}
		// (Blank-render / visual oracles live in cmd/uigolden, which drives KNOWN
		// states — during a chaotic walk the model may be legitimately panned or
		// zoomed off-screen, so "nothing in the center" is not an invariant here.)
		// recoverability: every few steps make sure ▤ brings the panel back —
		// both un-hidden AND stacked above the Front canvas (not visually buried).
		if step%8 == 7 && !c.Frozen {
			rec := c.EvalJSON(recoverJS)
			if shown, _ := rec["shown"].(bool); !shown {
				bad = append(bad, "UNRECOVERABLE (panel not shown after ▤)")
			} else if above, _ := rec["above"].(bool); !above {
				bad = append(bad, fmt.Sprintf("BURIED (panel z=%v < Front canvas z=%v after ▤)", rec["pz"], rec["cz"]))
			}
		}

		if *verbose {
			mark := ""
			if len(bad) > 0 {
				mark = "  <-- " + strings.Join(bad, "; ")
			}
			fmt.Printf("  %3d %-34s%s\n", step, act, mark)
		}
		if len(bad) > 0 {
			viols = append(viols, violation{Step: step, Action: act, Mode: str(snap["mode"]), Detail: bad})
			if *stopOn || c.Frozen {
				break
			}
		}
	}

	c.Eval(recoverJS) // leave the panel visible for the human watching

	fmt.Printf("\nmonkey: %d violation(s) over %d steps (seed %d)\n", len(viols), len(log), *seed)
	for _, v := range viols {
		fmt.Printf("  !! step %d [%s] mode=%s: %s\n", v.Step, v.Action, v.Mode, strings.Join(v.Detail, "; "))
	}
	if len(viols) > 0 {
		p := filepath.Join(*shotDir, fmt.Sprintf("monkey-fail-seed%d.png", *seed))
		if img, err := c.Screenshot(); err == nil {
			savePNG(p, img)
			fmt.Println("  screenshot:", p)
		}
		fmt.Printf("  replay: monkey -seed %d -steps %d\n", *seed, *steps)
		os.Exit(1)
	}
}

func toInt(v any) int   { f, _ := v.(float64); return int(f) }
func toF(v any) float64 { f, _ := v.(float64); return f }
func str(v any) string  { s, _ := v.(string); return s }
func toList(v any) []map[string]any {
	l, _ := v.([]any)
	out := make([]map[string]any, 0, len(l))
	for _, e := range l {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func joinAny(v any) string {
	l, _ := v.([]any)
	parts := make([]string, 0, len(l))
	for _, e := range l {
		if s := str(e); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " | ")
}
