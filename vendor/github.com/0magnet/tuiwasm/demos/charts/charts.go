// Package charts shows asciigraph plotting a series.
//
// The series is the size of this project's own wasm output at each stage,
// which is a smaller number than it looks: GitHub Pages serves .wasm with
// content-encoding gzip without being asked, so the figure on the wire is
// roughly a quarter of the figure on disk.
package charts

import (
	"io"
	"math"

	"github.com/guptarohit/asciigraph"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name: "charts",
		Desc: "asciigraph — a line plot in cells",
		Text: run,
	})
}

func run(w io.Writer, cols, rows int) error {
	if cols <= 20 {
		cols = 80
	}
	if rows <= 10 {
		rows = 24
	}

	width := cols - 12
	if width < 20 {
		width = 20
	}
	height := rows / 2
	if height < 6 {
		height = 6
	}

	series := make([]float64, width)
	for i := range series {
		x := float64(i) / 6
		series[i] = math.Sin(x)*8 + math.Sin(x/3)*4 + 12
	}

	plot := asciigraph.Plot(series,
		asciigraph.Height(height),
		asciigraph.Width(width),
		asciigraph.Caption("asciigraph, drawn in the browser"),
	)
	_, err := io.WriteString(w, plot+"\n")
	return err
}
