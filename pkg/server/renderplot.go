//go:build !js

package server

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/meters"
	"github.com/0magnet/chaosrack/pkg/recurrence"
	"github.com/0magnet/chaosrack/pkg/spectcol"
)

// The audio analyzers, as pictures.
//
// On the page these are panels — a spectrogram texture, an RTA's bars, a
// transfer function's curves, a recurrence plot's square — drawn from analysis
// that is already untagged in pkg/attractor and pkg/meters. Here the same
// analysis runs over a recording and the panel is drawn into an image. Each
// plot is a function of how much of the recording it has heard, so a still
// is the whole recording and an animation's frames are it growing.
//
// The waterfall is the exception: it is a 3-D surface on the page too, so it
// is a Figure and goes through the same writers as every other model.

// plotModels are the analyzers drawn as flat plots.
var plotModels = []string{"spectrogram", "rta", "xfer", "recurrence"}

func isPlotModel(key string) bool {
	for _, k := range plotModels {
		if k == key {
			return true
		}
	}
	return false
}

// plotFrame draws analyzer key over the first end samples of l and r.
func plotFrame(key string, l, r []float32, end int) (*image.RGBA, error) {
	total := len(l)
	l, r = l[:end], r[:end]
	switch key {
	case "spectrogram":
		return plotSpectrogram(mix(l, r), total)
	case "rta":
		return plotRTA(mix(l, r))
	case "xfer":
		return plotTransfer(l, r)
	case "recurrence":
		return plotRecurrence(mix(l, r))
	}
	return nil, fmt.Errorf("%s is not a plot", key)
}

// plotMinSamples is how much signal an analyzer needs before it can say
// anything; an animation starts there.
func plotMinSamples(key string) int {
	switch key {
	case "spectrogram":
		return sg.S.GetDFTSize()
	case "rta":
		return rtaFFT
	case "xfer":
		return xferFFT + (acoustics.TransferMinAvg-1)*xferFFT/2
	case "recurrence":
		span, _ := recurrenceWindow()
		return span
	}
	return 1
}

func mix(l, r []float32) []float32 {
	out := make([]float32, len(l))
	for i := range out {
		out[i] = (l[i] + r[i]) * 0.5
	}
	return out
}

// ── the canvas ───────────────────────────────────────────────────────────

var (
	plotBG    = color.RGBA{10, 13, 20, 255} // the panel's own near-black
	plotGrid  = color.RGBA{40, 46, 60, 255}
	plotText  = color.RGBA{150, 160, 180, 255}
	plotTrace = color.RGBA{80, 200, 255, 255}
	plotPeak  = color.RGBA{255, 160, 60, 255}
)

// canvas is an image with a plot area inside margins for the labels.
type canvas struct {
	img        *image.RGBA
	x0, y0     int // plot area, top left
	x1, y1     int // plot area, bottom right
	title      string
	face       font.Face
	lineHeight int
}

func newCanvas(title string) *canvas {
	c := &canvas{
		img:        image.NewRGBA(image.Rect(0, 0, renderW, renderH)),
		face:       basicfont.Face7x13,
		lineHeight: 13,
		title:      title,
	}
	draw.Draw(c.img, c.img.Bounds(), &image.Uniform{plotBG}, image.Point{}, draw.Src)
	c.x0, c.y0 = 56, 44
	c.x1, c.y1 = renderW-16, renderH-32
	c.text(8, 18, title, plotText)
	return c
}

func (c *canvas) text(x, y int, s string, col color.Color) {
	d := font.Drawer{Dst: c.img, Src: image.NewUniform(col), Face: c.face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func (c *canvas) textRight(x, y int, s string, col color.Color) {
	d := font.Drawer{Face: c.face}
	c.text(x-d.MeasureString(s).Round(), y, s, col)
}

func (c *canvas) textCenter(x, y int, s string, col color.Color) {
	d := font.Drawer{Face: c.face}
	c.text(x-d.MeasureString(s).Round()/2, y, s, col)
}

func (c *canvas) hline(y int, col color.Color) {
	for x := c.x0; x <= c.x1; x++ {
		c.img.Set(x, y, col)
	}
}

func (c *canvas) vline(x int, col color.Color) {
	for y := c.y0; y <= c.y1; y++ {
		c.img.Set(x, y, col)
	}
}

func (c *canvas) fill(x0, y0, x1, y1 int, col color.Color) {
	draw.Draw(c.img, image.Rect(x0, y0, x1, y1).Intersect(c.img.Bounds()), &image.Uniform{col}, image.Point{}, draw.Over)
}

// line draws a segment with a DDA, like rasterview.
func (c *canvas) line(xa, ya, xb, yb float64, col color.Color) {
	steps := int(math.Max(math.Abs(xb-xa), math.Abs(yb-ya))) + 1
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		c.img.Set(int(xa+(xb-xa)*t), int(ya+(yb-ya)*t), col)
	}
}

// ── frequency axes ───────────────────────────────────────────────────────

const plotLoHz, plotHiHz = 20.0, 20000.0

// freqX places a frequency on a logarithmic 20 Hz – 20 kHz axis.
func (c *canvas) freqX(hz float64) float64 {
	t := math.Log(hz/plotLoHz) / math.Log(plotHiHz/plotLoHz)
	return float64(c.x0) + t*float64(c.x1-c.x0)
}

func (c *canvas) freqGrid() {
	for _, hz := range []float64{20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000} {
		x := int(c.freqX(hz))
		c.vline(x, plotGrid)
		lbl := fmt.Sprintf("%g", hz)
		if hz >= 1000 {
			lbl = fmt.Sprintf("%gk", hz/1000)
		}
		c.textCenter(x, c.y1+16, lbl, plotText)
	}
	c.textCenter((c.x0+c.x1)/2, c.y1+29, "Hz", plotText)
}

// dbGrid labels a dB axis between lo and hi over rows y0..y1.
func (c *canvas) dbGrid(lo, hi float64, y0, y1, step int) func(float64) float64 {
	yOf := func(db float64) float64 {
		t := (hi - db) / (hi - lo)
		return float64(y0) + math.Max(0, math.Min(1, t))*float64(y1-y0)
	}
	for db := int(hi); db >= int(lo); db -= step {
		y := int(yOf(float64(db)))
		c.hline(y, plotGrid)
		c.textRight(c.x0-6, y+4, fmt.Sprintf("%d", db), plotText)
	}
	return yOf
}

// ── spectrogram ──────────────────────────────────────────────────────────

// plotSpectrogram is `uitool spec`'s picture: audioprism's columns, time left
// to right and 0 Hz at the bottom, stretched to the image.
//
// total is the length of the whole recording, so that an animation's picture
// is laid out against the length it will reach and grows across the frame.
func plotSpectrogram(x []float32, total int) (*image.RGBA, error) {
	size := sg.S.GetDFTSize()
	step := sg.S.StepSize()
	if step <= 0 {
		step = size / 2
	}
	if len(x) < size {
		return nil, fmt.Errorf("spectrogram needs at least %d samples", size)
	}
	rows := spectcol.Rows(size)
	cols := (len(x)-size)/step + 1
	src := image.NewRGBA(image.Rect(0, 0, cols, rows))
	frame := make([]float32, size)
	for i := 0; i < cols; i++ {
		copy(frame, x[i*step:i*step+size])
		col := spectcol.Column(spectcol.Mags(frame), rows)
		for y := 0; y < rows && len(col) >= rows*4; y++ {
			src.SetRGBA(i, rows-1-y, color.RGBA{col[y*4], col[y*4+1], col[y*4+2], 255})
		}
	}
	c := newCanvas(fmt.Sprintf("spectrogram  %d-point FFT, %d-sample hop, %.2f s", size, step, float64(len(x))/renderSampleRate))
	// Nearest-neighbor into the plot area.
	full := (total - size) / step
	if full < cols {
		full = cols
	}
	w, h := c.x1-c.x0, c.y1-c.y0
	for py := 0; py < h; py++ {
		sy := py * rows / h
		for px := 0; px < w; px++ {
			sx := px * full / w
			if sx >= cols {
				break
			}
			c.img.SetRGBA(c.x0+px, c.y0+py, src.RGBAAt(sx, sy))
		}
	}
	nyq := renderSampleRate / 2
	for _, f := range []int{0, nyq / 4, nyq / 2, 3 * nyq / 4, nyq} {
		y := c.y1 - f*h/nyq
		c.textRight(c.x0-6, y+4, fmt.Sprintf("%gk", float64(f)/1000), plotText)
	}
	secs := float64(total) / renderSampleRate
	for i := 0; i <= 4; i++ {
		c.textCenter(c.x0+i*w/4, c.y1+16, fmt.Sprintf("%.2gs", secs*float64(i)/4), plotText)
	}
	return c.img, nil
}

// ── RTA ──────────────────────────────────────────────────────────────────

const rtaFFT = 16384 // the page's analysis length

// plotRTA is the third-octave RTA over the recording: the power average of
// every window as bars, and the loudest any window reached as a peak tick.
func plotRTA(x []float32) (*image.RGBA, error) {
	if len(x) < rtaFFT {
		return nil, fmt.Errorf("rta needs at least %d ms of signal", rtaFFT*1000/renderSampleRate)
	}
	bands := acoustics.RTABands(3)
	avg := make([]float64, len(bands))
	peak := make([]float64, len(bands))
	levels := make([]float64, len(bands))
	for i := range peak {
		peak[i] = acoustics.RTAFloorDB
	}
	n := 0
	for at := 0; at+rtaFFT <= len(x); at += rtaFFT / 2 {
		mags := meters.ComputeFFTMagsKind(x[at:at+rtaFFT], acoustics.RTAWindowKind)
		acoustics.RTALevels(mags, rtaFFT, renderSampleRate, bands, acoustics.RTAWindowKind, levels)
		for i, db := range levels {
			avg[i] += math.Pow(10, db/10)
			peak[i] = math.Max(peak[i], db)
		}
		n++
	}
	c := newCanvas(fmt.Sprintf("RTA  1/3 octave, average of %d windows", n))
	c.freqGrid()
	yOf := c.dbGrid(-70, 0, c.y0, c.y1, 10)
	c.textRight(c.x0-6, c.y0-8, "dBFS", plotText)
	for i, b := range bands {
		db := 10 * math.Log10(avg[i]/float64(n))
		xa, xb := c.freqX(b.Lo)+1, c.freqX(b.Hi)-1
		c.fill(int(xa), int(yOf(db)), int(xb), c.y1, plotTrace)
		py := int(yOf(peak[i]))
		c.fill(int(xa), py-1, int(xb), py+1, plotPeak)
	}
	return c.img, nil
}

// ── transfer function ────────────────────────────────────────────────────

const (
	xferFFT = 8192 // the page's analysis length
)

// plotTransfer is the transfer function from the left channel (reference) to
// the right (measurement): magnitude above, phase below, each band drawn as
// brightly as its coherence and grey where the reference had no energy.
func plotTransfer(l, r []float32) (*image.RGBA, error) {
	var acc acoustics.TransferAccum
	for at := 0; at+xferFFT <= len(l); at += xferFFT / 2 {
		acc.Add(l[at:at+xferFFT], r[at:at+xferFFT], acoustics.TransferWindowKind)
	}
	res := acc.Result(renderSampleRate, 6)
	if !res.OK {
		return nil, fmt.Errorf("xfer needs at least %d windows (%.1f s of signal)",
			acoustics.TransferMinAvg, float64(plotMinSamples("xfer"))/renderSampleRate)
	}
	c := newCanvas(fmt.Sprintf("transfer function  left -> right, 1/6 octave, %d averages", res.Averages))
	c.freqGrid()
	split := c.y0 + (c.y1-c.y0)*3/5
	magY := c.dbGrid(-40, 40, c.y0, split-8, 10)
	phY := func(deg float64) float64 {
		return float64(split+8) + (180-deg)/360*float64(c.y1-split-8)
	}
	for _, d := range []int{180, 0, -180} {
		y := int(phY(float64(d)))
		c.hline(y, plotGrid)
		c.textRight(c.x0-6, y+4, fmt.Sprintf("%d", d), plotText)
	}
	c.text(c.x0+6, c.y0+14, "magnitude, dB", plotText)
	c.text(c.x0+6, split+24, "phase, degrees", plotText)
	var px, pm, pp float64
	for i, b := range res.Bands {
		x := c.freqX(b.Center)
		var col color.RGBA
		if res.RefDB[i] < -60 {
			col = plotGrid // nothing excited this band
		} else {
			k := math.Max(0.25, res.Coherence[i])
			col = color.RGBA{uint8(80 * k), uint8(200 * k), uint8(255 * k), 255}
		}
		m, p := magY(res.MagDB[i]), phY(res.PhaseDeg[i])
		if i > 0 {
			c.line(px, pm, x, m, col)
			if math.Abs(p-pp) < float64(c.y1-split)/2 { // don't join across a wrap
				c.line(px, pp, x, p, col)
			}
		}
		px, pm, pp = x, m, p
	}
	if d, ok := acoustics.TransferDelayMS(res, 0.8); ok {
		c.textRight(c.x1, c.y0-8, fmt.Sprintf("delay %.2f ms", d), plotText)
	}
	return c.img, nil
}

// ── recurrence plot ──────────────────────────────────────────────────────

const (
	recN   = 256  // the page's plot size, points per side
	recWin = 100  // the page's default window, ms
	recEps = 0.05 // the page's default threshold, a fraction of full scale
)

func recurrenceWindow() (span, stride int) {
	span = recWin * renderSampleRate / 1000
	stride = max(span/recN, 1)
	return stride * recN, stride
}

// plotRecurrence is the recurrence plot of the newest window of raw samples,
// box-filtered down to 256 points as the page does, with its RQA measures.
func plotRecurrence(x []float32) (*image.RGBA, error) {
	span, stride := recurrenceWindow()
	if len(x) < span {
		return nil, fmt.Errorf("recurrence needs at least %d ms of signal", recWin)
	}
	base := len(x) - span
	series := make([]float64, recN)
	for i := range series {
		var s float32
		for k := 0; k < stride; k++ {
			s += x[base+i*stride+k]
		}
		series[i] = float64(s) / float64(stride)
	}
	mat := make([]byte, recN*recN)
	recurrence.MatrixVec(series, 1, recEps*recurrence.VectorScale(1), mat)
	q := recurrence.RQA(mat, recN)
	c := newCanvas(fmt.Sprintf("recurrence  %d ms, eps %.2f   RR %.3f  DET %.3f  LAM %.3f", recWin, recEps, q.RR, q.DET, q.LAM))
	side := min(c.x1-c.x0, c.y1-c.y0)
	ox, oy := c.x0+(c.x1-c.x0-side)/2, c.y0
	for py := 0; py < side; py++ {
		j := (side - 1 - py) * recN / side // time runs up, as on the page
		for px := 0; px < side; px++ {
			if mat[j*recN+px*recN/side] != 0 {
				c.img.SetRGBA(ox+px, oy+py, plotTrace)
			}
		}
	}
	c.textCenter(ox+side/2, c.y1+16, "time ->", plotText)
	return c.img, nil
}

// ── waterfall ────────────────────────────────────────────────────────────

const (
	wfallSlices = 16   // the page's default line count
	wfallFFT    = 2048 // the page's default transform
	wfallRange  = 40.0 // dB shown below the top
)

// waterfallFigure is the waterfall surface: frequency across on a log axis,
// level up, and time into the screen. With --impulse it is the cumulative
// spectral decay of the impulse response from left (reference) to right
// (measurement), as the page draws after a sweep; otherwise it is the page's
// live mode, the spectrum at even steps through the recording.
func waterfallFigure(l, r []float32) (attractor.Figure, error) {
	freqs := acoustics.LogFreqPoints(plotLoHz, plotHiHz, 96)
	var slices [][]float64
	if renderImpulse {
		n := 1
		for n*2 <= len(l) {
			n *= 2
		}
		if n < 1<<15 {
			return attractor.Figure{}, fmt.Errorf("--impulse needs at least %.2f s of signal", float64(1<<15)/renderSampleRate)
		}
		ir := acoustics.ImpulseResponse(l[len(l)-n:], r[len(r)-n:], 1e-4)
		for _, s := range acoustics.CSD(ir, renderSampleRate, wfallSlices, 5, wfallFFT, freqs) {
			slices = append(slices, s.DB)
		}
	} else {
		x := mix(l, r)
		if len(x) < wfallFFT {
			return attractor.Figure{}, fmt.Errorf("waterfall needs at least %d samples", wfallFFT)
		}
		for i := 0; i < wfallSlices; i++ {
			// Newest first, like the page: the front line is now.
			end := len(x) - (len(x)-wfallFFT)*i/(wfallSlices-1)
			db := make([]float64, len(freqs))
			if acoustics.SpectrumPoints(x[end-wfallFFT:end], renderSampleRate, freqs, meters.WinHann, db) {
				slices = append(slices, db)
			}
		}
	}
	if len(slices) == 0 {
		return attractor.Figure{}, fmt.Errorf("waterfall: no spectrum to draw")
	}
	top := math.Inf(-1)
	for _, s := range slices {
		for _, v := range s {
			top = math.Max(top, v)
		}
	}
	var f attractor.Figure
	f.Kind = attractor.FigureLines
	for si, s := range slices {
		z := float64(si)/float64(len(slices)-1)*2 - 1
		for i, v := range s {
			y := math.Max(0, math.Min(1, (v-(top-wfallRange))/wfallRange))
			f.Points = append(f.Points, [3]float64{float64(i)/float64(len(s)-1)*2 - 1, y, z})
			if i > 0 {
				k := len(f.Points) - 1
				f.Edges = append(f.Edges, uint16(k-1), uint16(k)) //nolint:gosec // 16 × 96 points
			}
		}
	}
	return f, nil
}
