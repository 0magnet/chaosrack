package subcmds

import (
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/cmdfs"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// `chaosrack render` — a model to a file, with no browser anywhere.
//
// The instrument is wasm in a page and most of it has to be: the audio
// displays need a signal, the rack's layout is measured from real text in a
// real font, and the tube is a GPU. The MODELS are not like that. A flow is a
// vector field and a timestep, and drawing one is arithmetic — which is why
// pkg/attractor.Trajectory has always run on the host and pkg/rasterview has
// always been a pure software renderer.
//
// This joins them, and the reason is a bug that shipped. The chaos guard walks
// the catalog asking each mode for a Lyapunov exponent; the Sprott morph's
// flow is a blended coefficient table rather than a registered deriv, so it
// answers "n/a", the test skips it, and ten of nineteen systems drew a single
// motionless point for however long that had been true. `render --check`
// answers the question that cannot be skipped — did it draw anything — for
// every flow, in about a second, from a shell.

var (
	renderModel  string
	renderOut    string
	renderW      int
	renderH      int
	renderPts    int
	renderSecs   float64
	renderSpin   []float64
	renderCheck  bool
	renderColors int
	renderSet    []string
	renderParams bool
	renderFrames int
	renderFPS    int
	renderTurn   []float64
	renderTrail  int
	renderGrad   string
	renderBG     string
	renderSweep  = true // the flag's default, for callers that never parse flags (the tests)
	renderWalk   string
	renderCrop   bool
	renderRock   bool
	renderTumble int
)

func renderFlags(renderCmd *cobra.Command) {
	renderCmd.Flags().StringVar(&renderModel, "model", "", "which model to draw (chaosrack models lists them)")
	renderCmd.Flags().StringVarP(&renderOut, "out", "o", "", "file to write; .png or .svg by extension")
	renderCmd.Flags().IntVar(&renderW, "width", 1000, "image width in pixels")
	renderCmd.Flags().IntVar(&renderH, "height", 1000, "image height in pixels")
	renderCmd.Flags().IntVar(&renderPts, "points", 20000, "how many points of trajectory to keep")
	renderCmd.Flags().Float64Var(&renderSecs, "seconds", 0, "how much model time to integrate (0 = the model's default)")
	renderCmd.Flags().Float64SliceVar(&renderSpin, "angle", []float64{0.6, 0.9, 0}, "view angles x,y,z in radians")
	renderCmd.Flags().IntVar(&renderColors, "colors", 2, "palette: 1 mono, 2 two-color (the page's default), 3 three-color, 4 rainbow")
	renderCmd.Flags().BoolVar(&renderCheck, "check", false, "integrate every model and report the ones that draw nothing")
	renderCmd.Flags().StringArrayVar(&renderSet, "set", nil, "turn a control before drawing, as id=value (repeatable)")
	renderCmd.Flags().BoolVar(&renderParams, "params", false, "list the controls that --set can turn, with their ranges")
	renderCmd.Flags().IntVar(&renderFrames, "frames", 0, "write an animation of this many frames (.gif or .svg); 0 = a still")
	renderCmd.Flags().IntVar(&renderFPS, "fps", 20, "frames per second for --frames")
	renderCmd.Flags().Float64SliceVar(&renderTurn, "turn", []float64{0, 0, 0}, "radians the camera also turns over the animation, x,y,z (default: still camera)")
	renderCmd.Flags().IntVar(&renderTrail, "trail", 0, "points of trajectory visible at once (0 = a quarter of the run)")
	renderCmd.Flags().StringVar(&renderGrad, "gradient", "z", "what the palette follows: x, y, z, or trail (position along the path)")
	renderCmd.Flags().StringVar(&renderBG, "background", "#000000", "background color, #rrggbb (the page's canvas is black)")
	renderCmd.Flags().BoolVar(&renderSweep, "sweep", true, "in an animation, sweep a trail along the path; false draws the whole figure every frame, so only --turn moves it")
	renderCmd.Flags().BoolVar(&renderCrop, "crop", false, "frame the picture to what is drawn (across all frames of an animation) rather than to the figure's extent")
	renderCmd.Flags().BoolVar(&renderRock, "rock", false, "in an animation, swing the --turn out and back on a sine, centered on --angle, instead of turning steadily; a loop without a seam that need not be a whole turn")
	renderCmd.Flags().IntVar(&renderTumble, "tumble", 0, "in an animation, make this many eased quarter turns, alternately right and down, instead of --turn; 6 comes back to the start")
	renderCmd.Flags().StringVar(&renderWalk, "walk", "", "in an animation, step the colors through COLORS/GRADIENT settings in equal parts, e.g. 4/z,2/x,3/y,1/trail; a still takes the first")
}

// newModelsCmd is `chaosrack models`.
func newModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "list the models that can be drawn without a browser",
		Long: `List the models that render can draw, with their labels and catalog groups.

These are the models that can be computed without a browser: the flows, the
discrete maps, the Lissajous figure, the polyhedra, and the sphere, torus,
globe and magnetosphere wireframes.

The models marked "--audio or --signal" are drawn from sound: the Takens,
stereo and polar embeddings, the xy scope, the spectrogram, the RTA, the
transfer function, the recurrence plot and the waterfall. render records them
from this machine's audio, or draws them from a built-in test signal.

Page-based models such as the terminal, the desk and the STL viewer are not
listed, because they need a browser.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			labels := map[string][2]string{}
			for _, g := range attractor.Catalog() {
				for _, m := range g.Models {
					if _, dup := labels[m.Key]; !dup {
						labels[m.Key] = [2]string{m.Label, g.Label}
					}
				}
			}
			w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "MODEL\tLABEL\tGROUP\tDRAWN FROM"); err != nil {
				return err
			}
			row := func(k, from string) error {
				l := labels[k]
				if l[0] == "" {
					l = [2]string{k, "Attractors"}
				}
				_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", k, l[0], l[1], from)
				return err
			}
			for _, k := range drawableKeys() {
				if err := row(k, ""); err != nil {
					return err
				}
			}
			for _, k := range audioInputModels() {
				if err := row(k, "--audio or --signal"); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

// newRenderCmd is `chaosrack render`, its flags bound afresh.
func newRenderCmd() *cobra.Command {
	renderCmd := &cobra.Command{
		Use:   "render",
		Short: "draw a model to a PNG, SVG or GIF, without a browser",
		Long: `Draw a model to an image file without a browser.

The output format comes from the -o file extension:

  .png   a still image with the same depth shading and colors as the page
  .jpg   the same, as a JPEG
  .svg   a still image as a vector polyline, or an animation with --frames
  .gif   an animation (requires --frames, except for audio models)

Without -o, render prints the number of points and the size of the result
instead of writing a file.

  chaosrack render --model lorenz -o lorenz.png
  chaosrack render --model halvorsen -o h.svg --angle 0.3,1.2,0
  chaosrack render --model lorenz -o lorenz.gif --frames 120 --turn 0,6.283,0
  chaosrack render --model lorenz --set lorenz-r=40 -o rho40.png
  chaosrack render --model clifford -o clifford.png
  chaosrack render --model icosahedron -o ico.gif --frames 60 --turn 0,6.283,0
  chaosrack render --model lorenz -o loop.gif --frames 48 --turn 0,6.283,0 --sweep=false --walk 4/z,2/x,3/y,1/trail

The colors and background default to the page's: a red-to-blue gradient
along z on black. In an animation a trail sweeps along the path; --sweep=false
draws the whole figure every frame, so a full --turn loops without a seam, and
--walk steps the palette and its source through the animation while the turn
carries on smoothly.

Flows and curves are drawn as a line, maps as points, and polyhedra and other
geometry as wireframes. Maps are drawn face-on and colored by y unless
--angle or --gradient is given. In an animation a map's points accumulate
frame by frame.

Use chaosrack models to list the available models. Use --params to list the
controls that --set can change, with their ranges. Values outside a control's
range are rejected rather than clamped.

AUDIO MODELS

The models that are drawn from sound need a signal. --audio SECONDS records
that long from this machine's audio through PulseAudio or PipeWire, by
default whatever is playing (--audio-source monitor). --signal NAME uses a
built-in test signal instead, 2 seconds of it unless --audio says otherwise.

  chaosrack render --model takens --audio 5 -o music.png
  chaosrack render --model stereo --audio 10 -o music.gif
  chaosrack render --model spectrogram --audio 10 -o spec.png
  chaosrack render --model rta --signal pink -o rta.png
  chaosrack render --model xfer --signal oop -o xfer.png

- takens, stereo, polar and xy draw the last window of the signal as the page
  does: 85 ms (43 ms for xy), or --window. takens and polar measure the delay
  from the signal as the page does, unless --tau is given.
- spectrogram, rta, xfer and recurrence are drawn as .png plots of the whole
  signal. The transfer function runs from the left channel (reference) to
  the right (measurement).
- waterfall is a 3-D surface of the spectrum at even steps through the
  signal. With --impulse it is instead the cumulative spectral decay from
  the left channel to the right, as the page draws it after a sweep.
- A .gif of an audio model plays back in real time: its frames step through
  the signal, one per 1/--fps of a second, unless --frames is given. The
  waterfall is the exception: it is one surface, animated with --frames and
  --turn like the other 3-D models.

--check draws every model that needs no signal and reports any that produce
nothing: a trajectory that diverges, or one that stays at a single point. It
exits with an error if any fail.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if renderParams {
				return listParams()
			}
			if err := applySets(); err != nil {
				return err
			}
			if renderCheck {
				return checkEveryModel()
			}
			if _, err := parseWalk(renderWalk); err != nil {
				return err
			}
			if renderModel == "" {
				return fmt.Errorf("--model is required (chaosrack models lists them, --check tests them all)")
			}
			if isAudioInput(renderModel) {
				return renderAudio(cmd)
			}
			fig, err := figureFor(renderModel)
			if err != nil {
				return err
			}
			// A map's iterates lie in the z=0 plane, so the default oblique view
			// squashes them and a z gradient paints them one color. Face-on and
			// colored by y is how they read, unless the flags say otherwise.
			if fig.Kind == attractor.FigurePoints {
				if !cmd.Flags().Changed("angle") {
					renderSpin = []float64{0, 0, 0}
				}
				if !cmd.Flags().Changed("gradient") {
					renderGrad = "y"
				}
			}
			pts := fig.Points
			if len(pts) == 0 {
				return fmt.Errorf("%s: integrated to nothing — the trajectory diverged", renderModel)
			}
			dx, dy, dz := attractor.Extent(pts)
			if math.Max(dx, math.Max(dy, dz)) == 0 {
				return fmt.Errorf("%s: the figure has no extent — it is a single point", renderModel)
			}
			if renderFrames > 0 {
				if renderOut == "" {
					return fmt.Errorf("--frames needs -o: an animation has nowhere to go")
				}
				return writeFigureAnimation(renderOut, fig)
			}
			if renderOut == "" {
				outf("%s: %d points, extent %.3f x %.3f x %.3f\n", renderModel, len(pts), dx, dy, dz)
				return nil
			}
			return writeFigure(renderOut, fig)
		},
	}
	renderFlags(renderCmd)
	audioFlags(renderCmd)
	recordFlags(renderCmd)
	return renderCmd
}

// trajectoryFor integrates one model at the flags' settings.
//
// A DECIMATED TRACE IS A FACETED ONE, and the default used to be decimated.
// Trajectory thins to MaxPoints by keeping every Nth step, so 220 seconds of
// Lorenz at dt=0.005 — 44,000 steps — inside 20,000 points keeps every third,
// and the curve is drawn as chords across three integration steps. That is
// visibly polygonal at the turns, where the app's own trail, which draws every
// step it integrates, is smooth. It looks like a worse renderer and is not.
//
// So the run is sized to the point budget rather than the budget being spent
// on a longer run: with no --seconds, integrate exactly as long as --points can
// hold at full resolution. A longer run still works and still decimates —
// there is no way for it not to — but it now says so, because a quietly
// coarser picture is the kind of difference that gets blamed on the renderer.
func trajectoryFor(model string) [][3]float64 {
	o := dynamics.DefaultTrajectory()
	if renderPts > 1 {
		o.MaxPoints = renderPts
	}
	dt, _, haveFlow := dynamics.FlowFor(model)
	switch {
	case renderSecs > 0:
		o.Duration = renderSecs
		if haveFlow && dt > 0 {
			steps := int(renderSecs / dt)
			// The same rounding Trajectory uses, or the message names a stride
			// the picture was not drawn at.
			if every := (steps + o.MaxPoints - 1) / o.MaxPoints; every > 1 {
				fit := float64(o.MaxPoints) * dt
				advice := fmt.Sprintf("--seconds %.4g fits the points asked for", fit)
				// Only suggest more points if they can actually be indexed:
				// the renderer's index type is uint16.
				if steps <= 65536 {
					advice = fmt.Sprintf("--points %d draws every step, or ", steps) + advice
				} else {
					advice += fmt.Sprintf(" (a run this long cannot be drawn whole: %d steps is past the %d the renderer can index)",
						steps, 65536)
				}
				errf(
					"chaosrack: %s over %g model-seconds is %d steps drawn with %d points — "+
						"one point kept in %d, so the curve will be faceted.\n  %s.\n",
					model, renderSecs, steps, o.MaxPoints, every, advice)
			}
		}
	case haveFlow && dt > 0:
		// The whole budget at full resolution: one point per integration step.
		o.Duration = float64(o.MaxPoints) * dt
	}
	return dynamics.Trajectory(model, o)
}

// writeModel writes the picture in whichever format the name asks for.
func writeModel(name string, pts [][3]float64) error {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		// 0600: gosec is right that a tool should not widen permissions on the
		// user's behalf, and the umask gives them whatever they wanted anyway.
		return cmdfs.WriteFile(runCtx, name, []byte(svgOf(pts)), 0o600)
	case ".jpg", ".jpeg":
		f, err := cmdfs.Create(runCtx, name)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // the encode error below is the one that matters
		return jpeg.Encode(f, cropOne(attractor.Draw(pts, drawOptions())), &jpeg.Options{Quality: 90})
	case ".png", "":
		f, err := cmdfs.Create(runCtx, name)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // the encode error below is the one that matters
		return png.Encode(f, cropOne(attractor.Draw(pts, drawOptions())))
	default:
		return fmt.Errorf("%s: unknown format — use .png, .jpg or .svg", filepath.Ext(name))
	}
}

func drawOptions() attractor.DrawOptions {
	w, h := drawSize()
	a := [3]float64{}
	copy(a[:], renderSpin)
	// gradientFor, not a second copy of it. This built its own and so ignored
	// --gradient entirely: the palette followed the flag in an SVG and stayed
	// on z in a PNG, from one flag that only half the writers read — the same
	// mistake the hardcoded SVG stroke was.
	return attractor.DrawOptions{
		Width:  w,
		Height: h,
		View: rasterview.View{
			AngleX: a[0], AngleY: a[1], AngleZ: a[2],
		},
		Gradient:   gradientFor(),
		Background: background(),
	}
}

// checkEveryModel builds every model render can draw and reports the dead
// ones.
//
// The check this package could not make before: a Lyapunov exponent needs a
// registered deriv, and a model whose flow is computed some other way is
// skipped rather than failed. Extent needs only the points.
func checkEveryModel() error {
	keys := drawableKeys()
	var dead []string
	for _, k := range keys {
		fig, err := figureFor(k)
		if err != nil || len(fig.Points) == 0 {
			dead = append(dead, k+" (diverged)")
			continue
		}
		dx, dy, dz := attractor.Extent(fig.Points)
		span := math.Max(dx, math.Max(dy, dz))
		if span == 0 || math.IsNaN(span) || math.IsInf(span, 0) {
			dead = append(dead, k+" (a fixed point)")
			continue
		}
		outf("%-16s %6d points   %8.3f x %8.3f x %8.3f\n", k, len(fig.Points), dx, dy, dz)
	}
	if len(dead) > 0 {
		return fmt.Errorf("%d of %d models draw nothing: %s", len(dead), len(keys), strings.Join(dead, ", "))
	}
	outf("\nall %d models draw something\n", len(keys))
	return nil
}

// applySets turns the controls named by --set before anything is integrated.
//
// It goes through dynamics.SetParam rather than writing the variable, so the
// range the panel's knob enforces is enforced here too: a value the knob could
// not have produced is refused instead of quietly clamped, because a test that
// asks for rho=600 and silently measures rho=60 reports a pass about a system
// nobody ran.
func applySets() error {
	for _, s := range renderSet {
		id, val, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("--set %q is not id=value", s)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(val), 32)
		if err != nil {
			return fmt.Errorf("--set %s: %q is not a number", id, val)
		}
		// The scope models' knobs (turtle, Graphic Artist, Sprott Morph)
		// are the page's own state, not the parameter registry's.
		if attractor.SetFigureKnob(strings.TrimSpace(id), float32(v)) {
			continue
		}
		if err := dynamics.SetParam(strings.TrimSpace(id), float32(v)); err != nil {
			return fmt.Errorf("--set: %w", err)
		}
	}
	return nil
}

// listParams prints what --set can turn. The same table the panel builds its
// knobs from, which is the point: what you can set from a shell and what you
// can turn on screen are one list.
func listParams() error {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tLABEL\tDEFAULT\tRANGE\tSTEP"); err != nil {
		return err
	}
	for _, mode := range dynamics.ParamModes() {
		for _, p := range dynamics.Params(mode) {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%g\t%g .. %g\t%g\n",
				p.ID, p.Label, p.Def, p.Min, p.Max, p.Step); err != nil {
				return err
			}
		}
	}
	for _, k := range attractor.FigureKnobs() {
		if _, err := fmt.Fprintf(w, "%s\t\t%g\t\t\n", k.ID, k.Def); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(stdout, b.String())
	return err
}
