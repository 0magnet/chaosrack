// Package commands assembles the one chaosrack command, the same command on
// every target: on a host it is this binary, and in the page it is what the
// instrument's terminal runs when chaosrack is typed there.
//
// chaosrack itself is the panel. Where it finds the rack is the only thing
// that differs: in the page the rack is the process the command runs in, and
// on a host it is a page in a browser, reached over a cable (host.go). The
// subcommands that need a host — serve above all — are only in the host
// build.
package commands

import (
	"fmt"

	"github.com/0magnet/calvin/clihelp"
	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

const long = `chaosrack is an analog computer: a rack of instruments for dynamical
systems, geometry and live signals, computed by a Go WebAssembly core and
played from a panel of knobs, switches and LED readouts.

What the rack can show:

  flows        Lorenz, Rössler, Chua, the Sprott cases, or a system you type in
  maps         Hénon, Ikeda, Clifford, de Jong, Tinkerbell, the standard map
  geometry     parametric figures, polyhedra, sequence walks and solids,
               with the rack's own panels exportable as printable STL
  audio        spectrogram, XY scope, delay embeddings, and an analyzer suite:
               distortion, RTA, transfer function, loudness, wow & flutter,
               waterfall
  ripple tank  a wave-equation water surface driven by drags or by sound
  terminal     a real shell in the page, and a desk of windows to work in

With no subcommand, chaosrack is the rack's panel, in this terminal:

  up/down        select a control (PgUp/PgDn jump 10, Home/End go to the ends)
  left/right     turn it one step, or move a switch one position
  ctrl+arrows    scroll the view
  0              reset the control to its default
  tab            switch between the panel and list views
  type           filter controls by name (backspace erases, / clears)
  r              reload the controls from the page
  q, esc         quit`

// New is the chaosrack command, built fresh: in the page it runs once per
// time it is typed, and flags must not carry over from the last time.
func New() *cobra.Command {
	var (
		look  string
		plain bool
	)
	root := &cobra.Command{
		Use:          "chaosrack",
		Short:        "an analog computer",
		Long:         long + platformHelp,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, ok := racktui.ParseLook(look)
			if !ok {
				return fmt.Errorf("--look %q: want page, dial or punit", look)
			}
			racktui.SetLook(l)
			racktui.SetPlain(plain)
			return panel(cmd)
		},
	}
	root.Flags().StringVar(&look, "look", "page", "how the panel is drawn: page (as the page draws it, at a fixed scale), dial, or punit")
	root.Flags().BoolVar(&plain, "plain", false, "cells only: lay none of the page's own parts over them (w switches it while the panel runs)")
	platform(root)
	clihelp.Init(root, "chaosrack")
	return root
}
