// Package commands assembles the one chaosrack command, the same command on
// every target: on a host it is this binary, and in the page it is what the
// page's terminals run when chaosrack is typed there.
//
// chaosrack itself is the panel. Where it finds the rack is the only thing
// that differs: in the page the rack is the process the command runs in, and
// on a host it is a page in a browser, reached over a cable (host.go). The
// subcommands that need a host — serve, audiows, uitool — are only in the host
// build.
package commands

import (
	"fmt"

	"github.com/0magnet/calvin/clihelp"
	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/racktui"
	"github.com/0magnet/chaosrack/pkg/stlgen"
	"github.com/0magnet/chaosrack/pkg/subcmds"
)

// New is the chaosrack command, built fresh: in the page it runs once per
// time it is typed, and flags must not carry over from the last time.
func New() *cobra.Command {
	var (
		look  string
		plain bool
	)
	root := &cobra.Command{
		Use:          "chaosrack",
		Short:        "the rack's panel, in this terminal",
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
	root.AddCommand(subcmds.Commands(rackDialer())...)
	root.AddCommand(stlgen.Command())
	platform(root)
	clihelp.Init(root, "chaosrack")
	return root
}
