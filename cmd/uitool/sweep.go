// Subcommand sweep: every model and every switch, in a fixed order.
//
// The monkey finds what a random walk happens into, and replaying its failure
// means a seed and a step number. The two bugs it found last were both
// ORDER-dependent and neither needed luck once the order was known: the rack
// packer looped for ever on the Takens row, but only after the models before it
// had been visited, and a dial's listener was freed while still attached, but
// only noticed when that one knob was scrolled after that one rebuild.
//
// So this walks the rack deterministically instead:
//
//  1. every model in #mode-select, in catalog order;
//  2. on the first model of each category row, every switch on the panel,
//     flipped and flipped back, and every knob turned a detent each way.
//
// After each step it checks what the monkey checks — no new page error, a
// panel that is still there, a main thread that still answers, no NaN in the
// permalink or on an LED — and three things the monkey cannot:
//
//   - a listener an arena released while it was still attached (pkg/dom's
//     arena check, on whenever the page was opened with ?arenacheck, which
//     -headless does). It is reported at the rebuild, whether or not anything
//     is ever clicked;
//   - a bay whose modules overflow it;
//   - a layout shift: a switch or a knob that moves, resizes, adds or
//     removes anything on the panel (-shift). A control changes what the
//     panel reads, never where anything is.
//
// A failure names the model and the control, which is the whole replay.
//
// Usage:
//
//	uitool sweep -headless -serve ./chaosrack   # nothing on screen, nothing running first
//	uitool sweep -target 127.0.0.1:8305         # an open tab (it comes to the front)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0magnet/cdp"
)

var (
	sweepSwitches = flag.Bool("switches", true, "sweep: also flip every switch, on the first model of each category")
	sweepSettle   = flag.Int("step-settle", 60, "sweep: ms to let the page settle after each step, beyond two frames")
	sweepRender   = flag.Bool("render", false, "sweep: keep the render loop running; off by default, since the sweep checks controls and a headless browser draws on the CPU")
	sweepShift    = flag.Bool("shift", true, "sweep: report any module that moves, resizes, or gains or loses a control when a switch is flipped or a knob turned")
)

// settleJS waits two frames: a rebuild scheduled by the step has run, and so
// has the layout it queued.
const settleJS = `new Promise(function(r){requestAnimationFrame(function(){requestAnimationFrame(function(){r(1)})})})`

// sweepSnapJS is snapJS plus the arena leaks and the bays that overflow.
const sweepSnapJS = `(function(){
  var p=document.getElementById('controls-panel'),lb=[],over=[];
  if(p){
    [].forEach.call(p.querySelectorAll('.led,.numin'),function(e){var t=(e.value||e.textContent||'').trim();
      if(/NaN|undefined|Infinity/i.test(t))lb.push((e.id||e.className||'').slice(0,16)+'='+t.slice(0,10));});
    [].forEach.call(p.querySelectorAll('.runit-panel'),function(u,i){
      if(u.offsetParent&&u.scrollWidth>u.clientWidth+2)over.push('bay '+(i+1)+' '+u.scrollWidth+'>'+u.clientWidth);});
  }
  return JSON.stringify({
    nErr:(window.__errs||[]).length, lastErr:(window.__errs||[]).slice(-3),
    nLeak:(window.__domArenaLeaks||[]).length, lastLeak:(window.__domArenaLeaks||[]).slice(-3),
    hashBad:/NaN|undefined|Infinity/.test(location.hash),
    panel:!!p, mode:(document.getElementById('mode-select')||{}).value||'',
    ledBad:lb.slice(0,6), over:over
  });
})()`

// modelsJS lists the models, and which of them opens a category row: the
// first of each optgroup, or of the select when it has none.
const modelsJS = `(function(){
  var s=document.getElementById('mode-select'); if(!s) return '[]';
  var seen={};
  return JSON.stringify([].map.call(s.options,function(o){
    var g=o.parentNode.tagName==='OPTGROUP'?o.parentNode.label:'';
    var first=!seen[g]; seen[g]=1;
    return {v:o.value, first:first};
  }));
})()`

// switchesJS lists the visible switches by a selector that finds each again
// after the rebuild its own click may cause.
const switchesJS = `(function(){
  var p=document.getElementById('controls-panel'),out=[];
  if(p)[].forEach.call(p.querySelectorAll('.sw:not(#fullscreen-sw)'),function(e){
    if(!e.offsetParent)return;
    var input=e.querySelector('input[id]'), id=e.id||(input&&input.id);
    if(id)out.push(id);
  });
  return JSON.stringify(out);
})()`

// flipJS clicks the switch named by id (the switch itself, or the switch that
// holds the input with that id) and reports whether it was still there.
const flipJS = `(function(id){
  var e=document.getElementById(id); if(!e) return false;
  var sw=e.classList.contains('sw')?e:e.closest('.sw'); (sw||e).click(); return true;
})(%q)`

type sweeper struct {
	c        *cdp.Client
	baseErr  int
	baseLeak int
	fails    []string
	shifts   []string // layout shifts: reported, not yet failures (see -shift)
	steps    int
}

func runSweep() {
	c, err := dial()
	if err != nil {
		fmt.Println("sweep:", err)
		exit(1)
	}
	defer c.Close() //nolint:errcheck // the process is ending
	c.Eval(setupJS)
	s := &sweeper{c: c}
	base := c.EvalJSON(sweepSnapJS)
	s.baseErr, s.baseLeak = toInt(base["nErr"]), toInt(base["nLeak"])
	fmt.Printf("sweep: %s\n", c.URL)
	if base["nLeak"] == nil {
		fmt.Println("  (arena check off: open the page with ?arenacheck, or use -headless)")
	}
	// The picture is not what the sweep checks, and headless it is drawn by
	// SwiftShader on the CPU: a Lorenz trajectory every frame held two to
	// five cores for the whole run. Power off stops the render loop and
	// leaves every control working.
	if !*sweepRender {
		c.Eval(`(function(){var p=document.getElementById('power-sw');if(p&&p.checked){p.checked=false;p.dispatchEvent(new Event('change'));}})()`)
	}
	s.check("load", "")

	models := toList(anyJSON(c.Eval(modelsJS)))
	if *shotModels != "" {
		models = models[:0]
		for i, m := range strings.Split(*shotModels, ",") {
			models = append(models, map[string]any{"v": strings.TrimSpace(m), "first": i == 0})
		}
	}
	start := time.Now()
	for _, m := range models {
		mode := str(m["v"])
		c.Eval(fmt.Sprintf(`(function(){var s=document.getElementById('mode-select');s.value=%q;s.dispatchEvent(new Event('change'));})()`, mode))
		if !s.check("model "+mode, mode) {
			break
		}
	}
	fmt.Printf("  %d models in %s\n", len(models), time.Since(start).Round(time.Millisecond))

	if *sweepSwitches && !c.Frozen() {
		start, flips, turns := time.Now(), 0, 0
		for _, m := range models {
			if first, _ := m["first"].(bool); !first {
				continue
			}
			mode := str(m["v"])
			c.Eval(fmt.Sprintf(`(function(){var s=document.getElementById('mode-select');s.value=%q;s.dispatchEvent(new Event('change'));})()`, mode))
			s.check("model "+mode, mode)
			var ids []string
			for _, v := range toAnyList(anyJSON(c.Eval(switchesJS))) {
				ids = append(ids, str(v))
			}
			for _, id := range ids {
				for _, way := range []string{"on", "back"} {
					before := s.layout()
					if ok, _ := c.Eval(fmt.Sprintf(flipJS, id)).(bool); !ok {
						break // a flip before it took this one off the panel
					}
					flips++
					step := fmt.Sprintf("%s: flip %s %s", mode, id, way)
					if !s.check(step, mode) {
						goto done
					}
					s.shifted(step, before)
				}
			}
			// Every knob on the panel, a detent each way: turning a knob
			// may change a readout and nothing else.
			n := toInt(c.Eval(knobCountJS))
			for i := range n {
				for _, dy := range []float64{-120, 120} {
					at := c.EvalJSON(fmt.Sprintf(knobAtJS, i))
					if at == nil || at["x"] == nil {
						break
					}
					before := s.layout()
					c.Wheel(toF(at["x"]), toF(at["y"]), dy)
					turns++
					step := fmt.Sprintf("%s: turn knob %d (%s) %+g", mode, i, str(at["t"]), dy)
					if !s.check(step, mode) {
						goto done
					}
					s.shifted(step, before)
				}
			}
		}
	done:
		fmt.Printf("  %d switch flips, %d knob turns in %s\n", flips, turns, time.Since(start).Round(time.Millisecond))
	}

	if len(s.shifts) > 0 {
		// Warnings for now: the surface is still being revised, and the
		// modules a switch reveals are being dealt with one at a time.
		fmt.Printf("\nsweep: %d layout shift(s)\n", len(s.shifts))
	}
	fmt.Printf("\nsweep: %d failure(s) over %d steps\n", len(s.fails), s.steps)
	for _, f := range s.fails {
		fmt.Println("  !!", f)
	}
	if len(s.fails) > 0 {
		exit(1)
	}
}

// check lets the page settle and reports whatever broke, returning false
// when the page is too broken to go on (frozen, or the panel gone).
func (s *sweeper) check(step, mode string) bool {
	s.steps++
	s.c.Eval(settleJS)
	if *sweepSettle > 0 {
		time.Sleep(time.Duration(*sweepSettle) * time.Millisecond)
	}
	snap := s.c.EvalJSON(sweepSnapJS)
	var bad []string
	if s.c.Frozen() {
		bad = append(bad, "FROZEN (the main thread stopped answering)")
	}
	if n := toInt(snap["nErr"]); n > s.baseErr {
		bad = append(bad, "JSERR: "+joinAny(snap["lastErr"]))
		s.baseErr = n
	}
	if n := toInt(snap["nLeak"]); n > s.baseLeak {
		bad = append(bad, "LEAK (released listener still attached): "+joinAny(snap["lastLeak"]))
		s.baseLeak = n
	}
	if b, _ := snap["hashBad"].(bool); b {
		bad = append(bad, "BAD-HASH (NaN/undefined/Infinity in permalink)")
	}
	if lb := joinAny(snap["ledBad"]); lb != "" {
		bad = append(bad, "LED-GARBAGE: "+lb)
	}
	if o := joinAny(snap["over"]); o != "" {
		bad = append(bad, "BAY-OVERFLOW: "+o)
	}
	if p, _ := snap["panel"].(bool); !p && !s.c.Frozen() {
		bad = append(bad, "PANEL-GONE")
	}
	if len(bad) == 0 {
		return true
	}
	if mode != "" && !strings.Contains(step, mode) {
		step = mode + ": " + step
	}
	s.fails = append(s.fails, step+": "+strings.Join(bad, "; "))
	fmt.Printf("  !! %s: %s\n", step, strings.Join(bad, "; "))
	return !s.c.Frozen() && (snap["panel"] == true)
}

// anyJSON decodes the JSON string a script returned.
func anyJSON(v any) any {
	s, _ := v.(string)
	var out any
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out) //nolint:errcheck // a bad answer is an empty list
	}
	return out
}

func toAnyList(v any) []any { l, _ := v.([]any); return l }

// knobCountJS and knobAtJS find the panel's knobs by position in document
// order — a knob has no id of its own — scrolling each into view to turn it.
// Not the MODEL knobs: turning one is a model change, which the first phase
// already makes for every model, and which is allowed to change the panel.
const knobCountJS = `(function(){var p=document.getElementById('controls-panel');if(!p)return 0;
  return [].filter.call(p.querySelectorAll('.knob:not(.knob-fine)'),function(k){return k.offsetParent&&!k.closest('.bankblank,.catcell');}).length;})()`

const knobAtJS = `(function(i){var p=document.getElementById('controls-panel');
  var ks=[].filter.call(p.querySelectorAll('.knob:not(.knob-fine)'),function(k){return k.offsetParent&&!k.closest('.bankblank,.catcell');});
  var k=ks[i];if(!k)return '{}';k.scrollIntoView({block:'center'});var r=k.getBoundingClientRect();
  var s=k.closest('.sect'),h=s&&s.querySelector('.sect-hdr');
  return JSON.stringify({x:r.left+r.width/2,y:r.top+r.height/2,t:(h?h.textContent:'')+' '+(k.title||'').slice(0,40)});})(%d)`

// layoutJS is every visible module's box, relative to the rack rather than
// the viewport so scrolling does not read as movement, and how many knobs and
// switches it shows.
const layoutJS = `(function(){var p=document.getElementById('controls-panel'),out={};if(!p)return '{}';
  var m=p.querySelector('.modules')||p,o=m.getBoundingClientRect();
  [].forEach.call(p.querySelectorAll('.sect'),function(s){if(!s.offsetParent)return;
    var r=s.getBoundingClientRect(),h=s.querySelector('.sect-hdr'),key=s.id||(h?h.textContent:'?');
    var vis=function(q){return [].filter.call(s.querySelectorAll(q),function(e){return e.offsetParent;}).length;};
    out[key]=[Math.round(r.left-o.left),Math.round(r.top-o.top),Math.round(r.width),Math.round(r.height),vis('.knob:not(.knob-fine)'),vis('.sw,input[type=checkbox]')];});
  return JSON.stringify(out);})()`

// layout is the rack's shape now, for shifted to compare against.
func (s *sweeper) layout() map[string]any {
	if !*sweepShift {
		return nil
	}
	return s.c.EvalJSON(layoutJS)
}

// shifted reports what a step moved, resized, added or took away. A control
// changes what it reads, not where anything is.
func (s *sweeper) shifted(step string, before map[string]any) {
	if before == nil {
		return
	}
	after := s.c.EvalJSON(layoutJS)
	var diffs []string
	for k, b := range before {
		a, ok := after[k]
		switch {
		case !ok:
			diffs = append(diffs, k+" gone")
		case fmt.Sprint(a) != fmt.Sprint(b):
			diffs = append(diffs, fmt.Sprintf("%s %v→%v", k, b, a))
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			diffs = append(diffs, k+" appeared")
		}
	}
	if len(diffs) == 0 {
		return
	}
	sort.Strings(diffs)
	if len(diffs) > 4 {
		diffs = append(diffs[:4], fmt.Sprintf("…and %d more", len(diffs)-4))
	}
	msg := step + ": [x y w h knobs switches] " + strings.Join(diffs, "; ")
	s.shifts = append(s.shifts, msg)
	fmt.Println("  ~~ SHIFT", msg)
}
