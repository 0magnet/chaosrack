// Command uitool bundles the CDP-driven UI test harnesses as subcommands of
// one binary (they share internal/cdp and most flags):
//
//	uitool monkey [flags]   random-walk invariant fuzzer (see monkey.go)
//	uitool golden [flags]   known-state oracle + golden images (see golden.go)
//	uitool shots  [flags]   README gallery capture (see shots.go)
//	uitool gifs   [flags]   animated README gallery capture (see gifs.go)
//	uitool demo   [flags]   chaos-monkey demo-reel recorder (see demo.go)
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
	case "shots":
		runShots()
	case "gifs":
		runGifs()
	case "demo":
		runDemo()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: uitool <monkey|golden|shots|gifs|demo> [flags]   (uitool <sub> -h for flags)")
	os.Exit(2)
}
