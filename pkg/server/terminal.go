//go:build !js

package server

// The rack, from a terminal.
//
// These ship in the default build rather than in a tool beside it, because a
// terminal panel is not a test harness — it is a second front end over the
// same instrument, and the one you reach for when the browser is on another
// screen, or when you want to sweep a knob and watch a reading rather than
// drag something with a mouse.
//
// They drive a RUNNING rack: the instrument is wasm in a page and these are
// the cable to it (internal/rackcable). That is also why -attach defaults to
// this binary's own port — the usual case is the server you just started and
// the tab you opened on it.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/internal/rackcable"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

var (
	cdpPort   int
	attachTo  string
	bayMon    int
	rowSlots  int
	ctlValues bool
)

func init() {
	for _, c := range []*cobra.Command{tuiCmd, ctlCmd, rackCmd} {
		c.Flags().IntVar(&cdpPort, "cdp", 9222, "the browser's remote-debugging port")
		c.Flags().StringVar(&attachTo, "attach", "", "substring of the tab's URL (default: this binary's own port)")
	}
	tuiCmd.Flags().IntVar(&bayMon, "monitor", 0, "draw the bays with a chassis monitor this many slots wide")
	rackCmd.Flags().IntVar(&bayMon, "monitor", 0, "draw the bays again with a chassis monitor this many slots wide")
	rackCmd.Flags().IntVar(&rowSlots, "slots", 0, "slots per row (0 = ask the page)")
	ctlCmd.Flags().BoolVar(&ctlValues, "values", false, "read every control's current value too")
	runCmd.AddCommand(tuiCmd, ctlCmd, rackCmd)
}

// dialRack opens the cable, defaulting to a tab on this binary's own port.
func dialRack() *rackcable.Client {
	t := attachTo
	if t == "" {
		t = fmt.Sprintf("127.0.0.1:%d", webPort)
	}
	c, err := rackcable.Dial(cdpPort, t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chaosrack: %v\n", err)
		fmt.Fprintf(os.Stderr, "  (looking for a tab whose URL contains %q on CDP port %d)\n", t, cdpPort)
		os.Exit(1)
	}
	c.Monitor, c.Capacity = bayMon, rowSlots
	return c
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "drive the rack from a terminal",
	Long: `Control a running rack from a terminal.

The top of the screen shows the rack's bays and the bottom lists every
control. Changes take effect in the browser immediately.

  up/down        select a control (PgUp/PgDn jump 10, Home/End go to the ends)
  left/right     turn it one step, or move a switch one position
  ctrl+arrows    scroll the view
  0              reset the control to its default
  tab            switch between the panel and list views
  type           filter controls by name (backspace erases, / clears)
  r              reload the controls from the page
  q, esc         quit

Requires a rack open in a browser started with remote debugging:

  chaosrack &
  chromium --remote-debugging-port=9222 http://127.0.0.1:8080/
  chaosrack tui

By default it connects to the tab on this binary's --port. Use --attach to
choose a different tab and --cdp to use a different debugging port.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return racktui.Run(cmd.Context(), dialRack())
	},
}

var rackCmd = &cobra.Command{
	Use:   "rack",
	Short: "draw the rack's bays as text",
	Long: `Print the running rack's layout as text.

Each row of the rack is a fixed number of slots. Each module takes up a whole
number of slots, and blank panels fill the rest. The layout printed is the
one the page is currently using.

--slots sets the number of slots per row. By default the page decides.
--monitor N also prints the layout with an N-slot monitor at the start of
each bay, for comparison.

Like tui, this needs a rack open in a browser with remote debugging enabled
(see chaosrack tui --help).`,
	RunE: func(_ *cobra.Command, _ []string) error {
		c := dialRack()
		mon := c.Monitor
		c.Monitor = 0
		asIs, err := c.Rack()
		if err != nil {
			return err
		}
		fmt.Println("AS IT IS")
		fmt.Print(asIs)
		if mon > 0 {
			c.Monitor = mon
			withMon, err := c.Rack()
			if err != nil {
				return err
			}
			fmt.Printf("\nWITH A %d-SLOT MONITOR IN EVERY BAY\n", mon)
			fmt.Print(withMon)
		}
		return nil
	},
}

var ctlCmd = &cobra.Command{
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
page's permalink updates to match.

Like tui, this needs a rack open in a browser with remote debugging enabled
(see chaosrack tui --help).`,
	RunE: func(_ *cobra.Command, args []string) error {
		c := dialRack()
		if len(args) == 0 {
			return listControls(c)
		}
		for _, a := range args {
			id, val, isSet := strings.Cut(a, "=")
			if !isSet {
				v, err := c.Get(id)
				if err != nil {
					return err
				}
				fmt.Println(v)
				continue
			}
			if err := c.Set(id, val); err != nil {
				return err
			}
			fmt.Printf("%s = %s\n", id, val)
		}
		return nil
	},
}

func listControls(c *rackcable.Client) error {
	ctls, err := c.Controls()
	if err != nil {
		return err
	}
	fmt.Printf("%-24s %-14s %-12s %s\n", "ID", "LABEL", "VALUE", "RANGE")
	for _, x := range ctls {
		rng := strings.Join(x.Options, " ")
		if !x.IsSelect {
			rng = fmt.Sprintf("%g .. %g / %g", x.Min, x.Max, x.Step)
		}
		val := ""
		if ctlValues || x.IsSelect {
			val = x.Value
		}
		fmt.Printf("%-24s %-14s %-12s %s\n", x.ID, x.Label, val, rng)
	}
	fmt.Printf("\n%d controls\n", len(ctls))
	return nil
}
