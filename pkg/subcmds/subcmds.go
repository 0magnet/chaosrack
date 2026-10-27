// Package subcmds holds the chaosrack subcommands that run the same on a host
// and in the page: ctl and rack drive the running rack, mem reads its memory,
// and render and models draw models with no browser at all.
//
// They print through their command's output and write files through cmdfs,
// because in the page the os package's stdout is the browser console and it
// has no filesystem: the terminal and the shell's files are the cobra
// command's.
package subcmds

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/0magnet/chaosrack/internal/rackcable"
)

// The run in progress: its output, and the context its files are written
// under. Set as each command starts.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	runCtx           = context.Background()
)

// Dialer reaches the running rack, for ctl and rack: over a browser's
// debugging port on a host, and by calling it in the page.
type Dialer struct {
	// Flags adds the flags that say where the rack is, if any.
	Flags func(*pflag.FlagSet)
	// Dial reaches it.
	Dial func() (*rackcable.Client, error)
}

// Commands are ctl, rack, render, models, manual and mem, built fresh.
func Commands(d Dialer) []*cobra.Command {
	cmds := []*cobra.Command{newCtlCmd(d), newRackCmd(d), newRenderCmd(), newModelsCmd(), newManualCmd(d), newMemCmd(d)}
	for _, c := range cmds {
		run := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			stdout, stderr, runCtx = cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.Context()
			return run(cmd, args)
		}
	}
	return cmds
}

func newRackCmd(d Dialer) *cobra.Command {
	var monitor, slots int
	cmd := &cobra.Command{
		Use:   "rack",
		Short: "draw the rack's bays as text",
		Long: `Print the running rack's layout as text.

Each row of the rack is a fixed number of slots. Each module takes up a whole
number of slots, and blank panels fill the rest. The layout printed is the
one the page is currently using.

--slots sets the number of slots per row. By default the page decides.
--monitor N also prints the layout with an N-slot monitor at the start of
each bay, for comparison.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := d.Dial()
			if err != nil {
				return err
			}
			c.Capacity = slots
			asIs, err := c.Rack()
			if err != nil {
				return err
			}
			outln("AS IT IS")
			outs(asIs)
			if monitor > 0 {
				c.Monitor = monitor
				withMon, err := c.Rack()
				if err != nil {
					return err
				}
				outf("\nWITH A %d-SLOT MONITOR IN EVERY BAY\n", monitor)
				outs(withMon)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&monitor, "monitor", 0, "draw the bays again with a chassis monitor this many slots wide")
	cmd.Flags().IntVar(&slots, "slots", 0, "slots per row (0 = ask the page)")
	if d.Flags != nil {
		d.Flags(cmd.Flags())
	}
	return cmd
}

func newCtlCmd(d Dialer) *cobra.Command {
	var values bool
	cmd := &cobra.Command{
		Use:   "ctl [id | id=value ...]",
		Short: "list, read or set the rack's controls",
		Long: `List, read or set the running rack's controls by id.

With no arguments, ctl lists every control with its label and range. Add
--values to show every control's current value too. An argument that is just
an id prints that control's value. An argument of the form id=value sets it.

  chaosrack ctl
  chaosrack ctl wf-win
  chaosrack ctl wf-win=2 lufs-rate=1000

Setting a control has the same effect as turning it in the page, and the
page's permalink updates to match.`,
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := d.Dial()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return listControls(c, values)
			}
			for _, a := range args {
				id, val, isSet := strings.Cut(a, "=")
				if !isSet {
					v, err := c.Get(id)
					if err != nil {
						return err
					}
					outln(v)
					continue
				}
				if err := c.Set(id, val); err != nil {
					return err
				}
				outf("%s = %s\n", id, val)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&values, "values", false, "read every control's current value too")
	if d.Flags != nil {
		d.Flags(cmd.Flags())
	}
	return cmd
}

func listControls(c *rackcable.Client, values bool) error {
	ctls, err := c.Controls()
	if err != nil {
		return err
	}
	outf("%-24s %-14s %-12s %s\n", "ID", "LABEL", "VALUE", "RANGE")
	for _, x := range ctls {
		rng := strings.Join(x.Options, " ")
		if !x.IsSelect {
			rng = fmt.Sprintf("%g .. %g / %g", x.Min, x.Max, x.Step)
		}
		val := ""
		if values || x.IsSelect {
			val = x.Value
		}
		outf("%-24s %-14s %-12s %s\n", x.ID, x.Label, val, rng)
	}
	outf("\n%d controls\n", len(ctls))
	return nil
}

// The run's own printing. Like the fmt.Print* it replaces, a write the
// terminal refused is not a reason to stop: the files are what a run is for.
func outf(format string, a ...any) { fmt.Fprintf(stdout, format, a...) } //nolint:errcheck // see above
func outln(a ...any)               { fmt.Fprintln(stdout, a...) }        //nolint:errcheck // see above
func outs(a ...any)                { fmt.Fprint(stdout, a...) }          //nolint:errcheck // see above
func errf(format string, a ...any) { fmt.Fprintf(stderr, format, a...) } //nolint:errcheck // see above
func errln(a ...any)               { fmt.Fprintln(stderr, a...) }        //nolint:errcheck // see above
