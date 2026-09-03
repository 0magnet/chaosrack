// Command stlgen writes the rack and its models as STL files.
//
//	go run ./cmd/stlgen                 # everything, into docs/stl
//	go run ./cmd/stlgen -only lorenz    # one model
//	go run ./cmd/stlgen -list           # what it would write
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
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/rackspec"
)

var (
	outDir = flag.String("dir", "docs/stl", "where to write the files")
	only   = flag.String("only", "", "comma-separated name filter")
	list   = flag.Bool("list", false, "list what would be written and stop")
	seg    = flag.Int("seg", 0, "segments per revolution (0 = each model's default)")
)

func main() {
	flag.Parse()

	models := attractor.STLModels()
	want := map[string]bool{}
	if *only != "" {
		for _, k := range strings.Split(*only, ",") {
			want[strings.TrimSpace(k)] = true
		}
	}

	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	sort.Strings(names)

	if *list {
		for _, m := range models {
			fmt.Printf("%-16s %s\n", m.Name, m.Description)
		}
		fmt.Printf("\n%d models\n", len(models))
		return
	}

	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "stlgen:", err)
		os.Exit(1)
	}

	var written, skipped int
	var total int
	for _, m := range models {
		if len(want) > 0 && !want[m.Name] {
			continue
		}
		mesh := m.Build(*seg)
		if len(mesh.Tris) == 0 {
			fmt.Fprintf(os.Stderr, "  %-16s empty — skipped\n", m.Name)
			skipped++
			continue
		}
		path := filepath.Join(*outDir, m.Name+".stl")
		f, err := os.Create(path) //nolint:gosec // a path this command was told to write
		if err != nil {
			fmt.Fprintln(os.Stderr, "stlgen:", err)
			os.Exit(1)
		}
		err = meshstl.WriteBinarySTL(f, mesh, m.Description)
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "stlgen:", err)
			os.Exit(1)
		}
		size := mesh.Size()
		fmt.Printf("  %-16s %7d tris  %6.1f x %6.1f x %6.1f mm\n",
			m.Name+".stl", len(mesh.Tris), size[0], size[1], size[2])
		written++
		total += len(mesh.Tris)
	}
	if len(want) == 0 {
		ef, et := writeExactPanels(*outDir, *seg)
		written += ef
		total += et
	}
	fmt.Printf("stlgen: %d files, %d triangles → %s\n", written, total, *outDir)
	if skipped > 0 {
		fmt.Printf("stlgen: %d skipped\n", skipped)
	}
	// A reminder of what the rack numbers are, since this is the command that
	// makes them physical.
	fmt.Printf("stlgen: a slot is %d HP = %.2f mm wide and %.1f mm tall; %d fit an %d HP row\n",
		rackspec.ModuleHP, rackspec.SlotWidth, rackspec.PanelHeight3U,
		rackspec.SlotsPerRow(), rackspec.RowHP)
	if n := len(attractor.FlowKeys()); n > 0 {
		fmt.Printf("stlgen: %d of them are integrated flows\n", n)
	}
}

// writeExactPanels emits one STL per REAL module, from the layout `uitool
// modules` measured off the running rack.
//
// The built-in rack models are generic: a plausible module with three knobs
// down the middle. These are the actual ones — the Console's concentric model
// selector where the Console's concentric model selector is, the Patchbay's
// thirty-six pins on the pitch they are really on — because a model of
// something that merely looks like the rack is not a model of the rack.
func writeExactPanels(dir string, seg int) (files, tris int) {
	data, err := os.ReadFile(filepath.Join(dir, "layout.json")) //nolint:gosec // a generated file at a path this command owns
	if err != nil {
		fmt.Println("stlgen: no measured layout (run `uitool modules` against a tab); " +
			"the rack models are the generic ones")
		return 0, 0
	}
	var lays []meshstl.PanelLayout
	if err := json.Unmarshal(data, &lays); err != nil {
		fmt.Fprintln(os.Stderr, "stlgen: reading the layout:", err)
		return 0, 0
	}
	for _, l := range lays {
		if l.HP < 1 {
			continue
		}
		mesh := meshstl.ExactPanel(l, true, seg)
		path := filepath.Join(dir, "panel-"+l.ID+".stl")
		f, err := os.Create(path) //nolint:gosec // a path this command was told to write
		if err != nil {
			fmt.Fprintln(os.Stderr, "stlgen:", err)
			return files, tris
		}
		err = meshstl.WriteBinarySTL(f, mesh, l.Label+" — measured panel")
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "stlgen:", err)
			return files, tris
		}
		size := mesh.Size()
		fmt.Printf("  %-22s %7d tris  %6.1f x %6.1f x %6.1f mm  (%d HP, %d controls)\n",
			"panel-"+l.ID+".stl", len(mesh.Tris), size[0], size[1], size[2], l.HP, len(l.Controls))
		files++
		tris += len(mesh.Tris)
	}
	return files, tris
}
