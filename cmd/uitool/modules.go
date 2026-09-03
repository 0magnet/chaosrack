// Subcommand modules: one shot of every module in the rack, plus a manifest
// the README generator reads.
//
//	uitool modules                  # docs/img/module/<id>.jpg + modules.json
//	uitool modules -mm record,view  # just these
//
// Every module is switched on first, because half of them are put away by
// default and a module nobody turned on is a module the reference would be
// missing. Each is then scrolled into view and cropped out of a screenshot by
// its own bounding box — the panel scrolls, so most of them are off screen at
// any one time.
//
// The label and the description in the manifest are the module header's own
// text and tooltip, so the reference says what the interface says.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/rackspec"
)

var (
	modDir    = flag.String("mdir", "docs/img/module", "output directory for the module shots")
	modOnly   = flag.String("mm", "", "comma-separated id filter (default: every module)")
	modPad    = flag.Int("mpad", 6, "padding around a module's box, in px")
	modScale  = flag.Int("mwidth", 0, "scale shots to this width (0 = native)")
	modLayout = flag.String("layout", "docs/stl/layout.json", "also write each module's measured control layout here (empty to skip)")
)

func runModules() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*modDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		os.Exit(1)
	}

	// A known layout: default dock, default scale, nothing put away by a
	// previous session.
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)

	only := map[string]bool{}
	if *modOnly != "" {
		for _, k := range strings.Split(*modOnly, ",") {
			only[strings.TrimSpace(k)] = true
		}
	}

	var out []panelModule
	have := map[string]bool{}
	// The general pass, then one pass per mode that brings its own module —
	// the Pong controls only exist while Pong is the model.
	out = append(out, capturePass(c, "lorenz", "", only, have, true)...)
	for _, mo := range modeOwnedModules {
		out = append(out, capturePass(c, mo.mode, mo.id, only, have, false)...)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		os.Exit(1)
	}
	manifest := filepath.Join(*modDir, "modules.json")
	if err := os.WriteFile(manifest, append(data, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		os.Exit(1)
	}
	fmt.Printf("modules: %d shots + %s\n", len(out), manifest)

	if *modLayout != "" {
		writeLayouts(c)
	}
}

// writeLayouts measures the rack one more time — with everything switched on,
// in the default mode — and saves it for cmd/stlgen.
func writeLayouts(c *cdp.Client) {
	c.Eval(`location.hash='#lorenz'`)
	c.Reload(4 * time.Second)
	waitForPanel(c)
	showEveryModule(c)
	lays := measureLayouts(c)
	if len(lays) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(*modLayout), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		return
	}
	data, err := json.MarshalIndent(lays, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		return
	}
	if err := os.WriteFile(*modLayout, append(data, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "modules:", err)
		return
	}
	var n int
	for _, l := range lays {
		n += len(l.Controls)
	}
	fmt.Printf("modules: %d layouts, %d controls → %s\n", len(lays), n, *modLayout)
}

// modeOwnedModules are the models whose controls only exist while that model
// is selected — the Pong module is not put away when you leave Pong, it is
// gone. Each is visited so its module can be photographed.
//
// The element id is listed with the mode so the pass can wait for THAT
// module rather than for the panel in general. Waiting for the panel is not
// enough: a mode-owned module is added a beat after the rest, and a fixed
// sleep caught a different four of them each run.
var modeOwnedModules = []struct{ mode, id string }{
	{"pong", "pong-module"},
	{"scopetext", "stext-module"},
	{"sprottmorph", "smorph-module"},
	{"bounceball", "bounce-module"},
	{"stlfile", "stlfile-module"},
	{"custom", "eqn-module"},
}

// waitForModule holds until a specific module is on screen with a real box.
func waitForModule(c *cdp.Client, id string) {
	if id == "" {
		return
	}
	for i := 0; i < 40; i++ {
		v, _ := c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);
		  return !!e && e.getBoundingClientRect().width > 4;})()`, id)).(bool)
		if v {
			return
		}
		time.Sleep(400 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "modules: %s never appeared\n", id)
}

// featureSwitches reveal modules from outside the rack's own Modules column:
// the modulation and EQ strips appear with Audio mod, and the Counter, Keys,
// Matrix, Template and Patchbay have their own switches in the Console's
// other columns.
//
// Deliberately not here: Test tone, MIDI and Fullscreen, none of which reveal
// a module and the first of which makes a noise. Nor Edit eqn — it does reveal
// the Equation module, but by switching the app into Custom mode, which threw
// away the mode-owned module every pass had just navigated to. The Equation
// module is photographed in the "custom" pass instead, where it belongs.
var featureSwitches = []string{"audio-mod", "counter-on", "keys-on", "tm-on", "rhythm-on", "tpl-on", "patch-on",
	"analysis-on", "preset-on"}

// capturePass photographs every module visible in one mode that has not been
// photographed already, and returns them in DOM order.
func capturePass(c *cdp.Client, mode, wantID string, only, have map[string]bool, flip bool) []panelModule {
	c.Eval(fmt.Sprintf(`location.hash='#%s&ar=0&rot=25,40,0'`, mode))
	c.Reload(4 * time.Second)
	waitForPanel(c)
	waitForModule(c, wantID)

	// The mode's own modules FIRST, before any switch is touched. One of the
	// feature switches takes the app out of the mode it was just put into, and
	// with the switches flipped first the Pong scoreboard was never on screen
	// during its own pass — every pass listed it with a zero-sized box.
	out := shootRound(c, mode, only, have)
	if flip {
		showEveryModule(c)
		out = append(out, shootRound(c, mode, only, have)...)
	}
	return out
}

// shootRound photographs whatever is on screen and not yet collected.
func shootRound(c *cdp.Client, mode string, only, have map[string]bool) []panelModule {
	mods := listModules(c)
	if len(mods) == 0 {
		fmt.Fprintln(os.Stderr, "modules:", mode, "— found none")
		return nil
	}
	var out []panelModule
	for _, m := range mods {
		if have[m.ID] || (len(only) > 0 && !only[m.ID]) {
			continue
		}
		img, ok := shootModule(c, m.Sel)
		if !ok {
			continue // put away in this mode; another pass will catch it
		}
		rel := filepath.Join(*modDir, m.ID+".jpg")
		saveJPEG(rel, img, 90)
		m.Image = rel
		m.Mode = mode
		m.Sel = ""
		have[m.ID] = true
		out = append(out, m)
		fmt.Printf("  %-18s %s\n", m.ID, m.Label)
	}
	return out
}

// showEveryModule turns on every switch in the Console's Modules column plus
// the handful elsewhere that reveal a module. The Modules column's switches
// are the rack's own, generated from the module list, so that part covers
// whatever exists rather than a list kept in step by hand.
func showEveryModule(c *cdp.Client) {
	c.Eval(`(function(){
	  var host = document.getElementById('module-switches');
	  if (host) {
	    [].forEach.call(host.querySelectorAll('input[type=checkbox]'), function(sw){
	      if (!sw.checked) { sw.checked = true; sw.dispatchEvent(new Event('change',{bubbles:true})); }
	    });
	  }
	})()`)
	c.Eval(fmt.Sprintf(`(function(){
	  %q.split(',').forEach(function(id){
	    var sw = document.getElementById(id);
	    if (sw && !sw.checked) { sw.checked = true; sw.dispatchEvent(new Event('change',{bubbles:true})); }
	  });
	})()`, strings.Join(featureSwitches, ",")))
	time.Sleep(1500 * time.Millisecond)
}

// listModules reads every module's identity out of the live DOM and stamps
// each one with a selector to find it again.
//
// The stamp is the point: a third of the modules carry no id attribute, and
// keying on a name synthesized from the header meant getElementById could not
// find them again — the first run photographed six of thirty-one for exactly
// that reason.
func listModules(c *cdp.Client) []panelModule {
	raw, _ := c.Eval(`(function(){
	  var out = [];
	  [].forEach.call(document.querySelectorAll('.sect'), function(s, i){
	    var hdr = s.querySelector('.sect-hdr');
	    if (!hdr) return;
	    var label = (hdr.textContent||'').trim();
	    var stamp = 'm' + i;
	    s.setAttribute('data-uitool', stamp);
	    var id = s.id ? s.id.replace(/-module$/, '') : label.toLowerCase().replace(/[^a-z0-9]+/g,'-');
	    out.push({id: id, label: label, title: hdr.getAttribute('title')||'', sel: stamp});
	  });
	  return JSON.stringify(out);
	})()`).(string)
	var mods []panelModule
	if err := json.Unmarshal([]byte(raw), &mods); err != nil {
		fmt.Fprintln(os.Stderr, "modules: reading the panel:", err)
		return nil
	}
	return mods
}

// shootModule scrolls one module into view and crops it out of a screenshot.
func shootModule(c *cdp.Client, stamp string) (image.Image, bool) {
	raw, _ := c.Eval(fmt.Sprintf(`(function(){
	  var e = document.querySelector("[data-uitool=" + %q + "]");
	  if (!e) return "";
	  e.scrollIntoView({block:'center', inline:'center'});
	  var r = e.getBoundingClientRect();
	  // The ratio between what a screenshot measures and what the DOM
	  // measures: the renderer is devicePixelRatio-native, so on a HiDPI
	  // display the bitmap is larger than the layout box.
	  return JSON.stringify({x:r.left, y:r.top, w:r.width, h:r.height,
	    vw: window.innerWidth, vh: window.innerHeight});
	})()`, stamp)).(string)
	if raw == "" {
		return nil, false
	}
	var box struct{ X, Y, W, H, VW, VH float64 }
	if err := json.Unmarshal([]byte(raw), &box); err != nil || box.W < 4 || box.H < 4 {
		return nil, false
	}
	// scrollIntoView is not instant — it may animate, and the rect above was
	// read before the scroll settled, so re-read it after a beat.
	time.Sleep(400 * time.Millisecond)
	raw2, _ := c.Eval(fmt.Sprintf(`(function(){
	  var e = document.querySelector("[data-uitool=" + %q + "]"); if (!e) return "";
	  var r = e.getBoundingClientRect();
	  return JSON.stringify({x:r.left, y:r.top, w:r.width, h:r.height,
	    vw: window.innerWidth, vh: window.innerHeight});
	})()`, stamp)).(string)
	if raw2 != "" {
		// A re-read that fails leaves the first rect in place, which is the
		// right fallback — it was good enough to get here.
		if err := json.Unmarshal([]byte(raw2), &box); err != nil {
			fmt.Fprintln(os.Stderr, "modules: re-reading the box:", err)
		}
	}

	img, err := c.Screenshot()
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	scale := 1.0
	if box.VW > 0 {
		scale = float64(b.Dx()) / box.VW
	}
	pad := float64(*modPad)
	r := image.Rect(
		int((box.X-pad)*scale), int((box.Y-pad)*scale),
		int((box.X+box.W+pad)*scale), int((box.Y+box.H+pad)*scale),
	).Intersect(b)
	if r.Dx() < 4 || r.Dy() < 4 {
		return nil, false
	}
	out := cropTo(img, r)
	if *modScale > 0 && r.Dx() > *modScale {
		h := r.Dy() * *modScale / r.Dx()
		out = scaleBox(out, *modScale, h)
	}
	return out, true
}

// measureLayouts reads each module's real control layout out of the DOM and
// writes it as millimeters, for the STL export to build an exact panel from.
//
// This is the difference between a model of the rack and a model of something
// that looks like the rack: the panels were hand-placed before — three knobs
// down the middle on a guessed pitch — which is a plausible module and not
// any actual one.
//
// The conversion is the panel's own declared scale (rackspec.PxPerMM), and
// the Y axis is flipped: the DOM measures down from the top, a panel is
// dimensioned up from the bottom.
func measureLayouts(c *cdp.Client) []meshstl.PanelLayout {
	raw, ok := c.Eval(fmt.Sprintf(`(function(){
	  var MM = %v, HP = %v, SEAM = %v;
	  // What each control is, in the order it should be tested: the first
	  // selector that matches decides the kind.
	  var KINDS = [
	    ['.mxpin',      %d],
	    ['.pslot',      %d],
	    ['.knob-inner', %d],
	    ['.knob-ring',  %d],
	    ['input.sw',    %d],
	    ['.pushbtn',    %d],
	    ['.led',        %d]
	  ];
	  var out = [];
	  [].forEach.call(document.querySelectorAll('.sect'), function(s){
	    var hdr = s.querySelector('.sect-hdr');
	    var r = s.getBoundingClientRect();
	    if (r.width < 4 || r.height < 4) return;
	    var hp = Math.round((r.width/MM + SEAM) / HP);
	    var ctl = [];
	    var seen = [];
	    KINDS.forEach(function(k){
	      [].forEach.call(s.querySelectorAll(k[0]), function(e){
	        if (seen.indexOf(e) >= 0) return;
	        seen.push(e);
	        var b = e.getBoundingClientRect();
	        if (b.width < 1) return;
	        ctl.push({
	          x: (b.left + b.width/2 - r.left) / MM,
	          y: (r.bottom - (b.top + b.height/2)) / MM,
	          kind: k[1],
	          diam: b.width / MM
	        });
	      });
	    });
	    out.push({
	      id: s.id ? s.id.replace(/-module$/,'') : (hdr?hdr.textContent.trim():'').toLowerCase().replace(/[^a-z0-9]+/g,'-'),
	      label: hdr ? hdr.textContent.trim() : '',
	      hp: hp,
	      controls: ctl
	    });
	  });
	  return JSON.stringify(out);
	})()`,
		rackspec.PxPerMM, rackspec.HP, rackspec.Seam,
		meshstl.PinControl, meshstl.ButtonControl,
		meshstl.KnobControl, meshstl.KnobControl,
		meshstl.ToggleControl, meshstl.ButtonControl, meshstl.LEDControl,
	)).(string)
	if !ok {
		fmt.Fprintln(os.Stderr, "modules: could not measure the layouts")
		return nil
	}
	var out []meshstl.PanelLayout
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		fmt.Fprintln(os.Stderr, "modules: reading the layouts:", err)
		return nil
	}
	return out
}
