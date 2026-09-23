// Subcommand ctl: drive the rack from the terminal.
//
// Every panel control is a hidden input with an id and a change listener, so
// turning a knob from outside has always been one assignment away — what was
// missing was the list. The rack now records its control surface as it wires
// it (ControlRegistry) and hands it out on window.rackctl, so this can ask
// what exists instead of being told.
//
//	uitool ctl                      # every control: id, label, range, value
//	uitool ctl -ctl-get zoom
//	uitool ctl -ctl-set zoom=3.2
//	uitool ctl -ctl-set wf-win=2,lufs-rate=1000
//
// It is a remote control and not a headless rack: the instrument is wasm in a
// page and this is the cable to it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	ctlGet = flag.String("ctl-get", "", "print one control's value")
	ctlSet = flag.String("ctl-set", "", "id=value pairs, comma separated")
	ctlAll = flag.Bool("ctl-values", false, "read every control's value too (a round trip each)")
)

func runCtl() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctl:", err)
		os.Exit(1)
	}
	if v, ok := c.Eval(`typeof rackctl`).(string); !ok || v != "object" {
		fmt.Fprintln(os.Stderr, "ctl: the page has no rackctl — is it running a build with the control registry?")
		os.Exit(1)
	}
	switch {
	case *ctlSet != "":
		for _, pair := range strings.Split(*ctlSet, ",") {
			id, val, found := strings.Cut(strings.TrimSpace(pair), "=")
			if !found {
				fmt.Fprintf(os.Stderr, "ctl: %q is not id=value\n", pair)
				os.Exit(2)
			}
			qid := strconv.Quote(id)
			qval := strconv.Quote(val)
			ok, _ := c.Eval(fmt.Sprintf(`rackctl.set(%s,%s)`, qid, qval)).(bool)
			if !ok {
				fmt.Fprintf(os.Stderr, "ctl: the rack has no control %q\n", id)
				os.Exit(1)
			}
			fmt.Printf("%s = %s\n", id, val)
		}
	case *ctlGet != "":
		qid := strconv.Quote(*ctlGet)
		v := c.Eval(fmt.Sprintf(`rackctl.get(%s)`, qid))
		if v == nil {
			fmt.Fprintf(os.Stderr, "ctl: the rack has no control %q\n", *ctlGet)
			os.Exit(1)
		}
		fmt.Println(v)
	default:
		listControls(c)
	}
}

func listControls(c *cdp.Client) {
	s, _ := c.Eval(`rackctl.list()`).(string)
	var ctls []attractor.ControlInfo
	if err := json.Unmarshal([]byte(s), &ctls); err != nil {
		fmt.Fprintln(os.Stderr, "ctl:", err)
		os.Exit(1)
	}
	fmt.Printf("%-22s %-14s %-22s %-8s %s\n", "ID", "LABEL", "RANGE", "PERMA", "VALUE")
	for _, x := range ctls {
		rng := "select"
		if !x.IsSelect {
			rng = fmt.Sprintf("%g .. %g / %g", x.Min, x.Max, x.Step)
		}
		val := ""
		if *ctlAll {
			qid := strconv.Quote(x.ID)
			if v := c.Eval(fmt.Sprintf(`rackctl.get(%s)`, qid)); v != nil {
				val = fmt.Sprint(v)
			}
		}
		fmt.Printf("%-22s %-14s %-22s %-8s %s\n", x.ID, x.Label, rng, x.PermaKey, val)
	}
	fmt.Printf("\n%d controls\n", len(ctls))
}
