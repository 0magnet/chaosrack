// Package uitool bundles the CDP-driven UI test harnesses as subcommands of
// `chaosrack uitool` (they share github.com/0magnet/cdp and most flags):
//
//	chaosrack uitool monkey [flags]   random-walk invariant fuzzer (see monkey.go)
//	chaosrack uitool golden [flags]   known-state oracle + golden images (see golden.go)
//	chaosrack uitool shots  [flags]   README gallery capture (see shots.go)
//	chaosrack uitool gifs   [flags]   animated README gallery capture (see gifs.go)
//	chaosrack uitool portraits [flags] per-model stills + loops for the README (see portraits.go)
//	chaosrack uitool readme [flags]   regenerate the README's generated regions (see readme.go)
//	chaosrack uitool site   [flags]   per-model HTML pages + sitemap (see site.go)
//	chaosrack uitool demo   [flags]   chaos-monkey demo-reel recorder (see demo.go)
//	chaosrack uitool layout [flags]   control-panel geometry invariants (see layout.go)
//	chaosrack uitool sweep  [flags]   every model and switch in a fixed order, with invariants (see sweep.go)
//	chaosrack uitool lint   [flags]   panel faults found by measurement, named by address (see lint.go)
//	chaosrack uitool spec   [flags]   WAV → spectrogram PNG, and PNG diff (see spec.go)
//
// They talk to an already-open tab in a Chromium/Brave started with
// --remote-debugging-port (default 9222), or with --headless (and --serve) to a
// private headless one of their own that nothing sees (see headless.go).
package uitool

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// fs holds every flag of the harnesses. The subcommands share one set (a flag
// a subcommand does not read is ignored), which Command exposes as cobra flags.
var fs = flag.NewFlagSet("uitool", flag.ContinueOnError)

var (
	cdpPort = fs.Int("port", 9222, "Chrome DevTools remote-debugging port")
	target  = fs.String("target", "127.0.0.1:8300", "substring of the target tab's URL")
)

// subcommands names what Command accepts as its one argument.
var subcommands = []string{"css", "lint", "monkey", "sweep", "golden", "layout", "spec", "shots", "gifs", "portraits", "site", "readme", "modules", "html", "demo"}

// Command is `chaosrack uitool <sub> [flags]`.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "uitool <css|lint|monkey|sweep|golden|layout|spec|shots|gifs|portraits|site|readme|modules|html|demo>",
		Short:     "CDP-driven UI test, fuzz and capture harnesses",
		Long:      "The CDP-driven UI test, fuzz and capture harnesses. Run `chaosrack uitool <sub> --help` for the flags (all subcommands share one set).",
		Args:      cobra.ExactArgs(1),
		ValidArgs: subcommands,
		Run: func(_ *cobra.Command, args []string) {
			runSub(args[0])
		},
	}
	cmd.Flags().AddGoFlagSet(fs)
	return cmd
}

func runSub(sub string) {
	if err := setUp(); err != nil {
		fmt.Fprintln(os.Stderr, "uitool:", err)
		exit(1)
	}
	switch sub {
	case "lint":
		runLint()
	case "sweep":
		runSweep()
	case "monkey":
		runMonkey()
	case "golden":
		runGolden()
	case "layout":
		runLayout()
	case "spec":
		runSpec()
	case "shots":
		runShots()
	case "gifs":
		runGifs()
	case "portraits":
		runPortraits()
	case "site":
		runSite()
	case "readme":
		runReadme()
	case "modules":
		runModules()
	case "html":
		runHTML()
	case "css":
		runCSS()
	case "demo":
		runDemo()
	default:
		fmt.Fprintln(os.Stderr, "usage: chaosrack uitool <css|lint|monkey|sweep|golden|layout|spec|shots|gifs|portraits|site|readme|modules|html|demo> [flags]")
		os.Exit(2)
	}
	exit(0)
}
