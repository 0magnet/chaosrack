// Subcommand shots: capture the README gallery from a live tab — a hero shot
// (panel docked bottom, model raised above it) plus one contact-sheet image
// per attractor model sweeping the Colors module's two knobs: gradient SOURCE
// (X / Y / Z / trail) × PALETTE (mono / 2-color / 3-color / rainbow). Mono
// ignores the source, so its row holds the single unique frame (13 unique
// combos per model, arrayed on a 4×4 grid).
//
// Deterministic by construction (pinned pose, auto-rotate off, per-model crop
// from the first cell's bright bbox) so the gallery can be regenerated any
// time with:
//
//	uitool shots            # writes docs/img/*.jpg
//	uitool shots -models lorenz,thomas
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	shotDirOut = flag.String("dir", "docs/img", "output directory for gallery images")
	shotModels = flag.String("models", "", "comma-separated model filter (default: all attractors)")
	shotHero   = flag.Bool("hero", true, "capture the hero shot")
	cellPx     = flag.Int("cell", 320, "contact-sheet cell size in px")
)

// shotModelList is every model the gallery covers — DERIVED from the mode
// registry (all flows + parametric curves), so a newly added mode can't be
// silently missing from the gallery. "custom" is skipped: its default state
// duplicates Lorenz.
var shotModelList = func() []string {
	keys := attractor.ModeKeys(attractor.ClassFlow3D, attractor.ClassFlow4D, attractor.ClassParametric)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "custom" {
			out = append(out, k)
		}
	}
	return out
}()

func runShots() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shots:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*shotDirOut, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "shots:", err)
		os.Exit(1)
	}
	// Normalize persisted layout so the framing is reproducible.
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)

	if *shotHero {
		captureHero(c)
	}

	models := shotModelList
	if *shotModels != "" {
		models = strings.Split(*shotModels, ",")
	}
	for _, m := range models {
		captureSheet(c, m)
	}

	// Leave the tab in a clean state.
	c.Eval(`location.hash='#lorenz'`)
	c.Reload(2 * time.Second)
	fmt.Println("shots: done →", *shotDirOut)
}

// captureHero: panel docked to the bottom with the model raised above it
// (pan-y lifts the model clear of the panel).
func captureHero(c *cdp.Client) {
	c.Eval(`location.hash='#lorenz&ar=0&rot=25,40,0&py=2&z=-18'`)
	c.Reload(3500 * time.Millisecond)
	img, err := c.Screenshot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "shots: hero:", err)
		return
	}
	saveJPEG(filepath.Join(*shotDirOut, "hero.jpg"), img, 90)
	fmt.Println("  hero.jpg")
}

// captureSheet sweeps SRC × PALETTE for one model and writes its 4×4 grid.
// Grid rows top→bottom: mono (single unique frame), 2-color, 3-color,
// rainbow; columns left→right: source X, Y, Z, trail.
func captureSheet(c *cdp.Client, mode string) {
	c.Eval(fmt.Sprintf(`location.hash='#%s&ar=0&rot=25,40,0'`, mode))
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
		time.Sleep(450 * time.Millisecond)
	}

	cell := *cellPx
	grid := image.NewRGBA(image.Rect(0, 0, 4*cell+5*8, 4*cell+5*8))
	bg := color.RGBA{11, 14, 18, 255}
	draw.Draw(grid, grid.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	var crop image.Rectangle
	place := func(row, col int, img image.Image) {
		if crop.Empty() {
			crop = sheetCrop(img)
		}
		cellImg := scaleBox(cropTo(img, crop), cell, cell)
		x0 := 8 + col*(cell+8)
		y0 := 8 + row*(cell+8)
		draw.Draw(grid, image.Rect(x0, y0, x0+cell, y0+cell), cellImg, cellImg.Bounds().Min, draw.Src)
	}

	// Row 0: mono — the source knob has no effect, one unique frame (col 0).
	setKnobs(1, 2)
	if img, err := c.Screenshot(); err == nil {
		place(0, 0, img)
	}
	// Rows 1..3: 2-color / 3-color / rainbow × source X/Y/Z/trail.
	for ri, gc := range []int{2, 3, 4} {
		for gs := 0; gs < 4; gs++ {
			setKnobs(gc, gs)
			if img, err := c.Screenshot(); err == nil {
				place(ri+1, gs, img)
			}
		}
	}

	// Restore defaults + panel for the next model.
	setKnobs(2, 2)
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='';})()`)

	saveJPEG(filepath.Join(*shotDirOut, mode+".jpg"), grid, 87)
	fmt.Println(" ", mode+".jpg")
}

// sheetCrop picks the model's framing: the bright bbox padded 12%, squared,
// clamped — computed once per model so every cell aligns.
func sheetCrop(img image.Image) image.Rectangle {
	b := img.Bounds()
	bb := cdp.BrightBBox(img, b)
	if bb.Empty() {
		bb = b
	}
	pad := bb.Dx() / 8
	if p := bb.Dy() / 8; p > pad {
		pad = p
	}
	bb = image.Rect(bb.Min.X-pad, bb.Min.Y-pad, bb.Max.X+pad, bb.Max.Y+pad)
	// square it around its center
	cx, cy := (bb.Min.X+bb.Max.X)/2, (bb.Min.Y+bb.Max.Y)/2
	side := bb.Dx()
	if bb.Dy() > side {
		side = bb.Dy()
	}
	bb = image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2)
	return bb.Intersect(b)
}

func cropTo(img image.Image, r image.Rectangle) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

// scaleBox is a simple box-average downscaler (stdlib image has none, and
// x/image isn't vendored). Fine for photographic downscale.
func scaleBox(src image.Image, w, h int) *image.RGBA {
	sb := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy0 := sb.Min.Y + y*sb.Dy()/h
		sy1 := sb.Min.Y + (y+1)*sb.Dy()/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < w; x++ {
			sx0 := sb.Min.X + x*sb.Dx()/w
			sx1 := sb.Min.X + (x+1)*sb.Dx()/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, b, n uint32
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					pr, pg, pb, _ := src.At(xx, yy).RGBA()
					r += pr >> 8
					g += pg >> 8
					b += pb >> 8
					n++
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
		}
	}
	return out
}

func saveJPEG(path string, img image.Image, q int) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shots:", err)
		return
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: q}); err != nil {
		fmt.Fprintln(os.Stderr, "shots:", err)
	}
	_ = f.Close()
}
