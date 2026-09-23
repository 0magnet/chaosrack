//go:build !js

package server

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/attractor"
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
)

func init() {
	renderCmd.Flags().StringVar(&renderModel, "model", "", "which model to draw (see --list)")
	renderCmd.Flags().StringVarP(&renderOut, "out", "o", "", "file to write; .png or .svg by extension")
	renderCmd.Flags().IntVar(&renderW, "width", 1000, "image width in pixels")
	renderCmd.Flags().IntVar(&renderH, "height", 1000, "image height in pixels")
	renderCmd.Flags().IntVar(&renderPts, "points", 20000, "how many points of trajectory to keep")
	renderCmd.Flags().Float64Var(&renderSecs, "seconds", 0, "how much model time to integrate (0 = the model's default)")
	renderCmd.Flags().Float64SliceVar(&renderSpin, "angle", []float64{0.6, 0.9, 0}, "view angles x,y,z in radians")
	renderCmd.Flags().IntVar(&renderColors, "colors", 4, "palette: 1 mono, 2 two-color, 3 three-color, 4 rainbow")
	renderCmd.Flags().BoolVar(&renderCheck, "check", false, "integrate every model and report the ones that draw nothing")
	renderCmd.Flags().StringArrayVar(&renderSet, "set", nil, "turn a control before drawing, as id=value (repeatable)")
	renderCmd.Flags().BoolVar(&renderParams, "params", false, "list the controls that --set can turn, with their ranges")
	runCmd.AddCommand(renderCmd, modelsCmd)
}

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "list the models that can be drawn without a browser",
	Long: `List the flows.

A subset of the catalog, and the honest answer to what works headless: the
models that publish a vector field to the flow registry. The audio displays
and the DOM-backed models — terminal, desk, the STL viewer — are not here,
because without a browser they have no signal and no surface to read.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		for _, k := range dynamics.Keys() {
			fmt.Println(k)
		}
		return nil
	},
}

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "draw a model to a PNG or SVG, without a browser",
	Long: `Draw a model to a file.

  chaosrack render --model lorenz -o lorenz.png
  chaosrack render --model halvorsen -o h.svg --angle 0.3,1.2,0
  chaosrack render --check

SVG is a polyline, which is what a trail actually is: it scales, it diffs, and
it can be read. PNG goes through the same software renderer, so the depth
shading and the gradient are the page's.

--check is the one that earns this command. It integrates every flow and
reports any whose trajectory has no extent — a model that draws a point rather
than an attractor. That is a failure the Lyapunov guard cannot catch for a
model it cannot measure, and it is how a third of the Sprott catalog came to
be static without a test noticing.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		if renderParams {
			return listParams()
		}
		if err := applySets(); err != nil {
			return err
		}
		if renderCheck {
			return checkEveryFlow()
		}
		if renderModel == "" {
			return fmt.Errorf("--model is required (chaosrack models lists them, --check tests them all)")
		}
		if !dynamics.HasFlow(renderModel) {
			return fmt.Errorf("no model named %q; chaosrack models lists the %d that can be drawn headless",
				renderModel, len(dynamics.Keys()))
		}
		pts := trajectoryFor(renderModel)
		if len(pts) == 0 {
			return fmt.Errorf("%s: integrated to nothing — the trajectory diverged", renderModel)
		}
		dx, dy, dz := attractor.Extent(pts)
		if math.Max(dx, math.Max(dy, dz)) == 0 {
			return fmt.Errorf("%s: the trajectory has no extent — it is a fixed point, not an attractor", renderModel)
		}
		if renderOut == "" {
			fmt.Printf("%s: %d points, extent %.3f x %.3f x %.3f\n", renderModel, len(pts), dx, dy, dz)
			return nil
		}
		return writeModel(renderOut, pts)
	},
}

// trajectoryFor integrates one model at the flags' settings.
func trajectoryFor(model string) [][3]float64 {
	o := dynamics.DefaultTrajectory()
	if renderPts > 1 {
		o.MaxPoints = renderPts
	}
	if renderSecs > 0 {
		o.Duration = renderSecs
	}
	return dynamics.Trajectory(model, o)
}

// writeModel writes the picture in whichever format the name asks for.
func writeModel(name string, pts [][3]float64) error {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		// 0600: gosec is right that a tool should not widen permissions on the
		// user's behalf, and the umask gives them whatever they wanted anyway.
		return os.WriteFile(name, []byte(svgOf(pts)), 0o600)
	case ".png", "":
		f, err := os.Create(name) //nolint:gosec // the path is the user's own argument
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // the encode error below is the one that matters
		return png.Encode(f, attractor.Draw(pts, drawOptions()))
	default:
		return fmt.Errorf("%s: unknown format — use .png or .svg", filepath.Ext(name))
	}
}

func drawOptions() attractor.DrawOptions {
	a := [3]float64{}
	copy(a[:], renderSpin)
	g := rasterview.DefaultGradient()
	if renderColors > 0 {
		g.Colors = renderColors
	}
	return attractor.DrawOptions{
		Width:  renderW,
		Height: renderH,
		View: rasterview.View{
			AngleX: a[0], AngleY: a[1], AngleZ: a[2],
		},
		Gradient: g,
	}
}

// checkEveryFlow integrates every model and reports the dead ones.
//
// The check this package could not make before: a Lyapunov exponent needs a
// registered deriv, and a model whose flow is computed some other way is
// skipped rather than failed. Extent needs only the trajectory.
func checkEveryFlow() error {
	keys := dynamics.Keys()
	sort.Strings(keys)
	var dead []string
	for _, k := range keys {
		pts := trajectoryFor(k)
		if len(pts) == 0 {
			dead = append(dead, k+" (diverged)")
			continue
		}
		dx, dy, dz := attractor.Extent(pts)
		span := math.Max(dx, math.Max(dy, dz))
		if span == 0 {
			dead = append(dead, k+" (a fixed point)")
			continue
		}
		fmt.Printf("%-16s %6d points   %8.3f x %8.3f x %8.3f\n", k, len(pts), dx, dy, dz)
	}
	if len(dead) > 0 {
		return fmt.Errorf("%d of %d models draw nothing: %s", len(dead), len(keys), strings.Join(dead, ", "))
	}
	fmt.Printf("\nall %d models draw something\n", len(keys))
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
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := os.Stdout.WriteString(b.String())
	return err
}
