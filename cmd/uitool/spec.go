// Subcommand spec: render a WAV to a spectrogram PNG through chaosrack's own
// pipeline, and diff two such PNGs.
//
// This exists so that "does our spectrogram match audioprism's" is a question
// with an answer instead of an impression. The original renders a file
// straight to an image and never opens an audio device:
//
//	audioprism --dft-size 1024 --overlap 50 --orientation horizontal \
//	           --height 512 test.wav ref.png
//
// and this renders the same file through the same code the browser runs:
//
//	uitool spec -spec-wav test.wav -spec-out rack.png
//	uitool spec -spec-diff ref.png,rack.png
//
// Same input, same parameters, no live capture and no screenshots — the two
// pictures are then comparable pixel for pixel, and a drift in any constant
// (FFT size, hop, bin→row map, magnitude scale, color table) shows up as a
// number rather than as a feeling that one looks softer than the other.
//
// Compare against a FIXED build of the original — github.com/0magnet/audioprism
// — not against a release one. Upstream's file renderer packs each pixel as
// 0x00RRGGBB and hands the buffer to GraphicsMagick as "BGRA", four bytes per
// pixel with the always-zero top byte declared as alpha, then calls opacity(0)
// to undo it. What comes out is a channel rotating once per row: a heat render
// reduces to black, blue, green and red in near-equal quarters. Grayscale
// survives it, because all three of its channels carry the same byte, so a
// grayscale reference is the only usable one from an unfixed build.
//
// I compared a heat render here against a grayscale render there for a while on
// that reasoning, and every number it produced was meaningless — two different
// color maps disagree everywhere, and comparing their MEAN BRIGHTNESS makes it
// look like a magnitude problem, since blue (0,0,255) averages 85 and the gray
// of the same magnitude averages 51. The fork unpacks to explicit RGB, which
// makes all three schemes comparable, and this tool now counts exact colors
// rather than averaging channels.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sort"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	specWav     = flag.String("spec-wav", "", "WAV file to render (16-bit PCM)")
	specOut     = flag.String("spec-out", "spec.png", "PNG to write")
	specDiff    = flag.String("spec-diff", "", "two PNGs to compare instead, comma separated")
	specStats   = flag.String("spec-stats", "", "describe one PNG instead: brightness, columns, rows")
	specMagMax  = flag.Float64("spec-magmax", 0, "override the magnitude ceiling (both default to 45)")
	specShift   = flag.Int("spec-shift", 0, "shift the second image right by this many columns before diffing")
	specColors  = flag.String("spec-colors", "", "color scheme: heat, blue, grayscale")
	specWindow  = flag.String("spec-window", "", "window function: hann, hamming, bartlett, rectangular")
	specScale   = flag.String("spec-scale", "", "magnitude scale: logarithmic, linear")
	specMagMin  = flag.Float64("spec-magmin", math.Inf(-1), "magnitude floor")
	specDFTSize = flag.Int("spec-dft", 0, "DFT size (power of two)")
	specOverlap = flag.Float64("spec-overlap", 0, "samples overlap percentage (5-95), or a ratio (0.05-0.95)")
)

func runSpec() {
	if *specStats != "" {
		runSpecStats(*specStats)
		specTopColors(*specStats)
		return
	}
	if *specDiff != "" {
		runSpecDiff(*specDiff)
		return
	}
	if *specWav == "" {
		fmt.Fprintln(os.Stderr, "spec: -spec-wav is required (or -spec-diff a.png,b.png)")
		os.Exit(2)
	}
	// Every setting the original exposes, so a render here can be asked for
	// exactly what a render there was asked for.
	if *specMagMax > 0 {
		sg.S.SetMagMax(*specMagMax)
	}
	if !math.IsInf(*specMagMin, -1) {
		sg.S.SetMagMin(*specMagMin)
	}
	if *specColors != "" {
		sg.S.SetColorByName(*specColors)
	}
	if *specWindow != "" {
		sg.S.SetWindowByName(*specWindow)
	}
	if *specScale != "" {
		sg.S.SetScaleByName(*specScale)
	}
	if *specDFTSize > 0 {
		sg.S.SetDFTSize(*specDFTSize)
	}
	if *specOverlap > 0 {
		sg.S.SetOverlap(sg.OverlapFromFlag(*specOverlap))
	}
	samples, rate, err := readWAV(*specWav)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec:", err)
		os.Exit(2)
	}

	// The same framing the live path uses: a window of FFTSize advanced by the
	// step the shared settings define, which at 50% overlap is half a window.
	size := sg.S.GetDFTSize()
	step := sg.S.StepSize()
	if step <= 0 {
		step = size / 2
	}
	rows := attractor.SpectrogramRows(size)
	cols := 0
	if len(samples) >= size {
		cols = (len(samples)-size)/step + 1
	}
	if cols < 1 {
		fmt.Fprintf(os.Stderr, "spec: %s is shorter than one %d-sample window\n", *specWav, size)
		os.Exit(2)
	}

	img := image.NewRGBA(image.Rect(0, 0, cols, rows))
	frame := make([]float32, size)
	for x := 0; x < cols; x++ {
		copy(frame, samples[x*step:x*step+size])
		col := attractor.SpectrogramColumn(attractor.SpectrogramMags(frame), rows)
		if len(col) < rows*4 {
			break
		}
		for y := 0; y < rows; y++ {
			// 0 Hz is row 0 of the column and the BOTTOM of the picture, which
			// is how the original orients it too.
			img.Set(x, rows-1-y, color.RGBA{col[y*4], col[y*4+1], col[y*4+2], 255})
		}
	}
	savePNG(*specOut, img)
	fmt.Printf("spec: %s → %s (%d columns × %d rows, %d Hz, %d-point FFT, %d hop)\n",
		*specWav, *specOut, cols, rows, rate, size, step)
}

func runSpecDiff(spec string) {
	var a, b string
	if n, _ := fmt.Sscanf(spec, "%s", &a); n == 0 { //nolint:errcheck
		os.Exit(2)
	}
	for i := 0; i < len(spec); i++ {
		if spec[i] == ',' {
			a, b = spec[:i], spec[i+1:]
			break
		}
	}
	if b == "" {
		fmt.Fprintln(os.Stderr, "spec: -spec-diff wants two paths, comma separated")
		os.Exit(2)
	}
	ia, err := loadPNG(a)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec:", err)
		os.Exit(2)
	}
	ib, err := loadPNG(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec:", err)
		os.Exit(2)
	}
	ra, rb := ia.Bounds(), ib.Bounds()
	fmt.Printf("%s: %dx%d\n%s: %dx%d\n", a, ra.Dx(), ra.Dy(), b, rb.Dx(), rb.Dy())

	// Compare the overlapping region anchored TOP-LEFT, which is the one corner
	// that means the same thing in both layouts.
	//
	// This used to anchor bottom-left, on the reasoning that both put 0 Hz at
	// the bottom and the oldest column at the left. That holds for a horizontal
	// spectrogram, where the axis the two images differ along is time and time
	// runs left to right — and there it made no difference, because the heights
	// were equal and the anchor only matters on the axis that differs. Turned on
	// a VERTICAL spectrogram, where time runs down the y axis and the renderer
	// under test emitted one column fewer, it aligned the two by their last
	// column instead of their first and reported a whole picture of difference
	// where there was none. A shifted comparison does not announce itself; it
	// reads as a real disagreement, and I spent the effort chasing a renderer
	// bug that did not exist.
	//
	// Both layouts grow away from the top-left as audio is appended, so that
	// corner is the same moment of the same audio either way.
	w, h := min(ra.Dx(), rb.Dx()), min(ra.Dy(), rb.Dy())
	if w == 0 || h == 0 {
		fmt.Fprintln(os.Stderr, "spec: nothing overlaps")
		os.Exit(1)
	}
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		fmt.Printf("  (different sizes — comparing the %dx%d they share, from the top-left)\n", w, h)
	}
	// Mean brightness of each, reported alongside the difference. Two images
	// that are both nearly empty agree with each other perfectly and mean
	// nothing by it — a magnitude window shifted too far blacks the reference
	// out entirely and the diff collapses to "identical". The brightnesses make
	// that visible instead of letting it read as a pass.
	var lumA, lumB float64
	var contentA, contentB int
	var sum, worst float64
	var off int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bx := rb.Min.X + x - *specShift
			if bx < rb.Min.X || bx >= rb.Max.X {
				continue
			}
			ar, ag, ab, _ := ia.At(ra.Min.X+x, ra.Min.Y+y).RGBA()
			br, bg, bb, _ := ib.At(bx, rb.Min.Y+y).RGBA()
			d := (absInt(int(ar>>8)-int(br>>8)) +
				absInt(int(ag>>8)-int(bg>>8)) +
				absInt(int(ab>>8)-int(bb>>8))) / 3
			la := float64(int(ar>>8)+int(ag>>8)+int(ab>>8)) / 3
			lb := float64(int(br>>8)+int(bg>>8)+int(bb>>8)) / 3
			lumA += la
			lumB += lb
			if la > 8 {
				contentA++
			}
			if lb > 8 {
				contentB++
			}
			sum += float64(d)
			if float64(d) > worst {
				worst = float64(d)
			}
			if d > 24 {
				off++
			}
		}
	}
	n := float64(w * h)
	fmt.Printf("overlap %dx%d — mean brightness %.1f vs %.1f — mean channel difference %.2f/255, worst %.0f, %.2f%% of pixels differ by more than 24\n",
		w, h, lumA/n, lumB/n, sum/n, worst, 100*float64(off)/n)
	// Judged on how much of each picture is anything at all, not on how bright
	// it is on average. A clean tone on black is 98% empty and perfectly
	// meaningful; a reference whose magnitude window has been shifted past its
	// data is 100% empty and agrees with everything. The first must pass and the
	// second must not, so the test is content, not brightness.
	fmt.Printf("  content: %.2f%% vs %.2f%% of pixels are above black\n",
		100*float64(contentA)/n, 100*float64(contentB)/n)
	if float64(contentA)/n < 0.005 || float64(contentB)/n < 0.005 {
		fmt.Println("  (one of these is empty — the comparison says nothing)")
	}
}

// runSpecStats describes one render. ImageMagick's crop kept returning the
// whole image on these files — a page offset the PNGs carry — so every
// per-column number it gave was the same number, which is worse than no
// measurement because it looks like one. This reads the pixels directly.
func runSpecStats(path string) {
	img, err := loadPNG(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec:", err)
		os.Exit(2)
	}
	b := img.Bounds()
	lum := func(x, y int) float64 {
		r, g, bl, _ := img.At(x, y).RGBA()
		return float64(int(r>>8)+int(g>>8)+int(bl>>8)) / 3
	}
	var total float64
	hist := map[string]int{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := lum(x, y)
			total += v
			switch {
			case v < 8:
				hist["near black"]++
			case v < 64:
				hist["dark"]++
			case v < 160:
				hist["mid"]++
			default:
				hist["bright"]++
			}
		}
	}
	n := float64(b.Dx() * b.Dy())
	fmt.Printf("%s: %dx%d, mean brightness %.1f/255\n", path, b.Dx(), b.Dy(), total/n)
	fmt.Printf("  near black %.1f%%  dark %.1f%%  mid %.1f%%  bright %.1f%%\n",
		100*float64(hist["near black"])/n, 100*float64(hist["dark"])/n,
		100*float64(hist["mid"])/n, 100*float64(hist["bright"])/n)
	fmt.Print("  first 12 column means: ")
	for x := b.Min.X; x < b.Min.X+12 && x < b.Max.X; x++ {
		var c float64
		for y := b.Min.Y; y < b.Max.Y; y++ {
			c += lum(x, y)
		}
		fmt.Printf("%.0f ", c/float64(b.Dy()))
	}
	fmt.Println()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// readWAV reads a 16-bit PCM RIFF file into mono float32 in −1..1. Enough of
// the format to read what ffmpeg writes, and no more.
func readWAV(path string) ([]float32, int, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the file this command was told to read or write
	if err != nil {
		return nil, 0, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s: not a RIFF/WAVE file", path)
	}
	var channels, bits int
	var rate int
	var data []byte
	for p := 12; p+8 <= len(raw); {
		id := string(raw[p : p+4])
		size := int(binary.LittleEndian.Uint32(raw[p+4 : p+8]))
		body := p + 8
		if body+size > len(raw) {
			size = len(raw) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				channels = int(binary.LittleEndian.Uint16(raw[body+2 : body+4]))
				rate = int(binary.LittleEndian.Uint32(raw[body+4 : body+8]))
				bits = int(binary.LittleEndian.Uint16(raw[body+14 : body+16]))
			}
		case "data":
			data = raw[body : body+size]
		}
		p = body + size
		if size%2 == 1 {
			p++ // chunks are word aligned
		}
	}
	if bits != 16 || channels < 1 || data == nil {
		return nil, 0, fmt.Errorf("%s: want 16-bit PCM, got %d-bit / %d channels", path, bits, channels)
	}
	frames := len(data) / 2 / channels
	out := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var acc float64
		for c := 0; c < channels; c++ {
			s := int16(binary.LittleEndian.Uint16(data[(i*channels+c)*2:])) //nolint:gosec // PCM is signed
			acc += float64(s) / 32768
		}
		out[i] = float32(acc / float64(channels))
	}
	return out, rate, nil
}

// specTopColors prints the most common exact pixel values in a render. Summary
// statistics across two different color schemes are not comparable — blue is
// (0,0,255) and averages 85 while a grey of the same magnitude averages 51 —
// so when two renders of identical data disagree, the way to find out what is
// actually in them is to look at what colors they are made of.
func specTopColors(path string) {
	img, err := loadPNG(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec:", err)
		os.Exit(2)
	}
	b := img.Bounds()
	count := map[[3]int]int{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			count[[3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}]++
		}
	}
	type kv struct {
		c [3]int
		n int
	}
	all := make([]kv, 0, len(count))
	for c, n := range count {
		all = append(all, kv{c, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	n := float64(b.Dx() * b.Dy())
	fmt.Printf("%s: %d distinct colors\n", path, len(all))
	for i := 0; i < 6 && i < len(all); i++ {
		fmt.Printf("  rgb(%3d,%3d,%3d)  %5.2f%%\n",
			all[i].c[0], all[i].c[1], all[i].c[2], 100*float64(all[i].n)/n)
	}
}
