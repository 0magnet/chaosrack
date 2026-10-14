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

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/0magnet/chaosrack/internal/rackcable"
	"github.com/0magnet/chaosrack/pkg/hostaudio"
	"github.com/0magnet/chaosrack/pkg/rackhost"
	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/racktui"
	"github.com/0magnet/chaosrack/pkg/subcmds"
)

var (
	cdpPort   int
	attachTo  string
	bayMon    int
	tuiVia    string
	tuiServer string
)

// dialRack opens the cable, defaulting to a tab on this binary's own port.
func dialRack() (*rackcable.Client, error) {
	t := attachTo
	if t == "" {
		t = fmt.Sprintf("127.0.0.1:%d", webPort)
	}
	c, err := rackcable.Dial(cdpPort, t)
	if err != nil {
		return nil, fmt.Errorf("%w (looking for a tab whose URL contains %q on CDP port %d)", err, t, cdpPort)
	}
	c.Monitor = bayMon
	return c, nil
}

// RackDialer is how ctl and rack reach the rack from a host: a tab, over the
// browser's debugging port.
func RackDialer() subcmds.Dialer {
	return subcmds.Dialer{
		Flags: func(fs *pflag.FlagSet) {
			fs.IntVar(&cdpPort, "cdp", 9222, "the browser's remote-debugging port")
			fs.StringVar(&attachTo, "attach", "", "substring of the tab's URL (default: 127.0.0.1:8080)")
		},
		Dial: dialRack,
	}
}

// PanelFlags puts the flags that say where the rack is on the panel's
// command, which is chaosrack itself.
func PanelFlags(fs *pflag.FlagSet) {
	fs.StringVar(&tuiVia, "via", "", "drive a rack in a page instead of running one: auto (WebTransport, else a WebSocket), wt, ws, or cdp (a browser debugging port)")
	fs.StringVar(&tuiServer, "server", "", "the chaosrack server whose page to drive (default: http://127.0.0.1:8080)")
	fs.IntVar(&cdpPort, "cdp", 9222, "with --via cdp: the browser's remote-debugging port")
	fs.StringVar(&attachTo, "attach", "", "with --via cdp: substring of the tab's URL (default: 127.0.0.1:8080)")
	fs.IntVar(&bayMon, "monitor", 0, "draw the bays with a chassis monitor this many slots wide")
	fs.StringVar(&pprofAddr, "pprof", "", "serve Go's profiler at this address while the panel runs (e.g. 127.0.0.1:6061; go tool pprof http://ADDR/debug/pprof/profile)")
	fs.StringVar(&audioSource, "audio-source", "monitor", "what the rack's capture records, once the Mixer's CAP pins put it in an audio model: monitor (what is playing), default (the input), or a source ID")
}

// PanelHelp is how the panel reaches the rack on a host, for its help.
const PanelHelp = `On this machine chaosrack runs the rack itself: the panel as the page draws
it, its values held here, and the model drawn in the terminal. No browser is
needed.

To drive a rack running in a page instead, say how to reach it: --via auto
(WebTransport, else a WebSocket, through the page's own server), wt, ws, or
cdp (a browser's debugging port; --attach chooses the tab and --cdp the
port). --server alone means auto, and --attach alone cdp.

  chaosrack serve &
  xdg-open http://127.0.0.1:8080/
  chaosrack --via auto`

// remote reports whether the panel is to drive a page rather than run the
// rack itself: any of the flags that say where a page is.
func remote() bool { return tuiVia != "" || tuiServer != "" || attachTo != "" }

// Panel runs the panel on this terminal: the rack itself, or with --via, a
// rack running in a page.
func Panel(ctx context.Context) error {
	servePprof()
	if !remote() {
		r, err := rackhost.New()
		if err != nil {
			return err
		}
		r.SetAudioSource(audioSource)
		// The rack is heard on this machine's speakers once a generator is
		// switched on, and is silent again when the panel goes.
		sp := hostaudio.New(r.Fill, r.SampleRate())
		defer sp.Close()
		r.SetSpeaker(sp)
		return racktui.Run(ctx, r)
	}
	if tuiVia == "" {
		tuiVia = racklink.ViaAuto
		if attachTo != "" && tuiServer == "" {
			tuiVia = "cdp"
		}
	}
	if tuiVia == "cdp" {
		c, err := dialRack()
		if err != nil {
			return err
		}
		defer racktui.PowerOn(c)()
		return racktui.Run(ctx, c)
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
	defer racktui.PowerOn(c)()
	return racktui.Run(ctx, c)
}

// Serve is `chaosrack serve`.
func Serve() *cobra.Command { return runCmd }
