// Subcommand tui: the rack's control surface in a terminal.
//
// The same model the browser panel draws — the bays packBySection packs and
// the controls adoptDescControl records — rendered by pkg/racktui instead of
// by the DOM. This file is only the cable: it reads the rack over CDP and
// sends changes back, and knows nothing about how any of it is drawn.
//
//	uitool tui                      # drive the running rack from a terminal
//	uitool tui -rack-mon 3          # draw the bays with a chassis monitor
//
// ↑↓ moves, ←→ turns the control under the cursor — one step for a dial, one
// detent for a switch — 0 resets it to the default the registry knows, r
// reloads, typing filters, q quits.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// liveRack is a Source backed by a running page.
type liveRack struct {
	c   *cdp.Client
	mon int
}

// readValues asks for every control's current value and, for a selector, the
// detents it has — in ONE crossing, because the alternative is a round trip
// per control and there are seventy-two of them.
const readValues = `(function(){
  return JSON.stringify(JSON.parse(rackctl.list()).map(function(c){
    var el = document.getElementById(c.id);
    var opts = null;
    if (el && el.tagName === 'SELECT') {
      opts = [].slice.call(el.options).map(function(o){ return o.value; });
    }
    return {id: c.id, v: el ? String(el.value) : '', o: opts};
  }));
})()`

func (r liveRack) Controls() ([]racktui.Control, error) {
	s, _ := r.c.Eval(`rackctl.list()`).(string)
	var info []attractor.ControlInfo
	if err := json.Unmarshal([]byte(s), &info); err != nil {
		return nil, fmt.Errorf("reading the control registry: %w", err)
	}
	vs, _ := r.c.Eval(readValues).(string)
	var vals []struct {
		ID string   `json:"id"`
		V  string   `json:"v"`
		O  []string `json:"o"`
	}
	if err := json.Unmarshal([]byte(vs), &vals); err != nil {
		return nil, fmt.Errorf("reading the control values: %w", err)
	}
	byID := make(map[string]int, len(vals))
	for i, v := range vals {
		byID[v.ID] = i
	}
	out := make([]racktui.Control, 0, len(info))
	for _, in := range info {
		c := racktui.Control{ControlInfo: in}
		if i, ok := byID[in.ID]; ok {
			c.Value, c.Options = vals[i].V, vals[i].O
		}
		out = append(out, c)
	}
	return out, nil
}

func (r liveRack) Set(id, value string) error {
	ok, _ := r.c.Eval(fmt.Sprintf(`rackctl.set(%s,%s)`, strconv.Quote(id), strconv.Quote(value))).(bool)
	if !ok {
		return fmt.Errorf("the rack has no control %q", id)
	}
	return nil
}

func (r liveRack) Rack() (string, error) { return drawLiveRack(r.c, r.mon) }

func runTUI() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
	if v, ok := c.Eval(`typeof rackctl`).(string); !ok || v != "object" {
		fmt.Fprintln(os.Stderr, "tui: the page has no rackctl — is it running a build with the control registry?")
		os.Exit(1)
	}
	// No screen in the context: a terminal is exactly where tcell.NewScreen
	// is the right answer. A caller inside a page passes one instead.
	if err := racktui.Run(context.Background(), liveRack{c: c, mon: *rackMon}); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}
