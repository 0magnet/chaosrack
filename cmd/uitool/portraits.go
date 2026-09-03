// Subcommand portraits: one still and one short loop per model, for the
// README's model reference — every entry of every category of the mode
// selector, not just the attractors the contact sheets cover.
//
//	uitool portraits                       # docs/img/model/<key>.{jpg,gif}
//	uitool portraits -pm turtle,globe      # just these
//	uitool portraits -pgif=false           # stills only
//
// The list comes from attractor.Catalog(), so a model added to the registry
// gets an image here and a section in the README without either being edited.
//
// These are deliberately small — 57 models, and the gallery GIFs next door are
// already 8-11 MB each. A portrait is one pose turning for a second and a half,
// not the whole colors walk, so it lands around 200 KB.
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
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/gifenc"
)

var (
	portDir    = flag.String("pdir", "docs/img/model", "output directory for the per-model portraits")
	portModels = flag.String("pm", "", "comma-separated model filter (default: every model in the catalog)")
	portStillP = flag.Int("pstill", 520, "still size in px")
	portGifP   = flag.Int("pgif-px", 260, "GIF frame size in px")
	portFrames = flag.Int("pframes", 18, "frames per portrait GIF")
	portWantGi = flag.Bool("pgif", true, "also write the loop GIF")
	portSettle = flag.Duration("psettle", 3200*time.Millisecond, "time to let a model draw before capturing")
	portWSURL  = flag.String("pws", "", "websocket audio feed for the audio models, e.g. ws://127.0.0.1:8093/ws (default: the built-in signal generator)")
	portFill   = flag.Duration("pfill", 90*time.Second, "how long to wait for the spectrogram to fill the plane")
	portResume = flag.Bool("presume", false, "skip models whose still already exists")
	portColors = flag.Bool("pcolors", true, "walk the palette knob's four positions instead of shooting the default coloring")
	portExtras = flag.Bool("pextras", false, "capture ONLY the per-model Parameters shots and named variants, leaving the stills and loops alone")
)

// colorVariation is one setting of the Colors module's two knobs: gc is the
// PALETTE ring (1 mono, 2 two-color, 3 three-color, 4 rainbow) and gs is the
// gradient SOURCE ring (0 X, 1 Y, 2 Z, 3 trail).
type colorVariation struct{ gc, gs int }

// portraitVariations is what a portrait walks: all four positions of the
// palette ring, each on a different position of the source ring, so one loop
// shows both knobs rather than one model's default coloring fifty-seven times.
// Mono is last because it ignores the source entirely.
//
// The starting rotation follows the model's index, so consecutive entries in
// the README are not all photographed in the same color for their still.
func portraitVariations(idx int) []colorVariation {
	all := []colorVariation{{4, 3}, {3, 2}, {2, 0}, {1, 2}}
	out := make([]colorVariation, len(all))
	for i := range all {
		out[i] = all[(i+idx)%len(all)]
	}
	return out
}

func runPortraits() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "portraits:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*portDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "portraits:", err)
		os.Exit(1)
	}
	// Normalize persisted layout so framing does not depend on how the tab was
	// left — a hidden module or a docked panel changes the viewport.
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)

	models := attractor.CatalogKeys()
	if *portModels != "" {
		models = strings.Split(*portModels, ",")
	}
	var blank []string
	for i, m := range models {
		if *portResume && fileExists(filepath.Join(*portDir, m+".jpg")) {
			fmt.Printf("[%2d/%2d] %s — have it\n", i+1, len(models), m)
			continue
		}
		fmt.Printf("[%2d/%2d] %s", i+1, len(models), m)
		if !capturePortrait(c, m, i) {
			blank = append(blank, m)
		}
	}
	if len(blank) > 0 {
		// Named rather than silently skipped: a model that draws nothing here
		// draws nothing for a reader either, and that is worth knowing.
		fmt.Println("portraits: nothing bright to frame, captured whole viewport:", strings.Join(blank, " "))
	}
	c.Eval(`location.hash='#lorenz'`)
	c.Reload(2 * time.Second)
	fmt.Println("portraits: done →", *portDir)
}

// capturePortrait writes <key>.jpg and <key>.gif. It reports whether the model
// gave a bright region to frame; when it does not, the whole viewport is used.
func capturePortrait(c *cdp.Client, mode string, idx int) bool {
	audio := needsAudio[mode]
	if audio && *portWSURL != "" {
		// Face-on and still for the audio displays: a spectrogram read at an
		// angle is a worse picture than one read flat. The websocket backend
		// is chosen by a query parameter, so this needs a real navigation.
		gotoModel(c, "?audio=ws&wsurl="+*portWSURL, mode+"&ar=0&rot=0,0,0")
		waitForPanel(c)
	} else {
		// A pinned start pose, then the default auto-rotate turns it — flat
		// scope modes (pong, the text banner, the bouncing ball) boot face-on
		// and still regardless, and animate themselves. The empty query drops
		// any ?audio= an earlier audio model left behind: a visual model has
		// no use for a websocket, and leaving one connected keeps a capture
		// running for the rest of the sweep.
		gotoModel(c, "", mode+"&rot=20,0,0")
	}
	if *portExtras {
		// The stills and loops are expensive and depend on conditions that may
		// not be repeatable — the audio models were shot against live music.
		// This path refreshes only what a panel change invalidates.
		time.Sleep(700 * time.Millisecond)
		got := captureParams(c, mode)
		captureVariants(c, mode)
		if got {
			fmt.Print("  params")
		}
		fmt.Println()
		return true
	}
	hideChrome(c)
	if audio {
		if *portWSURL == "" {
			feedSilentAudio(c)
			// Turning the generator on reveals the oscillator modules, and
			// the panel puts itself back on screen to show them — so hide it
			// again, after the source is running rather than before.
			hideChrome(c)
		}
		waitForFill(c, mode)
	}

	crop, framed := portraitCrop(c)

	// The audio displays get their color from the spectrogram's own scheme,
	// not from the gradient rings, so there is nothing to sweep there.
	vars := []colorVariation{{0, 0}}
	sweep := *portColors && !audio
	if sweep {
		vars = portraitVariations(idx)
	}

	frames := make([]*image.RGBA, 0, *portFrames)
	var still *image.RGBA
	bestLit := -1
	per := *portFrames / len(vars)
	if per < 1 {
		per = 1
	}
	for vi, v := range vars {
		if sweep {
			setColors(c, v.gc, v.gs)
			time.Sleep(450 * time.Millisecond)
		}
		for i := 0; i < per; i++ {
			img, err := c.Screenshot()
			if err != nil {
				continue
			}
			cell := cropTo(img, crop)
			frames = append(frames, scaleBox(cell, *portGifP, *portGifP))
			// The still comes from the FIRST variation only — which rotates
			// per model — so the stills down the page are not all one color,
			// and so the still is never a frame caught mid-change.
			if lit := litPixels(img, crop); vi == 0 && lit > bestLit {
				bestLit = lit
				still = scaleBox(cell, *portStillP, *portStillP)
			}
		}
	}
	if sweep {
		setColors(c, 2, 2) // back to the defaults for whatever comes next
	}
	if audio {
		silenceAudio(c)
	}
	showChrome(c)
	// With the panel back on screen, photograph this model's own Parameters
	// module — which is a different panel for every system.
	time.Sleep(500 * time.Millisecond)
	captureParams(c, mode)
	captureVariants(c, mode)

	if still != nil {
		saveJPEG(filepath.Join(*portDir, mode+".jpg"), still, 86)
	}
	if *portWantGi && len(frames) > 1 {
		pal := gifenc.AdaptivePalette(frames)
		memo := make(map[uint32]uint8, 4096)
		pf := make([]*image.Paletted, len(frames))
		for i, f := range frames {
			pf[i] = gifenc.Palettize(f, pal, memo)
		}
		writeGif(filepath.Join(*portDir, mode+".gif"), pf)
	}
	fmt.Printf("  (%d frames)\n", len(frames))
	return framed
}

// portraitCrop unions the bright bbox over a few frames so a turning model
// stays inside the crop, pads, squares and clamps it. Falls back to the
// largest centered square of the viewport for modes with nothing bright to
// find — the spectrogram plane and the scope screens fill the frame anyway.
func portraitCrop(c *cdp.Client) (image.Rectangle, bool) {
	full := image.Rect(0, 0, 1280, 800)
	var bb image.Rectangle
	for i := 0; i < 6; i++ {
		img, err := c.Screenshot()
		if err != nil {
			continue
		}
		full = img.Bounds()
		if b := cdp.BrightBBox(img, full); !b.Empty() {
			if bb.Empty() {
				bb = b
			} else {
				bb = bb.Union(b)
			}
		}
		time.Sleep(350 * time.Millisecond)
	}
	if bb.Empty() {
		return centerSquare(full), false
	}
	pad := bb.Dx() / 7
	if p := bb.Dy() / 7; p > pad {
		pad = p
	}
	bb = image.Rect(bb.Min.X-pad, bb.Min.Y-pad, bb.Max.X+pad, bb.Max.Y+pad)
	cx, cy := (bb.Min.X+bb.Max.X)/2, (bb.Min.Y+bb.Max.Y)/2
	side := bb.Dx()
	if bb.Dy() > side {
		side = bb.Dy()
	}
	// A crop wider than the viewport would letterbox the model; cap it there.
	if side > full.Dy() {
		side = full.Dy()
	}
	if side > full.Dx() {
		side = full.Dx()
	}
	r := image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2)
	// Slide the square back inside the viewport rather than intersecting it,
	// which would leave a non-square crop and stretch the model.
	if r.Min.X < full.Min.X {
		r = r.Add(image.Pt(full.Min.X-r.Min.X, 0))
	}
	if r.Max.X > full.Max.X {
		r = r.Add(image.Pt(full.Max.X-r.Max.X, 0))
	}
	if r.Min.Y < full.Min.Y {
		r = r.Add(image.Pt(0, full.Min.Y-r.Min.Y))
	}
	if r.Max.Y > full.Max.Y {
		r = r.Add(image.Pt(0, full.Max.Y-r.Max.Y))
	}
	return r, true
}

func centerSquare(b image.Rectangle) image.Rectangle {
	side := b.Dy()
	if b.Dx() < side {
		side = b.Dx()
	}
	cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	return image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2)
}

// litPixels counts pixels above the background inside the crop — how much of
// the figure has been drawn.
func litPixels(img image.Image, r image.Rectangle) int {
	r = r.Intersect(img.Bounds())
	n := 0
	// Every fourth pixel: this only ranks frames against each other.
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if cr>>8+cg>>8+cb>>8 > 120 {
				n++
			}
		}
	}
	return n
}

// needsAudio: the models that draw nothing without a signal — taken from the
// selector's own Audio category rather than from ModeClass.
//
// The two are not the same thing and the difference is not academic: the
// Takens embedding is registered ClassParametric, because what it does with
// the audio is trace it into the trail like any other parametric curve. Keyed
// on the class, it silently photographed a black frame.
var needsAudio = func() map[string]bool {
	m := map[string]bool{}
	for _, g := range attractor.Catalog() {
		if g.Label != "Audio" {
			continue
		}
		for _, mo := range g.Models {
			m[mo.Key] = true
		}
	}
	return m
}()

// feedSilentAudio gives the audio models something to display. With no source
// the spectrogram, the scope and the delay embedding are all a black frame —
// truthful, and useless as documentation.
//
// The built-in signal generator is the source that needs neither the server
// nor the microphone. Its per-oscillator OUT ring has an "off" position that
// mutes that oscillator, and with all three off no Web Audio graph reaches the
// speakers at all — the analysis paths (scope, spectrogram, meters) are fed
// from the generator's samples directly. So this fills the display without
// playing anything.
func feedSilentAudio(c *cdp.Client) {
	c.Eval(`(function(){
	  ['gen-x-out','gen-y-out','gen-z-out'].forEach(function(id){
	    var s=document.getElementById(id);
	    if(s && s.value!=='0'){ s.value='0'; s.dispatchEvent(new Event('change',{bubbles:true})); }
	  });
	  var fg=document.getElementById('fg-on');
	  if(fg && !fg.checked){ fg.checked=true; fg.dispatchEvent(new Event('change',{bubbles:true})); }
	})()`)
	// The browser's autoplay policy keeps the AudioContext suspended until a
	// real user gesture, and a suspended context delivers no samples — which
	// is why enabling the generator alone still photographed a black plane.
	// CDP-dispatched input is trusted, so one click unlocks it. The click
	// lands on the canvas, where a press and release at the same point is a
	// zero-degree rotate, and the OUT rings above are already off, so nothing
	// reaches the speakers.
	c.Click(120, 120)
	// The spectrogram scrolls one column at a time; give it long enough to
	// cover the plane rather than photographing a sliver.
	time.Sleep(5500 * time.Millisecond)
}

// gotoModel puts the tab on one model, in EXACTLY ONE navigation.
//
// That is the whole point of the function. The obvious spelling — assign
// location.href, then reload to make sure the new hash is read at boot —
// starts the wasm and then navigates out from under it, and the Go side comes
// up far enough to look for its canvas in a document that is already being
// torn down. It alerts "cannot find #gocanvas, exiting" and the tab is left
// sitting on a modal dialog, which blocks every later evaluation: one stray
// reload stalled a whole capture run.
//
// So: when the path and query are already right, only the hash needs to
// change and a plain reload picks it up. When they are not, replace() does
// the whole thing at once — the new hash included.
func gotoModel(c *cdp.Client, query, hash string) {
	sameDoc, _ := c.Eval(fmt.Sprintf(`(function(){
	  var q = %q, h = '#' + %q;
	  if (location.pathname === '/' && location.search === q) { location.hash = h; return true; }
	  location.replace(location.origin + '/' + q + h);
	  return false;
	})()`, query, hash)).(bool)
	if sameDoc {
		// Only the hash moved, so the document is untouched and the app —
		// which reads its mode at boot — is still showing the previous model
		// until this reload.
		c.Reload(*portSettle)
		return
	}
	// replace() has already started the load; waiting is all that is left.
	time.Sleep(*portSettle)
}

// waitForPanel holds until the wasm has built the control panel. A navigation
// returns a page whose DOM the Go side has not populated yet, and hiding the
// furniture before it exists hides nothing — which is how the first live
// spectrogram capture came back with the whole rack in shot.
// It waits for a MODULE, not for the shell. The shell exists as soon as the
// wasm starts building the panel and the modules appear a moment later, so
// waiting on the shell returns too early — which silently cost the module
// capture its four mode-owned modules once the app grew slow enough to boot
// for the gap to matter.
func waitForPanel(c *cdp.Client) {
	for i := 0; i < 60; i++ {
		if n, ok := c.Eval(`document.querySelectorAll('.sect').length`).(float64); ok && n > 0 {
			// One more beat: the first module existing does not mean the last
			// one does.
			time.Sleep(600 * time.Millisecond)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// waitForFill holds until the spectrogram has scrolled all the way across the
// plane. The texture starts empty and one column arrives per hop, so a capture
// taken on the usual settle timer photographs a stripe of picture against a
// black plane — which is not what the mode looks like in use.
//
// What is watched is the WIDTH of the lit region, not its area: the picture
// grows rightward one column at a time and never shrinks, whereas its area
// rises and falls with every quiet passage in whatever is playing. Area was
// the first attempt and it never converged for exactly that reason.
func waitForFill(c *cdp.Client, mode string) {
	deadline := time.Now().Add(*portFill)
	widest, stable := 0, 0
	for time.Now().Before(deadline) {
		img, err := c.Screenshot()
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		w := cdp.BrightBBox(img, img.Bounds()).Dx()
		switch {
		case w > widest:
			widest = w
			stable = 0
		default:
			stable++
		}
		// Grown to most of the viewport and not grown further for a few
		// samples: the scroll has reached the far edge.
		if stable >= 3 && widest > img.Bounds().Dx()*3/5 {
			return
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Printf(" [%s: still filling after %s]", mode, *portFill)
}

// silenceAudio puts the generator back, so a capture run does not leave the
// tab producing a signal.
func silenceAudio(c *cdp.Client) {
	c.Eval(`(function(){var fg=document.getElementById('fg-on');
	  if(fg && fg.checked){ fg.checked=false; fg.dispatchEvent(new Event('change',{bubbles:true})); }})()`)
}

// hideChrome / showChrome leave the canvas and hide every other child of the
// body — the panel shell, the dock handle, the wasm switch, the overlays.
//
// Named-element hiding is what the older gallery captures do, and it rots:
// they still hide #controls-panel, which the panel-shell refactor replaced, so
// the dock button stayed on screen and dragged the bright bbox off-center.
// Whatever the furniture is called, it is not the canvas.
//
// The resize event afterwards matters: with the panel docked the renderer has
// centered the model in the space left over, and hiding the panel does not by
// itself tell it the space is back.
func hideChrome(c *cdp.Client) {
	c.Eval(`(function(){
	  // Each element's original display is recorded ONCE. Hiding twice — which
	  // the audio models do, because turning the generator on brings the panel
	  // back — would otherwise record 'none' as the original and leave the
	  // interface hidden for good. Re-hiding still happens; only the record
	  // of what to put back is kept from the first pass.
	  if (!window.__uitoolHidden) { window.__uitoolHidden = []; }
	  var known = window.__uitoolHidden.map(function(p){ return p[0]; });
	  [].forEach.call(document.body.children, function(e){
	    if (e.id === 'gocanvas-container' || e.tagName === 'SCRIPT') return;
	    if (known.indexOf(e) < 0) { window.__uitoolHidden.push([e, e.style.display]); }
	    e.style.display = 'none';
	  });
	  window.dispatchEvent(new Event('resize'));
	})()`)
	time.Sleep(700 * time.Millisecond)
}

func showChrome(c *cdp.Client) {
	c.Eval(`(function(){
	  (window.__uitoolHidden || []).forEach(function(p){ p[0].style.display = p[1]; });
	  window.__uitoolHidden = null;
	  window.dispatchEvent(new Event('resize'));
	})()`)
}

// setColors turns the Colors module's two rings. The hidden <select>s behind
// the knobs are what the app listens to, which is also how the contact-sheet
// and gallery captures drive them.
func setColors(c *cdp.Client, gc, gs int) {
	c.Eval(fmt.Sprintf(`(function(){
	  var s=document.getElementById('gradient-source');
	  if(s){ s.value='%d'; s.dispatchEvent(new Event('change',{bubbles:true})); }
	  var g=document.getElementById('gradient-colors');
	  if(g){ g.value='%d'; g.dispatchEvent(new Event('change',{bubbles:true})); }
	})()`, gs, gc))
}

// captureParams photographs the Parameters module as it stands for one model.
//
// The module is not one thing: its knobs are the current system's, so the
// Lorenz entry's σ/ρ/β and the turtle's mod/seq/dim are different panels
// under the same header. A model reference that shows the figure without the
// controls that shape it is half an entry.
//
// Returns false when the model has no parameters of its own — the polyhedra
// and most of the geometry — in which case there is nothing to photograph and
// the README simply has no column for it.
func captureParams(c *cdp.Client, mode string) bool {
	// The Parameters module carries no id, so it is found by its header.
	raw, _ := c.Eval(`(function(){
	  var s = [].filter.call(document.querySelectorAll('.sect'), function(e){
	    var h = e.querySelector('.sect-hdr');
	    return h && h.textContent.trim() === 'Parameters';
	  })[0];
	  if (!s) return '';
	  s.scrollIntoView({block:'center', inline:'center'});
	  var r = s.getBoundingClientRect();
	  // A module with a header and nothing under it is an empty module.
	  var knobs = s.querySelectorAll('.knob-ring, .knob-inner, input.numin, .led').length;
	  return JSON.stringify({x:r.left, y:r.top, w:r.width, h:r.height,
	    vw: window.innerWidth, knobs: knobs});
	})()`).(string)
	if raw == "" {
		return false
	}
	var box struct {
		X, Y, W, H, VW float64
		Knobs          int
	}
	if err := json.Unmarshal([]byte(raw), &box); err != nil || box.W < 8 || box.Knobs == 0 {
		return false
	}
	time.Sleep(350 * time.Millisecond) // let the scroll settle before measuring for real
	img, err := c.Screenshot()
	if err != nil {
		return false
	}
	b := img.Bounds()
	scale := 1.0
	if box.VW > 0 {
		scale = float64(b.Dx()) / box.VW
	}
	const pad = 6
	r := image.Rect(
		int((box.X-pad)*scale), int((box.Y-pad)*scale),
		int((box.X+box.W+pad)*scale), int((box.Y+box.H+pad)*scale),
	).Intersect(b)
	if r.Dx() < 8 || r.Dy() < 8 {
		return false
	}
	saveJPEG(filepath.Join(*portDir, mode+"-params.jpg"), cropTo(img, r), 88)
	return true
}

// modelVariantShots are extra faces of a model, captured as <key>-<suffix>.jpg
// and shown as their own column in the README.
//
// The turtle earns this: the same integer sequence walked in two dimensions
// and in three are not the same figure, and a reference that shows only the
// 3-D one is hiding half of what the mode does. `set` is run against the
// panel before the shot, the same way the color rings are driven.
type variantShot struct {
	suffix, set, reset string
	settle             time.Duration
	// colors, when set, turns the gradient rings to something the variant can
	// actually use before the shot.
	colors *colorVariation
}

var modelVariantShots = map[string][]variantShot{
	// The modulus matters as much as the dimension. Classifying the Fibonacci
	// walk with pisano.Classify for m = 2..40 says the default 25 is one of
	// the OPEN ones — it drifts, and photographed as a diagonal streak. 30
	// closes after two passes with a half turn: 120 terms of figure, which is
	// what a 2-D Pisano path is worth showing as. The column head says which
	// modulus, so it is not mistaken for the same walk as the 3-D still.
	"turtle": {{
		suffix: "2d",
		set:    setTurtleDim(2) + setTurtleMod(30),
		reset:  setTurtleDim(3) + setTurtleMod(25),
		// Changing the dimension restarts the walk, and the walk extrudes at
		// the Speed knob's rate rather than appearing all at once. At 2.5 s
		// this photographed a stub of a figure off in one corner.
		settle: 9 * time.Second,
		// A flat figure has no Z for the color to follow, which is what the
		// mode's own documentation says to do about it: put the gradient on
		// the trail instead. Left on Z the 2-D walk came out nearly black.
		colors: &colorVariation{gc: 4, gs: 3},
	}},
}

func setTurtleDim(d int) string {
	return fmt.Sprintf(`(function(){var e=document.getElementById('turtle-dim');
	  if(e){e.value='%d'; e.dispatchEvent(new Event('input',{bubbles:true}));
	  e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, d)
}

// captureVariants photographs each of a model's named variants.
func captureVariants(c *cdp.Client, mode string) {
	for _, v := range modelVariantShots[mode] {
		c.Eval(v.set)
		if v.colors != nil {
			setColors(c, v.colors.gc, v.colors.gs)
		}
		settle := v.settle
		if settle <= 0 {
			settle = 3 * time.Second
		}
		time.Sleep(settle)
		// Twice: turning a parameter knob rebuilds the panel, which replaces
		// the elements the first pass hid, and the second call catches the
		// new ones. Hidden once, the variant shot came back with half the
		// rack in it.
		hideChrome(c)
		hideChrome(c)
		// The crop is computed AFTER the settle and the hide, so it frames the
		// figure the variant actually draws rather than the stub it starts as
		// — or the panel.
		crop, _ := portraitCrop(c)
		var best *image.RGBA
		bestLit := -1
		for i := 0; i < 6; i++ {
			img, err := c.Screenshot()
			if err != nil {
				continue
			}
			if lit := litPixels(img, crop); lit > bestLit {
				bestLit = lit
				best = scaleBox(cropTo(img, crop), *portStillP, *portStillP)
			}
			time.Sleep(250 * time.Millisecond)
		}
		showChrome(c)
		if best != nil {
			saveJPEG(filepath.Join(*portDir, mode+"-"+v.suffix+".jpg"), best, 86)
			fmt.Printf("  +%s", v.suffix)
		}
		// Put it back. A variant that leaves the control where it set it is a
		// variant that changes what the NEXT capture of this model shows —
		// and the panel persists, so it would change what the app shows too.
		if v.reset != "" {
			c.Eval(v.reset)
		}
		if v.colors != nil {
			setColors(c, 2, 2)
		}
	}
}

func setTurtleMod(m int) string {
	return fmt.Sprintf(`;(function(){var e=document.getElementById('turtle-mod');
	  if(e){e.value='%d'; e.dispatchEvent(new Event('input',{bubbles:true}));
	  e.dispatchEvent(new Event('change',{bubbles:true}));}})()`, m)
}
