// Command uitool bundles the CDP-driven UI test harnesses as subcommands of
// one binary (they share internal/cdp and most flags):
//
//	uitool monkey [flags]   random-walk invariant fuzzer (see monkey.go)
//	uitool golden [flags]   known-state oracle + golden images (see golden.go)
//	uitool shots  [flags]   README gallery capture (see shots.go)
//	uitool gifs   [flags]   animated README gallery capture (see gifs.go)
//	uitool portraits [flags] per-model stills + loops for the README (see portraits.go)
//	uitool readme [flags]   regenerate the README's generated regions (see readme.go)
//	uitool demo   [flags]   chaos-monkey demo-reel recorder (see demo.go)
//	uitool layout [flags]   control-panel geometry invariants (see layout.go)
//	uitool spec   [flags]   WAV → spectrogram PNG, and PNG diff (see spec.go)
//
// Both talk to an already-open tab in a Chromium/Brave started with
// --remote-debugging-port (default 9222).
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	cdpPort = flag.Int("port", 9222, "Chrome DevTools remote-debugging port")
	target  = flag.String("target", "127.0.0.1:8300", "substring of the target tab's URL")
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	sub := os.Args[1]
	if err := flag.CommandLine.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	switch sub {
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
	case "readme":
		runReadme()
	case "modules":
		runModules()
	case "demo":
		runDemo()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: uitool <monkey|golden|shots|gifs|portraits|modules|readme|demo> [flags]   (uitool <sub> -h for flags)")
	os.Exit(2)
}
