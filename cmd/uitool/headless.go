package main

// -serve and -headless: a rack and a browser of the harness's own.
//
// Every subcommand drives a tab in a browser someone already opened, which is
// right for watching a run and wrong for everything else. cdp.Dial brings the
// tab to the front so its timers are not throttled, and on a desktop that
// means the fuzzer's window jumps over whatever its owner is working in, once
// per attach. It also shares that browser's localStorage, so every run starts
// from wherever the last session left the rack.
//
// -headless starts a private headless Brave (or Chrome) with a throwaway
// profile on a free debugging port and opens the target in it: nothing
// appears on screen, and every run starts from a factory rack. -serve BIN runs
// a chaosrack server of its own on a free port for it to open, so a run needs
// nothing running beforehand. Both are torn down when the subcommand exits.
//
// The page is opened with ?arenacheck, which turns on pkg/dom's released-
// listener check: a leak is then an error on the page, which every harness
// already fails on.
//
// Headless draws with SwiftShader, on the CPU. That is fine for what these
// harnesses check — exceptions, freezes, leaks, layout — and wrong for judging
// how anything looks.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/0magnet/cdp"
)

var (
	headless = flag.Bool("headless", false, "run in a private headless browser instead of attaching to one (nothing on screen, fresh storage)")
	serveBin = flag.String("serve", "", "start this chaosrack binary on a free port and test it, rather than a server already running at -target")
	browser  = flag.String("browser", "", "browser for -headless (default: $CHROME, then brave, chromium, google-chrome)")
)

// atExit runs, last first, when the subcommand ends — through exit, which the
// harnesses call instead of os.Exit so that a failing run still stops the
// server and the browser it started.
var atExit []func()

func exit(code int) {
	for i := len(atExit) - 1; i >= 0; i-- {
		atExit[i]()
	}
	os.Exit(code)
}

// setUp starts whatever -serve and -headless ask for and points -port and
// -target at it.
func setUp() error {
	if *serveBin != "" {
		if err := startServer(*serveBin); err != nil {
			return err
		}
	}
	if *headless {
		return startBrowser()
	}
	return nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close() //nolint:errcheck // only borrowed for its number
	return l.Addr().(*net.TCPAddr).Port, nil
}

// child starts a process that is stopped when this one exits — and, on
// Linux, even when this one is killed instead (see proc_linux.go).
func child(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // the harness's own server and browser
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	atExit = append(atExit, func() {
		killGroup(cmd)
		_ = cmd.Wait() //nolint:errcheck // killed on purpose
	})
	return nil
}

func startServer(bin string) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	if err := child(bin, "--port", strconv.Itoa(port)); err != nil {
		return fmt.Errorf("-serve: %w", err)
	}
	addr := "127.0.0.1:" + strconv.Itoa(port)
	for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close() //nolint:errcheck // only asking whether it listens
			break
		}
		if time.Now().After(deadline) {
			return errors.New("-serve: the server never listened on " + addr)
		}
	}
	*target = addr
	return nil
}

func findBrowser() (string, error) {
	for _, b := range []string{*browser, os.Getenv("CHROME"), "brave", "chromium", "google-chrome"} {
		if b == "" {
			continue
		}
		if p, err := exec.LookPath(b); err == nil {
			return p, nil
		}
	}
	return "", errors.New("-headless: no Chromium-family browser found (set -browser or $CHROME)")
}

func startBrowser() error {
	bin, err := findBrowser()
	if err != nil {
		return err
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	profile, err := os.MkdirTemp("", "uitool-profile-")
	if err != nil {
		return err
	}
	atExit = append(atExit, func() { _ = os.RemoveAll(profile) }) //nolint:errcheck // a temp dir
	url := "http://" + *target + "/?arenacheck"
	if err := child(bin,
		"--headless=new",
		"--remote-debugging-port="+strconv.Itoa(port),
		"--user-data-dir="+profile,
		"--window-size=1600,1000",
		"--no-first-run", "--no-default-browser-check",
		"--disable-background-networking",
		"--mute-audio", "--autoplay-policy=no-user-gesture-required",
		"--enable-unsafe-swiftshader", "--use-angle=swiftshader",
		url,
	); err != nil {
		return fmt.Errorf("-headless: %w", err)
	}
	*cdpPort = port

	// Wait for the tab, then for the rack: a harness that starts on a page
	// still booting finds no controls and passes, having tested nothing.
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := cdp.Local(port).Find(ctx, *target)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("-headless: no tab for %s: %w", url, err)
		}
	}
	c, err := cdp.Dial(port, *target)
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck // only used to wait
	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(250 * time.Millisecond) {
		if ok, _ := c.Eval(`!!document.querySelector('#controls-panel .sect')`).(bool); ok {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("-headless: the rack never built in " + url)
		}
	}
}

// dial attaches to the target tab. Headless, the page draws on the CPU, and on
// a busy machine one frame can outlast cdp's ten-second default — which the
// harnesses read as a frozen main thread. A minute still catches a real hang.
func dial() (*cdp.Client, error) {
	c, err := cdp.Dial(*cdpPort, *target)
	if err == nil && *headless {
		c.Timeout = time.Minute
	}
	return c, err
}
