//go:build js && wasm

package commands

import (
	"context"
	"fmt"

	"github.com/0magnet/sh/v3/interp"
	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

const platformHelp = `

Here the rack is this page, and the panel is the program it runs: ctrl+c, or
q, stops the model and its panel, and chaosrack starts them again.`

// platform adds nothing: in the page the rack is in this process, and the
// subcommands all need a host.
func platform(*cobra.Command) {}

// panel runs the panel on the terminal the command was typed in.
func panel(cmd *cobra.Command) error { return attractor.Panel(cmd.Context()) }

// Shell runs chaosrack as typed in one of the page's terminals: args is the
// command line, ctx the shell's, whose stdio it uses.
func Shell(ctx context.Context, args []string) int {
	hc := interp.HandlerCtx(ctx)
	root := New()
	root.SetArgs(args[1:])
	root.SetIn(hc.Stdin)
	root.SetOut(hc.Stdout)
	root.SetErr(hc.Stderr)
	root.SilenceErrors = true
	if err := root.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(hc.Stderr, "chaosrack:", err) //nolint:errcheck // a closed stderr is not a reason to do anything else
		return 1
	}
	return 0
}
