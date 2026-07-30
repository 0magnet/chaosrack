// Subcommand demo: record a chaos-monkey run as a demo reel — panel hidden,
// the monkey driving ONLY state that changes what's on screen (modes, params,
// colors, gradient knobs, pose/zoom/pan, spin, trail, persist, ring, points,
// speed, canvas drags), never the panel plumbing (dock, knob style, LED
// color, interface size, template…). One frame is captured after every
// action, so at playback speed something changes every frame.
//
//	uitool demo                       # writes docs/img/gif/monkey.gif + frames
//	uitool demo -demo-steps 300 -demo-seed 7
//
// Frames are also written as PNGs (for a high-quality ffmpeg encode):
//
//	ffmpeg -framerate 10 -i <frames>/f%04d.png -pix_fmt yuv420p demo.mp4
package main

import (
	"flag"
	"fmt"
	"image"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/0magnet/wasm-stuff/internal/cdp"
)

var (
	demoSteps   = flag.Int("demo-steps", 240, "actions (= frames) to record")
	demoSeed    = flag.Int64("demo-seed", 1, "RNG seed")
	demoOut     = flag.String("demo-out", "docs/img/gif/monkey.gif", "output GIF path")
	demoFrames  = flag.String("demo-frames", "", "also write per-frame PNGs to this dir (for ffmpeg)")
	demoPx      = flag.Int("demo-px", 800, "output width in px (height keeps aspect)")
	demoMonkeys = flag.Int("demo-monkeys", 1, "concurrent action streams (interleaved)")
	demoDur     = flag.Int("demo-dur", 0, "run for this many seconds instead of -demo-steps (implies no capture; for external screen recording)")
	demoEpoch   = flag.Int64("demo-epoch", 0, "performance mode: recording start as unix seconds — shows a 10s on-screen countdown, keeps ALL speaker-output switches untouched, and runs specialized monkey roles until epoch+dur")
)

// demoModes tours the whole visual surface: attractors, parametric curves,
// geometry (incl. skinnable solids), and the audio modes.
var demoModes = []string{"lorenz", "rossler", "chua", "aizawa", "sprott", "thomas",
	"halvorsen", "chen", "dadras", "rabinovich", "burkeshaw", "lu", "newtonleipnik",
	"hyperrossler", "sprottb", "sprottf", "sprottl", "sprottp", "lissajou", "graphicartist",
	"spectrogram", "xy", "fvf", "cube", "torus", "sphere", "globe", "magnetosphere",
	"tetrahedron", "dodecahedron", "icosahedron", "nestedcube"}

func runDemo() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
	if *demoEpoch > 0 {
		runPerformance(c)
		return
	}
	rng := rand.New(rand.NewSource(*demoSeed))
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)
	c.Eval(`location.hash='#lorenz&rot=20,0,0'`)
	c.Reload(3 * time.Second)
	// Test tone on for the whole run so the spectrogram / xy / FVF modes and
	// the audio backdrops have live content even on a silent feed.
	c.Eval(`(function(){var t=document.getElementById('test-tone');if(t&&!t.checked){t.checked=true;t.dispatchEvent(new Event('change',{bubbles:true}));}})()`)
	// WebAudio kickstart with a REAL click (the autoplay gesture): engage
	// Model Out on the CAM ring so the attractor is HEARD — the soundtrack
	// tracks the visuals. Must happen before the panel is hidden.
	if pos := c.EvalJSON(`(function(){
	  var m=document.getElementById('sonify-module'); if(!m) return '{}';
	  m.scrollIntoView({inline:'end'});
	  var labs=[].slice.call(m.querySelectorAll('.knob-dial-lab')).filter(function(e){return e.textContent==='CAM';});
	  if(!labs.length) return '{}';
	  var r=labs[0].getBoundingClientRect();
	  return JSON.stringify({x:Math.round(r.left+r.width/2), y:Math.round(r.top+r.height/2)});
	})()`); pos["x"] != nil {
		x, _ := pos["x"].(float64)
		y, _ := pos["y"].(float64)
		c.Click(x, y)
		time.Sleep(300 * time.Millisecond)
	}
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='none';
	  ['runtime'].forEach(function(id){var e=document.getElementById(id);if(e)e.style.display='none';});
	  [].forEach.call(document.querySelectorAll('button[title^="Show / hide controls"],a'),function(e){e.style.display='none';});})()`)
	time.Sleep(300 * time.Millisecond)

	setSlider := func(id string, v float64) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value='%g';e.dispatchEvent(new Event('input',{bubbles:true}));}})()`, id, v))
	}
	setSel := func(id string, v string) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value=%q;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, id, v))
	}
	toggle := func(id string) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.checked=!e.checked;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, id))
	}
	randHex := func() string { return fmt.Sprintf("#%02x%02x%02x", rng.Intn(256), rng.Intn(256), rng.Intn(256)) }

	// Nudge a RANDOM visible param knob's hidden slider to a random position
	// within its own range — the trajectory reshapes live.
	nudgeParam := func() {
		c.Eval(fmt.Sprintf(`(function(){
		  var sl=[].slice.call(document.querySelectorAll('#params input[type=range]'));
		  if(!sl.length) return;
		  var e=sl[%d%%sl.length], mn=parseFloat(e.min), mx=parseFloat(e.max);
		  e.value=String(mn+(mx-mn)*%g); e.dispatchEvent(new Event('input',{bubbles:true}));
		})()`, rng.Intn(1<<30), rng.Float64()))
	}

	var frames []*image.RGBA
	if *demoFrames != "" {
		_ = os.MkdirAll(*demoFrames, 0o750)
	}
	act := ""
	rngs := []*rand.Rand{rng}
	for i := 1; i < *demoMonkeys; i++ {
		rngs = append(rngs, rand.New(rand.NewSource(*demoSeed+int64(i)*7919)))
	}
	deadline := time.Time{}
	if *demoDur > 0 {
		deadline = time.Now().Add(time.Duration(*demoDur) * time.Second)
	}
	for step := 0; *demoDur > 0 || step < *demoSteps; step++ {
		if *demoDur > 0 && time.Now().After(deadline) {
			break
		}
		rng := rngs[step%len(rngs)]
		switch r := rng.Float64(); {
		case r < 0.12:
			m := demoModes[rng.Intn(len(demoModes))]
			act = "mode " + m
			setSel("mode-select", m)
			time.Sleep(400 * time.Millisecond)
		case r < 0.38:
			act = "param"
			nudgeParam()
		case r < 0.50: // recolor a gradient stop / background
			ids := []string{"color-base", "color-mid", "color-top", "color-base", "color-top", "color-bg"}
			id := ids[rng.Intn(len(ids))]
			act = "recolor " + id
			c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value=%q;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));}})()`,
				id, randHex()))
		case r < 0.58: // gradient knobs
			if rng.Intn(2) == 0 {
				act = "gradient-source"
				setSel("gradient-source", fmt.Sprint(rng.Intn(4)))
			} else {
				act = "gradient-colors"
				setSel("gradient-colors", fmt.Sprint(1+rng.Intn(4)))
			}
		case r < 0.66: // pose drag on the canvas (real input; the panel is hidden)
			act = "drag"
			x, y := 600+rng.Intn(700), 200+rng.Intn(500)
			c.Drag(float64(x), float64(y), float64(x+rng.Intn(360)-180), float64(y+rng.Intn(360)-180), 6)
		case r < 0.74: // spin rates
			axis := []string{"rotation-controls-x", "rotation-controls-y", "rotation-controls-z"}[rng.Intn(3)]
			act = "spin " + axis
			setSlider(axis, float64(rng.Intn(21)-10)/10)
		case r < 0.80: // zoom / pan — pans kept SMALL (±2) and recentered often,
			// so the model never parks at a screen edge for long stretches.
			switch rng.Intn(4) {
			case 0:
				act = "zoom"
				setSlider("camera-zoom", float64(rng.Intn(70)-35))
			case 1:
				act = "pan-x"
				setSlider("pan-x", float64(rng.Intn(5)-2))
			case 2:
				act = "pan-y"
				setSlider("pan-y", float64(rng.Intn(5)-2))
			default: // recenter
				act = "recenter"
				setSlider("pan-x", 0)
				setSlider("pan-y", 0)
			}
		case r < 0.86: // trail length / rainbow period / speed
			switch rng.Intn(3) {
			case 0:
				act = "trail"
				setSlider("trail-slider", float64(2000+rng.Intn(80000)))
			case 1:
				act = "rainbow"
				setSlider("rainbow-freq", 0.2+rng.Float64()*6)
			default:
				act = "speed"
				setSlider("speed-slider", rng.Float64()*1.4-0.2)
			}
		case r < 0.94:
			id := []string{"persist-trail", "ring-sw", "use-points", "gradient-reverse",
				"bg-spectro", "bg-xy", "spectro-skin", "audio-mod"}[rng.Intn(8)]
			act = "toggle " + id
			toggle(id)
		default: // sonification: retune / remap Model Out so the soundtrack moves
			if rng.Intn(2) == 0 {
				act = "sonify-freq"
				setSlider("sonify-freq", float64(12+rng.Intn(60)))
			} else {
				act = "sonify-map"
				setSel("sonify-map", []string{"cam", "xy", "xz", "yz"}[rng.Intn(4)])
			}
		}
		fmt.Printf("  %3d %s\n", step, act)
		if *demoDur > 0 { // external recorder is filming — no CDP screenshots,
			// just a fast action cadence across the streams
			fmt.Printf("  %3d %s\n", step, act)
			time.Sleep(time.Duration(140/len(rngs)) * time.Millisecond)
			continue
		}
		img, err := c.Screenshot()
		if err != nil {
			continue
		}
		b := img.Bounds()
		w := *demoPx
		h := b.Dy() * w / b.Dx()
		fr := scaleBox(img, w, h)
		frames = append(frames, fr)
		if *demoFrames != "" {
			savePNG(filepath.Join(*demoFrames, fmt.Sprintf("f%04d.png", len(frames)-1)), fr)
		}
	}

	// restore the tab
	c.Eval(`location.hash='#lorenz'`)
	c.Reload(2 * time.Second)

	if *demoDur > 0 {
		fmt.Println("demo: timed run complete (external recording)")
		return
	}
	if len(frames) == 0 {
		fmt.Fprintln(os.Stderr, "demo: no frames captured")
		os.Exit(1)
	}
	pal := adaptivePalette(frames)
	memo := make(map[uint32]uint8, 4096)
	pf := make([]*image.Paletted, len(frames))
	for i, f := range frames {
		pf[i] = palettize(f, pal, memo)
	}
	writeGif(*demoOut, pf)
	fmt.Printf("demo: %s (%d frames)\n", *demoOut, len(pf))
}

// runPerformance: the recording-day mode. Differences from the plain demo:
// nothing the monkeys do makes sound (no Model Out, no test tone — the
// soundtrack is whatever the operator plays), audio-mod is ON from the start
// so that soundtrack MODULATES the visuals, one monkey specializes in
// re-routing that modulation (channels + levels per param), and a big
// on-screen 10-second countdown cues the recording start.
func runPerformance(c *cdp.Client) {
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)
	c.Eval(`location.hash='#lorenz&rot=20,0,0'`)
	c.Reload(3 * time.Second)
	// Audio-mod ON (visual: routes the incoming feed into the knobs) — but
	// keep the top-left feature METERS hidden: this is a performance, not a
	// mixing console.
	c.Eval(`(function(){var m=document.getElementById('show-meters');if(m&&m.checked){m.checked=false;m.dispatchEvent(new Event('change',{bubbles:true}));}
	  var e=document.getElementById('audio-mod');if(e&&!e.checked){e.checked=true;e.dispatchEvent(new Event('change',{bubbles:true}));}
	  var am=document.getElementById('audio-meters');if(am)am.style.display='none';})()`)
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='none';
	  ['runtime'].forEach(function(id){var e=document.getElementById(id);if(e)e.style.display='none';});
	  [].forEach.call(document.querySelectorAll('button[title^="Show / hide controls"],a'),function(e){e.style.display='none';});})()`)

	setSlider := func(id string, v float64) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value='%g';e.dispatchEvent(new Event('input',{bubbles:true}));}})()`, id, v))
	}
	setSel := func(id string, v string) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value=%q;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, id, v))
	}
	toggle := func(id string) {
		c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.checked=!e.checked;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, id))
	}

	// Countdown: big center numbers from T-10s; removed at T0.
	start := time.Unix(*demoEpoch, 0)
	for time.Now().Before(start.Add(-10 * time.Second)) {
		time.Sleep(100 * time.Millisecond)
	}
	for n := 10; n >= 1; n-- {
		c.Eval(fmt.Sprintf(`(function(n){var d=document.getElementById('cdown');if(!d){d=document.createElement('div');d.id='cdown';d.style.cssText='position:fixed;inset:0;display:flex;align-items:center;justify-content:center;font:700 260px Chakra Petch,monospace;color:#7fe0a0;z-index:99;pointer-events:none;text-shadow:0 0 60px #1e7d3a;';document.body.appendChild(d);}d.textContent=String(n);})(%d)`, n))
		wait := start.Add(time.Duration(-n+1) * time.Second)
		for time.Now().Before(wait) {
			time.Sleep(20 * time.Millisecond)
		}
	}
	c.Eval(`(function(){var d=document.getElementById('cdown');if(d)d.remove();})()`)
	fmt.Println("GO", time.Now().Unix())

	end := start.Add(time.Duration(*demoDur) * time.Second)
	nStreams := *demoMonkeys
	if nStreams < 1 {
		nStreams = 1
	}
	rngs := make([]*rand.Rand, nStreams)
	for i := range rngs {
		rngs[i] = rand.New(rand.NewSource(*demoSeed + int64(i)*7919))
	}
	step := 0
	var lastSuper image.Image
	posRouted := 0
	// Planned mode TOUR: a shuffled pass through the ENTIRE catalog, hopping
	// every ~6.5s — random hops kept missing whole families of models.
	tour := append([]string(nil), demoModes...)
	rngs[0].Shuffle(len(tour), func(i, j int) { tour[i], tour[j] = tour[j], tour[i] })
	tourIdx := 0
	lastHop := time.Now()
	for time.Now().Before(end.Add(8 * time.Second)) { // small overrun past the tape
		mk := step % nStreams
		rng := rngs[mk]
		role := mk % 5
		if nStreams < 5 { // few monkeys still cover every role, just slower
			role = rng.Intn(5)
		}
		switch role {
		case 0: // pose & mode monkey — timed tour through the whole catalog
			if time.Since(lastHop) > 6500*time.Millisecond {
				setSel("mode-select", tour[tourIdx%len(tour)])
				tourIdx++
				lastHop = time.Now()
				time.Sleep(350 * time.Millisecond)
				// resume accumulating in the new mode right away
				c.Eval(`(function(){var e=document.getElementById('persist-trail');if(e&&!e.checked){e.checked=true;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`)
			} else {
				x, y := 600+rng.Intn(700), 200+rng.Intn(500)
				c.Drag(float64(x), float64(y), float64(x+rng.Intn(360)-180), float64(y+rng.Intn(360)-180), 5)
			}
		case 1: // parameter monkey
			c.Eval(fmt.Sprintf(`(function(){var sl=[].slice.call(document.querySelectorAll('#params input[type=range]'));if(!sl.length)return;var e=sl[%d%%sl.length],mn=parseFloat(e.min),mx=parseFloat(e.max);e.value=String(mn+(mx-mn)*%g);e.dispatchEvent(new Event('input',{bubbles:true}));})()`, rng.Intn(1<<30), rng.Float64()))
		case 2: // color monkey
			if rng.Float64() < 0.6 {
				ids := []string{"color-base", "color-mid", "color-top", "color-base", "color-top", "color-bg"}
				c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById(%q);if(e){e.value=%q;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));}})()`,
					ids[rng.Intn(len(ids))], fmt.Sprintf("#%02x%02x%02x", rng.Intn(256), rng.Intn(256), rng.Intn(256))))
			} else if rng.Intn(2) == 0 {
				setSel("gradient-source", fmt.Sprint(rng.Intn(4)))
			} else {
				setSel("gradient-colors", fmt.Sprint(1+rng.Intn(4)))
			}
		case 3: // audio-mod routing monkey — split personalities: attractor
			// PARAM targets (symbol-labeled cards) get whisper-level
			// modulation (they're trivially overdriven), while VIEW targets
			// (zoom/pan/spin — word-labeled) get enough level that the model
			// visibly moves to the music.
			switch rng.Intn(4) {
			case 0: // route something to a random (non-off) channel
				c.Eval(fmt.Sprintf(`(function(){var s=[].slice.call(document.querySelectorAll('.punit-mod select'));if(!s.length)return;var e=s[%d%%s.length],o=e.options;e.selectedIndex=1+%d%%(o.length-1);e.dispatchEvent(new Event('change',{bubbles:true}));})()`, rng.Intn(1<<30), rng.Intn(1<<30)))
			case 1: // un-route one (keeps the wired set small)
				c.Eval(fmt.Sprintf(`(function(){var s=[].slice.call(document.querySelectorAll('.punit-mod select'));if(!s.length)return;var e=s[%d%%s.length];e.selectedIndex=0;e.dispatchEvent(new Event('change',{bubbles:true}));})()`, rng.Intn(1<<30)))
			case 2: // param level: WHISPER (50.5–52.5%% of ±4 ⇒ |lvl| ≤ ~0.2)
				c.Eval(fmt.Sprintf(`(function(){var s=[].slice.call(document.querySelectorAll('.punit-mod')).filter(function(m){var l=m.querySelector('.u-modlbl');return l&&l.classList.contains('sym');});if(!s.length)return;var e=s[%d%%s.length].querySelector('input[type=range]');if(!e)return;var mn=parseFloat(e.min),mx=parseFloat(e.max);e.value=String(mn+(mx-mn)*%g);e.dispatchEvent(new Event('input',{bubbles:true}));})()`, rng.Intn(1<<30), 0.505+rng.Float64()*0.02))
			default: // view level: DANCE (56–68%%) — zoom/pan/spin move to the music
				c.Eval(fmt.Sprintf(`(function(){var s=[].slice.call(document.querySelectorAll('.punit-mod')).filter(function(m){var l=m.querySelector('.u-modlbl');return l&&!l.classList.contains('sym');});if(!s.length)return;var e=s[%d%%s.length].querySelector('input[type=range]');if(!e)return;var mn=parseFloat(e.min),mx=parseFloat(e.max);e.value=String(mn+(mx-mn)*%g);e.dispatchEvent(new Event('input',{bubbles:true}));})()`, rng.Intn(1<<30), 0.56+rng.Float64()*0.12))
			}
		default: // motion & texture monkey (no speaker-output switches)
			switch rng.Intn(10) {
			case 0:
				setSlider("trail-slider", float64(2000+rng.Intn(80000)))
			case 1:
				setSlider("rainbow-freq", 0.2+rng.Float64()*6)
			case 2:
				setSlider("speed-slider", rng.Float64()*1.4-0.2)
			case 3:
				setSlider([]string{"rotation-controls-x", "rotation-controls-y", "rotation-controls-z"}[rng.Intn(3)], float64(rng.Intn(21)-10)/10)
			case 4: // mild zoom only — extremes shrink the model to a dot or clip it
				setSlider("camera-zoom", float64(rng.Intn(36)-20))
			case 5: // recenter fully: pans AND zoom
				setSlider("pan-x", 0)
				setSlider("pan-y", 0)
				setSlider("camera-zoom", 0)
			case 6:
				// phosphor flip (visual: trace color + CRT afterglow)
				c.Eval(fmt.Sprintf(`(function(){var e=document.getElementById('phosphor');if(e&&e.options.length){e.selectedIndex=%d%%e.options.length;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, rng.Intn(1<<30)))
			case 7: // persist is the star: mostly ON (accumulation), with an
				// occasional flush so a fresh painting starts
				if rng.Float64() < 0.8 {
					c.Eval(`(function(){var e=document.getElementById('persist-trail');if(e&&!e.checked){e.checked=true;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`)
				} else {
					c.Eval(`(function(){var e=document.getElementById('persist-trail');if(e&&e.checked){e.checked=false;e.dispatchEvent(new Event('change',{bubbles:true}));}})()`)
				}
			case 8: // backdrops hide the persist painting — mostly keep them OFF
				if rng.Float64() < 0.8 {
					c.Eval(`(function(){['bg-spectro','bg-xy'].forEach(function(id){var e=document.getElementById(id);if(e&&e.checked){e.checked=false;e.dispatchEvent(new Event('change',{bubbles:true}));}});})()`)
				} else {
					toggle([]string{"bg-spectro", "bg-xy"}[rng.Intn(2)])
				}
			default:
				toggle([]string{"ring-sw", "use-points", "gradient-reverse", "spectro-skin", "spect-fill"}[rng.Intn(5)])
			}
		}
		step++
		// Twice per performance: hard-route the POSITION mod group (X/Y/zoom)
		// at dance level so the model demonstrably moves to the music.
		if elapsed := time.Since(start); (posRouted == 0 && elapsed > 40*time.Second) || (posRouted == 1 && elapsed > 150*time.Second) {
			posRouted++
			c.Eval(`(function(){
			  var secs=[].slice.call(document.querySelectorAll('.modules > .sect'));
			  var pos=secs.filter(function(s){var h=s.querySelector('.sect-hdr');return h&&h.textContent.trim()==='Position';})[0];
			  if(!pos)return; var mm=pos.nextElementSibling;
			  while(mm&&!(mm.classList&&mm.classList.contains('modmodule')))mm=mm.nextElementSibling;
			  if(!mm)return;
			  [].forEach.call(mm.querySelectorAll('.punit-mod'),function(card,i){
			    var sel=card.querySelector('select'), lv=card.querySelector('input[type=range]');
			    if(sel&&sel.options.length>1){sel.selectedIndex=1+(i%(sel.options.length-1));sel.dispatchEvent(new Event('change',{bubbles:true}));}
			    if(lv){var mn=parseFloat(lv.min),mx=parseFloat(lv.max);lv.value=String(mn+(mx-mn)*0.63);lv.dispatchEvent(new Event('input',{bubbles:true}));}
			  });
			})()`)
			fmt.Println("  position group routed to audio")
		}
		// Supervisor: closed-loop screen feedback every ~5s — the monkeys must
		// keep something ALIVE on screen. Blank/collapsed → calm the
		// modulation and restore the mode's param defaults; static → kick it.
		if step%150 == 0 {
			if img, err := c.Screenshot(); err == nil {
				b := img.Bounds()
				bright := cdp.BrightFrac(img, b)
				bb := cdp.BrightBBox(img, b)
				verdict := "ok"
				switch {
				case bright >= 0.0006 && bb.Dx() >= b.Dx()/14 && bb.Dx() < b.Dx()/6:
					verdict = "SMALL→refit"
					setSlider("camera-zoom", 0)
					setSlider("pan-x", 0)
					setSlider("pan-y", 0)
				case bright < 0.0006 || bb.Dx() < b.Dx()/14:
					verdict = "RECOVER"
					c.Eval(`(function(){
					  [].forEach.call(document.querySelectorAll('.punit-mod input[type=range]'),function(e){e.value=String((parseFloat(e.min)+parseFloat(e.max))/2);e.dispatchEvent(new Event('input',{bubbles:true}));});
					  [].forEach.call(document.querySelectorAll('#params input[type=range]'),function(e){e.value=e.defaultValue;e.dispatchEvent(new Event('input',{bubbles:true}));});
					  ['pan-x','pan-y','camera-zoom'].forEach(function(id){var e=document.getElementById(id);if(e){e.value='0';e.dispatchEvent(new Event('input',{bubbles:true}));}});
					})()`)
				case lastSuper != nil && cdp.DiffFrac(img, lastSuper, 40) < 0.001:
					verdict = "STATIC→tour hop"
					setSel("mode-select", tour[tourIdx%len(tour)])
					tourIdx++
					lastHop = time.Now()
				}
				fmt.Printf("  super @%d bright=%.3f%% bbox=%d %s\n", step, bright*100, bb.Dx(), verdict)
				lastSuper = img
			}
		}
		time.Sleep(time.Duration(150/nStreams) * time.Millisecond)
	}
	c.Eval(`location.hash='#lorenz'`)
	_, _ = c.Call("Page.reload", map[string]any{"ignoreCache": true})
	fmt.Printf("performance done: %d actions\n", step)
}
