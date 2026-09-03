// Subcommand gifs: capture the animated README gallery — one GIF per primary
// model, rotating (pinned start pose + auto-rotate, for depth) while walking
// the same 13 SRC × PALETTE variations as the static contact sheets. Each
// variation plays twice over: first with the flowing trail (persist OFF — you
// can see which way the colors run), then with Persist ON so the accumulation
// fills the figure. A master reel (reel.gif) sequences every model's
// rainbow-persist segment.
//
//	uitool gifs                  # writes docs/img/gif/*.gif (16 models + reel)
//	uitool gifs -gif-models lorenz,thomas
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/gifenc"
)

var (
	gifDir    = flag.String("gif-dir", "docs/img/gif", "output directory for animated gallery")
	gifModels = flag.String("gif-models", "", "comma-separated model filter (default: the 16 primary models)")
	gifPx     = flag.Int("gif-px", 560, "GIF frame size in px")
)

// gifModelList: the primary models (the Sprott catalog stays static — 19 more
// GIFs would put the repo well past a sensible size).
var gifModelList = []string{
	"lorenz", "rossler", "chua", "aizawa", "sprott", "thomas", "halvorsen",
	"chen", "dadras", "rabinovich", "burkeshaw", "lu", "newtonleipnik",
	"hyperrossler", "lissajou", "graphicartist",
}

const (
	gifFlowFrames    = 5  // frames per variation with persist off (flow visible)
	gifPersistFrames = 7  // frames per variation with persist on (accumulation)
	gifDelay         = 12 // GIF frame delay in 1/100s (~8 fps playback)
)

func runGifs() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gifs:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*gifDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "gifs:", err)
		os.Exit(1)
	}
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)

	models := gifModelList
	if *gifModels != "" {
		models = strings.Split(*gifModels, ",")
	}
	var reel []*image.Paletted // rainbow-persist segment per model
	for _, m := range models {
		seg := captureModelGif(c, m)
		reel = append(reel, seg...)
	}
	// Only rewrite the reel on a FULL run — a filtered run would replace it
	// with just the filtered models' segments.
	if len(reel) > 0 && *gifModels == "" {
		writeGif(filepath.Join(*gifDir, "reel.gif"), reel)
		fmt.Println("  reel.gif")
	}
	c.Eval(`location.hash='#lorenz'`)
	c.Reload(2 * time.Second)
	fmt.Println("gifs: done →", *gifDir)
}

// captureModelGif records one model's variation walk and returns its
// rainbow-persist segment (paletted with the model's own palette) for the reel.
func captureModelGif(c *cdp.Client, mode string) []*image.Paletted {
	// Pinned start pose so framing is reproducible; auto-rotate (the default)
	// then spins it for depth.
	c.Eval(fmt.Sprintf(`location.hash='#%s&rot=20,0,0'`, mode))
	c.Reload(2800 * time.Millisecond)
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='none';
	  ['runtime'].forEach(function(id){var e=document.getElementById(id);if(e)e.style.display='none';});
	  [].forEach.call(document.querySelectorAll('button[title^="Show / hide controls"],a'),function(e){e.style.display='none';});})()`)
	time.Sleep(200 * time.Millisecond)

	setKnobs := func(gc, gs int) {
		c.Eval(fmt.Sprintf(`(function(){
		  var s=document.getElementById('gradient-source'); s.value='%d'; s.dispatchEvent(new Event('change',{bubbles:true}));
		  var g=document.getElementById('gradient-colors'); g.value='%d'; g.dispatchEvent(new Event('change',{bubbles:true}));
		})()`, gs, gc))
	}
	setPersist := func(on bool) {
		c.Eval(fmt.Sprintf(`(function(){var p=document.getElementById('persist-trail');if(p&&p.checked!=%v){p.checked=%v;p.dispatchEvent(new Event('change',{bubbles:true}));}})()`, on, on))
	}

	// Framing: union the bright bbox over half a rotation so the model stays
	// inside the crop while spinning, pad 15%, square, clamp.
	var crop image.Rectangle
	for i := 0; i < 8; i++ {
		img, err := c.Screenshot()
		if err != nil {
			continue
		}
		bb := cdp.BrightBBox(img, img.Bounds())
		if !bb.Empty() {
			if crop.Empty() {
				crop = bb
			} else {
				crop = crop.Union(bb)
			}
		}
		time.Sleep(450 * time.Millisecond)
	}
	if crop.Empty() {
		fmt.Fprintln(os.Stderr, "gifs:", mode, "blank — skipped")
		return nil
	}
	full := image.Rect(0, 0, 1<<30, 1<<30)
	{
		img, _ := c.Screenshot() //nolint:errcheck
		if img != nil {
			full = img.Bounds()
		}
	}
	pad := crop.Dx() / 7
	if p := crop.Dy() / 7; p > pad {
		pad = p
	}
	crop = image.Rect(crop.Min.X-pad, crop.Min.Y-pad, crop.Max.X+pad, crop.Max.Y+pad)
	cx, cy := (crop.Min.X+crop.Max.X)/2, (crop.Min.Y+crop.Max.Y)/2
	side := crop.Dx()
	if crop.Dy() > side {
		side = crop.Dy()
	}
	crop = image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2).Intersect(full)

	// The variation walk. Order matches the static sheets: mono once, then
	// 2-color / 3-color / rainbow across sources X, Y, Z, trail.
	type variation struct{ gc, gs int }
	vars := []variation{{1, 2}}
	for _, gc := range []int{2, 3, 4} {
		for gs := 0; gs < 4; gs++ {
			vars = append(vars, variation{gc, gs})
		}
	}

	px := *gifPx
	var frames []*image.RGBA
	reelStart, reelEnd := -1, -1
	for _, v := range vars {
		setKnobs(v.gc, v.gs)
		setPersist(false)
		time.Sleep(350 * time.Millisecond)
		for i := 0; i < gifFlowFrames; i++ {
			if img, err := c.Screenshot(); err == nil {
				frames = append(frames, scaleBox(cropTo(img, crop), px, px))
			}
		}
		setPersist(true)
		if v.gc == 4 && v.gs == 2 {
			reelStart = len(frames)
		}
		for i := 0; i < gifPersistFrames; i++ {
			if img, err := c.Screenshot(); err == nil {
				frames = append(frames, scaleBox(cropTo(img, crop), px, px))
			}
		}
		if v.gc == 4 && v.gs == 2 {
			reelEnd = len(frames)
		}
		setPersist(false)
	}
	setKnobs(2, 2)
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='';})()`)

	if len(frames) == 0 {
		return nil
	}
	pal := gifenc.AdaptivePalette(frames)
	memo := make(map[uint32]uint8, 4096)
	pf := make([]*image.Paletted, len(frames))
	for i, f := range frames {
		pf[i] = gifenc.Palettize(f, pal, memo)
	}
	writeGif(filepath.Join(*gifDir, mode+".gif"), pf)
	fmt.Println(" ", mode+".gif", fmt.Sprintf("(%d frames)", len(pf)))

	if reelStart >= 0 && reelEnd > reelStart {
		return pf[reelStart:reelEnd]
	}
	return nil
}

func writeGif(path string, frames []*image.Paletted) {
	f, err := os.Create(path) //nolint:gosec // the path is the file this command was told to read or write
	if err != nil {
		fmt.Fprintln(os.Stderr, "gifs:", err)
		return
	}
	if err := gifenc.Encode(f, frames, gifDelay); err != nil {
		fmt.Fprintln(os.Stderr, "gifs:", err)
	}
	_ = f.Close() //nolint:errcheck
}
