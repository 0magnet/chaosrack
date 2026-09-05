//go:build js && wasm

package deskapp

import (
	"context"
	"strconv"

	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"

	"github.com/0magnet/lolcat-go/lol"
	"github.com/0magnet/toilet-go/toilet"
)

// RegisterApplets puts toilet and lolcat into the shell.
//
// They belong here rather than in either port's own repository. Each of those
// ports one C program and should demonstrate that one thing; the pipeline
// people actually type joins two of them, and a shell is the only place a
// pipeline means anything. tuiwasm already hosts other repositories' terminal
// programs — termanim's animations are registered here the same way — so this
// is where the join goes.
//
// Both read stdin and write stdout, so `toilet hi | lolcat` is a real
// pipeline through websh's interpreter, not a special case wired up for a
// screenshot.
func RegisterApplets() {
	shell.RegisterApplet("toilet", "big text — a port of TOIlet (try: toilet -f smblock hi)", runToilet)
	shell.RegisterApplet("lolcat", "rainbow-color stdin — a port of lolcat", runLolcat)
}

func runToilet(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
	cx := toilet.New()
	var words []string

	// A deliberately small slice of the CLI: the flags worth reaching for at
	// a prompt. The full option set lives in the binary, and the README of
	// the port says so rather than this pretending to be it.
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-f", "--font":
			if i++; i < len(args) {
				cx.Font = args[i]
			}
		case "-F", "--filter":
			if i++; i < len(args) {
				if err := cx.AddFilter(args[i]); err != nil {
					return fail(hc, "toilet: "+err.Error())
				}
			}
		case "-w", "--width":
			if i++; i < len(args) {
				cx.TermWidth, _ = strconv.Atoi(args[i]) //nolint:errcheck // atoi semantics: junk is 0
			}
		case "-k", "--kern":
			cx.Mode = "kern"
		case "-W", "--wide":
			cx.Mode = "none"
		case "-S", "--smush":
			cx.Mode = "smush"
		case "-o", "--overlap":
			cx.Mode = "overlap"
		case "--fonts":
			return listFonts(hc)
		default:
			// An unrecognized flag is refused rather than rendered. Silently
			// treating -X as text would print it in big letters and look like
			// the flag had worked.
			if len(a) > 1 && a[0] == '-' {
				return unknownFlag(hc, "toilet", a)
			}
			words = append(words, a)
		}
	}

	if err := cx.Init(); err != nil {
		return fail(hc, "toilet: "+err.Error())
	}
	if err := cx.Render(words, hc.Stdin, hc.Stdout); err != nil {
		return fail(hc, "toilet: "+err.Error())
	}
	return 0
}

func runLolcat(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
	opts := lol.DefaultOptions()
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-p", "--spread":
			if i++; i < len(args) {
				opts.Spread, _ = strconv.ParseFloat(args[i], 64) //nolint:errcheck
			}
		case "-F", "--freq":
			if i++; i < len(args) {
				opts.Freq, _ = strconv.ParseFloat(args[i], 64) //nolint:errcheck
			}
		case "-S", "--seed":
			if i++; i < len(args) {
				opts.Seed, _ = strconv.Atoi(args[i]) //nolint:errcheck
				opts.OS = float64(opts.Seed)
			}
		case "-i", "--invert":
			opts.Invert = true
		default:
			// -a/--animate in particular: it never returns, and an applet that
			// never returns holds the terminal until Ctrl+C. Refusing beats
			// accepting a flag and not honoring it.
			if len(a) > 1 && a[0] == '-' {
				return unknownFlag(hc, "lolcat", a)
			}
		}
	}

	// The terminal is truecolor, so say so rather than sniffing an
	// environment variable a browser has no reason to set.
	c := &lol.Cat{Opts: opts, Out: hc.Stdout, TTY: true}
	c.SetMode("truecolor")
	if err := c.Cat(hc.Stdin); err != nil {
		return fail(hc, "lolcat: "+err.Error())
	}
	return 0
}

func listFonts(hc *interp.HandlerContext) int {
	cx := toilet.New()
	for _, f := range cx.FontFile() {
		if _, err := hc.Stdout.Write([]byte(f + "\n")); err != nil {
			return 1
		}
	}
	return 0
}

// unknownFlag refuses a flag this applet does not implement, and says where
// the rest of them live. These are a slice of each port's CLI, not the whole
// of it, and a shell that quietly ignores half a command line is worse than
// one that says so.
func unknownFlag(hc *interp.HandlerContext, cmd, flag string) int {
	return fail(hc, cmd+": unknown option "+flag+
		"\n"+cmd+": this is a subset for the shell; the full CLI is in github.com/0magnet/"+cmd+"-go")
}

func fail(hc *interp.HandlerContext, msg string) int {
	hc.Stderr.Write([]byte(msg + "\n")) //nolint:errcheck // a closed pipe is the caller's business
	return 1
}
