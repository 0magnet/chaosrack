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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/0magnet/chaosrack/internal/rackcable"
	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

var (
	cdpPort   int
	attachTo  string
	bayMon    int
	rowSlots  int
	ctlValues bool
	tuiVia    string
	tuiServer string
)

func init() {
	for _, c := range []*cobra.Command{ctlCmd, rackCmd} {
		c.Flags().IntVar(&cdpPort, "cdp", 9222, "the browser's remote-debugging port")
		c.Flags().StringVar(&attachTo, "attach", "", "substring of the tab's URL (default: this binary's own port)")
	}
	rackCmd.Flags().IntVar(&bayMon, "monitor", 0, "draw the bays again with a chassis monitor this many slots wide")
	rackCmd.Flags().IntVar(&rowSlots, "slots", 0, "slots per row (0 = ask the page)")
	ctlCmd.Flags().BoolVar(&ctlValues, "values", false, "read every control's current value too")
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

// PanelFlags puts the flags that say where the rack is on the panel's
// command, which is chaosrack itself.
func PanelFlags(fs *pflag.FlagSet) {
	fs.StringVar(&tuiVia, "via", racklink.ViaAuto, "how to reach the rack: auto (WebTransport, else a WebSocket), wt, ws, or cdp (a browser debugging port)")
	fs.StringVar(&tuiServer, "server", "", "the chaosrack server whose page to drive (default: http://127.0.0.1:8080)")
	fs.IntVar(&cdpPort, "cdp", 9222, "with --via cdp: the browser's remote-debugging port")
	fs.StringVar(&attachTo, "attach", "", "with --via cdp: substring of the tab's URL (default: 127.0.0.1:8080)")
	fs.IntVar(&bayMon, "monitor", 0, "draw the bays with a chassis monitor this many slots wide")
}

// PanelHelp is how the panel reaches the rack on a host, for its help.
const PanelHelp = `On this machine the rack is a page in a browser, served by chaosrack serve.
The page offers the rack to its own server and the panel reaches it there:
over WebTransport when it can, otherwise a WebSocket (--via). No browser
debugging port is needed.

  chaosrack serve &
  xdg-open http://127.0.0.1:8080/
  chaosrack

With several pages open, the one opened last answers. --via cdp reaches the
page over a browser's debugging port instead: --attach chooses the tab and
--cdp the port.`

// Panel runs the panel on this terminal, driving a rack in a browser.
func Panel(ctx context.Context) error {
	if tuiVia == "cdp" {
		return racktui.Run(ctx, dialRack())
	}
	switch tuiVia {
	case racklink.ViaAuto, racklink.ViaWT, racklink.ViaWS:
	default:
		return fmt.Errorf("--via %q: want auto, wt, ws or cdp", tuiVia)
	}
	server := tuiServer
	if server == "" {
		server = fmt.Sprintf("http://127.0.0.1:%d", webPort)
	}
	c, err := racklink.Dial(ctx, server, tuiVia)
	if err != nil {
		return fmt.Errorf("reaching the rack at %s: %w (is chaosrack serve running, with its page open?)", server, err)
	}
	defer c.Close() //nolint:errcheck // on the way out
	return racktui.Run(ctx, c)
}

// Commands are chaosrack's host subcommands: serve, and the ones that drive
// or draw the rack without being its panel.
func Commands() []*cobra.Command {
	return []*cobra.Command{runCmd, ctlCmd, rackCmd, renderCmd, modelsCmd}
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

Like the panel, this needs a rack open in a browser with remote debugging enabled
(see chaosrack --help).`,
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

Like the panel, this needs a rack open in a browser with remote debugging enabled
(see chaosrack --help).`,
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
