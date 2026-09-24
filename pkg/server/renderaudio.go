//go:build !js

package server

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/audiocap"
	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/gifenc"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// `chaosrack render` for the models the page draws from live audio.
//
// The page gets its audio from this binary: --audio on the server records
// the machine's sound through pkg/audiocap and streams it to the tab. render
// records through the same package and draws the recording itself, so an
// embedding of whatever is playing needs no browser either. A built-in test
// signal stands in for the recording when there is nothing to record, or when
// the picture has to come out the same every time.

var (
	renderAudioSecs float64
	renderAudioSrc  string
	renderSignal    string
	renderTau       float64
	renderWindow    float64
	renderImpulse   bool
)

// renderSampleRate is what render records and generates at: the rate the τ
// knob is counted in, so the delay means the same number of samples here as
// its label says.
const renderSampleRate = 48000

func init() {
	renderCmd.Flags().Float64Var(&renderAudioSecs, "audio", 0, "for an audio model, record this many seconds of what is playing (or, with --signal, generate that much; default 2)")
	renderCmd.Flags().StringVar(&renderAudioSrc, "audio-source", "monitor", "what --audio records: monitor (what is playing), default (the input), or a source ID")
	renderCmd.Flags().StringVar(&renderSignal, "signal", "", "for an audio model, draw a built-in test signal instead of recording: "+strings.Join(signalNames(), ", "))
	renderCmd.Flags().Float64Var(&renderTau, "tau", 0, "embedding delay in samples at 48 kHz (0 = measured from the signal for takens and polar, as the page does; 72 otherwise)")
	renderCmd.Flags().BoolVar(&renderImpulse, "impulse", false, "waterfall: draw the cumulative spectral decay of the impulse response from the left channel (reference) to the right (measurement), as the page does after a sweep")
	renderCmd.Flags().Float64Var(&renderWindow, "window", 0, "milliseconds of signal in each picture (0 = the page's default: 85, or 43 for xy)")
}

// signalNames are the short names --signal takes, the test-signal dial's own
// ring labels.
func signalNames() []string {
	return audiosrc.TestSignalRing[1:]
}

func testSignal(name string) (audiosrc.TestSignal, error) {
	for i, n := range audiosrc.TestSignalRing {
		if i > 0 && (strings.EqualFold(n, name) || strings.EqualFold(audiosrc.TestSignalNames[i], name)) {
			return audiosrc.TestSignal(i), nil
		}
	}
	return 0, fmt.Errorf("--signal %q: not a test signal; use one of %s", name, strings.Join(signalNames(), ", "))
}

func audioOptions() attractor.AudioOptions {
	return attractor.AudioOptions{
		SampleRate: renderSampleRate,
		Tau:        float32(renderTau),
		WindowMS:   float32(renderWindow),
		Budget:     renderPts,
	}
}

// audioSignal records or generates the stereo signal an audio model draws.
func audioSignal() (l, r []float32, err error) {
	secs := renderAudioSecs
	if renderSignal != "" {
		sig, err := testSignal(renderSignal)
		if err != nil {
			return nil, nil, err
		}
		if secs <= 0 {
			secs = 2
		}
		n := int(secs * renderSampleRate)
		l, r = make([]float32, n), make([]float32, n)
		audiosrc.NewTestSource(sig, renderSampleRate).Fill(l, r)
		return l, r, nil
	}
	if secs <= 0 {
		return nil, nil, fmt.Errorf("%s is drawn from audio: --audio SECONDS records what is playing, --signal NAME uses a test signal", renderModel)
	}
	return record(secs)
}

// record captures secs seconds of stereo audio through PulseAudio/PipeWire.
func record(secs float64) (l, r []float32, err error) {
	want := int(secs * renderSampleRate)
	l, r = make([]float32, 0, want), make([]float32, 0, want)
	done := make(chan struct{})
	full := false
	fmt.Fprintf(os.Stderr, "chaosrack: recording %gs from %s\n", secs, renderAudioSrc)
	stop, err := audiocap.Options{
		SampleRate: renderSampleRate,
		Channels:   2,
		Source:     renderAudioSrc,
	}.Start(func(p []float32) error {
		if full {
			return nil
		}
		for i := 0; i+1 < len(p) && len(l) < want; i += 2 {
			l = append(l, p[i])
			r = append(r, p[i+1])
		}
		if len(l) >= want {
			full = true
			close(done)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	select {
	case <-done:
	case <-time.After(time.Duration(secs*float64(time.Second)) + 5*time.Second):
		stop()
		return nil, nil, fmt.Errorf("recording stalled after %d of %d samples", len(l), want)
	}
	stop()
	return l, r, nil
}

// isAudioInput reports whether a model is drawn from a signal: an embedding,
// an analyzer plot, or the waterfall.
func isAudioInput(key string) bool {
	return attractor.IsAudioModel(key) || isPlotModel(key) || key == "waterfall"
}

// audioInputModels lists them in the order models prints them.
func audioInputModels() []string {
	return append(append(append([]string{}, attractor.AudioModels...), plotModels...), "waterfall")
}

// renderAudio draws a model from a signal: a still of the whole recording, or
// an animation whose frames step through it.
func renderAudio(cmd *cobra.Command) error {
	l, r, err := audioSignal()
	if err != nil {
		return err
	}
	if rms(l, r) < 1e-4 {
		fmt.Fprintln(os.Stderr, "chaosrack: the signal is silent (is anything playing?)")
	}
	if renderModel == "waterfall" {
		fig, err := waterfallFigure(l, r)
		if err != nil {
			return err
		}
		if renderFrames > 0 {
			if renderOut == "" {
				return fmt.Errorf("--frames needs -o: an animation has nowhere to go")
			}
			return writeFigureAnimation(renderOut, fig)
		}
		if renderOut == "" {
			return fmt.Errorf("waterfall needs -o")
		}
		if strings.EqualFold(filepath.Ext(renderOut), ".gif") {
			return fmt.Errorf("the waterfall is one surface, so a .gif of it needs --frames, and --turn to rotate it")
		}
		return writeFigure(renderOut, fig)
	}
	if isPlotModel(renderModel) {
		return renderPlot(cmd, l, r)
	}
	o := audioOptions()
	if o.Tau <= 0 && (renderModel == "takens" || renderModel == "polar") {
		if tau, ok := attractor.MeasureTau(l, r, renderSampleRate); ok {
			o.Tau = tau
			fmt.Fprintf(os.Stderr, "chaosrack: measured τ = %g samples (%.2f ms)\n", tau, float64(tau)*1000/renderSampleRate)
		}
	}
	need := attractor.AudioWindow(renderModel, o)
	if len(l) < need {
		return fmt.Errorf("%s needs at least %d ms of signal", renderModel, need*1000/renderSampleRate)
	}

	frames := renderFrames
	if frames == 0 && renderOut != "" && strings.EqualFold(filepath.Ext(renderOut), ".gif") && !cmd.Flags().Changed("frames") {
		// A GIF of a recording plays back in real time unless asked otherwise.
		frames = int(float64(len(l)) / renderSampleRate * float64(renderFPS))
	}
	if frames <= 1 {
		fig, _ := attractor.AudioFigure(renderModel, l, r, len(l), o)
		if renderOut == "" {
			dx, dy, dz := attractor.Extent(fig.Points)
			fmt.Printf("%s: %d points, extent %.3f x %.3f x %.3f\n", renderModel, len(fig.Points), dx, dy, dz)
			return nil
		}
		return writeFigure(renderOut, fig)
	}
	if renderOut == "" {
		return fmt.Errorf("--frames needs -o: an animation has nowhere to go")
	}

	// Frame i shows the window ending at an even step through the signal.
	figs := make([]attractor.Figure, frames)
	var all [][3]float64
	for i := range figs {
		end := need + (len(l)-need)*i/(frames-1)
		figs[i], _ = attractor.AudioFigure(renderModel, l, r, end, o)
		all = append(all, figs[i].Points...)
	}
	views := frameViews(frames)
	switch strings.ToLower(filepath.Ext(renderOut)) {
	case ".gif":
		imgs := make([]*image.RGBA, frames)
		for i, a := range views {
			d := drawOptions()
			d.View.AngleX, d.View.AngleY, d.View.AngleZ = a[0], a[1], a[2]
			d.Fit = all
			imgs[i] = attractor.DrawFigure(figs[i], 0, len(figs[i].Points), d)
		}
		f, err := os.Create(renderOut) //nolint:gosec // the path is the user's own argument
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // the encode error below is the one that matters
		delay := int(math.Round(100 / float64(renderFPS)))
		if delay < 1 {
			delay = 1
		}
		return gifenc.EncodeRGBA(f, imgs, delay)
	case ".svg":
		return os.WriteFile(renderOut, []byte(audioSVG(figs, all, views)), 0o600)
	default:
		return fmt.Errorf("%s: --frames writes .gif or .svg", filepath.Ext(renderOut))
	}
}

// audioSVG shows a different figure in each frame, all framed alike.
func audioSVG(figs []attractor.Figure, all [][3]float64, views [][3]float64) string {
	mid := attractor.BoundsMid(all)
	shift := func(ps [][3]float64) [][3]float64 {
		out := make([][3]float64, len(ps))
		for i, p := range ps {
			out[i] = [3]float64{p[0] - mid[0], p[1] - mid[1], p[2] - mid[2]}
		}
		return out
	}
	r, mx, my := svgFit(shift(all), views)
	g := gradientFor()
	min, max := rasterview.ModelBounds(attractor.Vertices(shift(all)))
	dur := float64(len(views)) / float64(renderFPS)

	var defs, body strings.Builder
	for i, a := range views {
		vals := make([]string, len(views))
		for k := range vals {
			vals[k] = "0"
		}
		vals[i] = "1"
		fmt.Fprintf(&body, `<g opacity="0"><animate attributeName="opacity" calcMode="discrete" dur="%gs" repeatCount="indefinite" values="%s"/>`,
			dur, strings.Join(vals, ";"))
		pts := shift(figs[i].Points)
		writeColoredTrail(&defs, &body, fmt.Sprintf("g%d", i), pts, a, r, mx, my, g, min, max, 0, len(pts))
		body.WriteString(`</g>`)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		renderW, renderH, renderW, renderH)
	if defs.Len() > 0 {
		b.WriteString(`<defs>` + defs.String() + `</defs>`)
	}
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0a0d14"/>`, renderW, renderH)
	b.WriteString(body.String())
	b.WriteString(`</svg>`)
	return b.String()
}

func rms(l, r []float32) float64 {
	if len(l) == 0 {
		return 0
	}
	var s float64
	for i := range l {
		s += float64(l[i])*float64(l[i]) + float64(r[i])*float64(r[i])
	}
	return math.Sqrt(s / float64(2*len(l)))
}

// renderPlot writes an analyzer plot: a PNG of the whole recording, or a GIF
// whose frames show the analysis as the recording is heard.
func renderPlot(cmd *cobra.Command, l, r []float32) error {
	if renderOut == "" {
		return fmt.Errorf("%s needs -o", renderModel)
	}
	ext := strings.ToLower(filepath.Ext(renderOut))
	switch ext {
	case ".png", "":
		img, err := plotFrame(renderModel, l, r, len(l))
		if err != nil {
			return err
		}
		f, err := os.Create(renderOut) //nolint:gosec // the path is the user's own argument
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // the encode error below is the one that matters
		return png.Encode(f, img)
	case ".gif":
	default:
		return fmt.Errorf("%s: %s is drawn as .png, or .gif for an animation", ext, renderModel)
	}
	frames := renderFrames
	if !cmd.Flags().Changed("frames") {
		frames = int(float64(len(l)) / renderSampleRate * float64(renderFPS))
	}
	start := plotMinSamples(renderModel)
	if len(l) < start {
		_, err := plotFrame(renderModel, l, r, len(l))
		return err
	}
	frames = max(frames, 1)
	imgs := make([]*image.RGBA, 0, frames)
	for i := 0; i < frames; i++ {
		end := len(l)
		if frames > 1 {
			end = start + (len(l)-start)*i/(frames-1)
		}
		img, err := plotFrame(renderModel, l, r, end)
		if err != nil {
			return err
		}
		imgs = append(imgs, img)
	}
	f, err := os.Create(renderOut) //nolint:gosec // the path is the user's own argument
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the encode error below is the one that matters
	return gifenc.EncodeRGBA(f, imgs, max(int(math.Round(100/float64(renderFPS))), 1))
}
