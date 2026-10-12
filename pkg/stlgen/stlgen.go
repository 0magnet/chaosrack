// Package stlgen (`chaosrack stlgen`) writes the rack and its models as STL files.
//
//	chaosrack stlgen                 # everything, into docs/stl
//	chaosrack stlgen --only lorenz    # one model
//	chaosrack stlgen --list           # what it would write
//
// Three families come out of it:
//
//   - the rack — a 3U module panel with a body behind it that slides into a
//     frame, in one, two and three slot widths, plus the 19-inch frame itself;
//   - the geometry — the Platonic solids, sphere, torus and nested cube, as
//     closed solids rather than the wireframes the app draws;
//   - the attractors — every registered flow, integrated with its own
//     timestep from its own initial condition and swept as a tube.
//
// The same generators back the app's built-in STL models, so a file written
// here is the model the STL mode shows.
package stlgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/cmdfs"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// Command is `chaosrack stlgen`.
func Command() *cobra.Command {
	var o options
	cmd := &cobra.Command{
		Use:   "stlgen",
		Short: "write the rack and its models as STL files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.outDir, "dir", "docs/stl", "where to write the files")
	f.StringVar(&o.only, "only", "", "comma-separated name filter")
	f.BoolVar(&o.list, "list", false, "list what would be written and stop")
	f.IntVar(&o.seg, "seg", 0, "segments per revolution (0 = each model's default)")
	return cmd
}

type options struct {
	outDir string
	only   string
	list   bool
	seg    int
}

// writeSTL writes one mesh to path.
func writeSTL(ctx context.Context, path string, mesh meshstl.Mesh, header string) error {
	f, err := cmdfs.Create(ctx, path)
	if err != nil {
		return err
	}
	err = meshstl.WriteBinarySTL(f, mesh, header)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func run(ctx context.Context, out, errOut io.Writer, o options) error {
	outf := func(format string, a ...any) { fmt.Fprintf(out, format, a...) }    //nolint:errcheck // as fmt.Printf was: the files are what a run is for
	errf := func(format string, a ...any) { fmt.Fprintf(errOut, format, a...) } //nolint:errcheck // as above
	models := attractor.STLModels()
	want := map[string]bool{}
	if o.only != "" {
		for k := range strings.SplitSeq(o.only, ",") {
			want[strings.TrimSpace(k)] = true
		}
	}

	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	sort.Strings(names)

	if o.list {
		for _, m := range models {
			outf("%-16s %s\n", m.Name, m.Description)
		}
		outf("\n%d models\n", len(models))
		return nil
	}

	if err := cmdfs.MkdirAll(ctx, o.outDir, 0o750); err != nil {
		return err
	}

	var written, skipped int
	var total int
	for _, m := range models {
		if len(want) > 0 && !want[m.Name] {
			continue
		}
		mesh := m.Build(o.seg)
		if len(mesh.Tris) == 0 {
			errf("  %-16s empty — skipped\n", m.Name)
			skipped++
			continue
		}
		if err := writeSTL(ctx, filepath.Join(o.outDir, m.Name+".stl"), mesh, m.Description); err != nil {
			return err
		}
		size := mesh.Size()
		outf("  %-16s %7d tris  %6.1f x %6.1f x %6.1f mm\n",
			m.Name+".stl", len(mesh.Tris), size[0], size[1], size[2])
		written++
		total += len(mesh.Tris)
	}
	if len(want) == 0 {
		ef, et, err := writeExactPanels(ctx, out, o.outDir, o.seg)
		if err != nil {
			return err
		}
		written += ef
		total += et
	}
	outf("stlgen: %d files, %d triangles → %s\n", written, total, o.outDir)
	if skipped > 0 {
		outf("stlgen: %d skipped\n", skipped)
	}
	// A reminder of what the rack numbers are, since this is the command that
	// makes them physical.
	outf("stlgen: a slot is %d HP = %.2f mm wide and %.1f mm tall; %d fit an %d HP row\n",
		rackspec.ModuleHP, rackspec.SlotWidth, rackspec.PanelHeight3U,
		rackspec.SlotsPerRow(), rackspec.RowHP)
	if n := len(dynamics.Keys()); n > 0 {
		outf("stlgen: %d of them are integrated flows\n", n)
	}
	return nil
}

// writeExactPanels emits one STL per REAL module, from the layout `uitool
// modules` measured off the running rack.
//
// The built-in rack models are generic: a plausible module with three knobs
// down the middle. These are the actual ones — the Console's concentric model
// selector where the Console's concentric model selector is, the Patchbay's
// thirty-six pins on the pitch they are really on — because a model of
// something that merely looks like the rack is not a model of the rack.
func writeExactPanels(ctx context.Context, out io.Writer, dir string, seg int) (files, tris int, err error) {
	outf := func(format string, a ...any) { fmt.Fprintf(out, format, a...) } //nolint:errcheck // as fmt.Printf was
	data, err := cmdfs.ReadFile(ctx, filepath.Join(dir, "layout.json"))
	if err != nil {
		outf("%s\n", "stlgen: no measured layout (run `chaosrack uitool modules` against a tab); "+
			"the rack models are the generic ones")
		return 0, 0, nil
	}
	var lays []meshstl.PanelLayout
	if err := json.Unmarshal(data, &lays); err != nil {
		return 0, 0, fmt.Errorf("reading the layout: %w", err)
	}
	for _, l := range lays {
		if l.HP < 1 {
			continue
		}
		mesh := meshstl.ExactPanel(l, true, seg)
		name := "panel-" + l.ID + ".stl"
		if err := writeSTL(ctx, filepath.Join(dir, name), mesh, l.Label+" — measured panel"); err != nil {
			return files, tris, err
		}
		size := mesh.Size()
		outf("  %-22s %7d tris  %6.1f x %6.1f x %6.1f mm  (%d HP, %d controls)\n",
			name, len(mesh.Tris), size[0], size[1], size[2], l.HP, len(l.Controls))
		files++
		tris += len(mesh.Tris)
	}
	return files, tris, nil
}
