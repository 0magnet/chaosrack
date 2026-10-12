//go:build js && wasm

package commands

import (
	"context"
	"fmt"
	"os"
	"syscall/js"

	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"
	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/internal/rackcable"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/cmdfs"
	"github.com/0magnet/chaosrack/pkg/subcmds"
)

// platform adds nothing the shared tree lacks.
func platform(*cobra.Command) {}

// panel runs the panel on the terminal the command was typed in.
func panel(cmd *cobra.Command) error { return attractor.Panel(cmd.Context()) }

// pageEval evaluates in this page: the rack ctl and rack reach is the one
// this process is running.
type pageEval struct{}

// Eval gives back what CDP would have: a string, bool, float64 or nil.
func (pageEval) Eval(expr string) any {
	v := js.Global().Call("eval", expr)
	switch v.Type() {
	case js.TypeString:
		return v.String()
	case js.TypeBoolean:
		return v.Bool()
	case js.TypeNumber:
		return v.Float()
	}
	return nil
}

// rackDialer reaches the rack in this page.
func rackDialer() subcmds.Dialer {
	return subcmds.Dialer{Dial: func() (*rackcable.Client, error) { return rackcable.Over(pageEval{}) }}
}

// Shell runs chaosrack as typed in one of the page's terminals: args is the
// command line, ctx the shell's, whose stdio, files and working directory it
// uses.
func Shell(ctx context.Context, args []string) int {
	hc := interp.HandlerCtx(ctx)
	// The process has no environment of its own; coloredcobra looks at TERM
	// and NO_COLOR to decide whether the help is colored.
	for _, name := range []string{"TERM", "NO_COLOR"} {
		if v := hc.Env.Get(name); v.IsSet() {
			_ = os.Setenv(name, v.String()) //nolint:errcheck // cannot fail in js
		} else {
			_ = os.Unsetenv(name) //nolint:errcheck // cannot fail in js
		}
	}
	if sh := shell.FromContext(ctx); sh != nil && sh.FS != nil {
		ctx = cmdfs.With(ctx, sh.FS, hc.Dir)
	}
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
