// Subcommand rack: draw the rack's bays as text, from the live panel.
//
// A rack of 84 HP rows IS a table — a row is a fixed number of slots, a module
// occupies a whole number of them, and the rest is blank panel — so it can be
// drawn exactly rather than described. What comes off the page is only what
// has to be measured: each module's name and how many slots it takes at the
// interface scale in use. The bays are then packed by packBySection and drawn
// by the same code the rack's own layout uses, so the picture cannot disagree
// with the instrument.
//
//	uitool rack                    # the rack as it is now
//	uitool rack -rack-mon 4        # and again with a 4-slot monitor per bay
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/0magnet/chaosrack/internal/cdp"
	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	rackMon = flag.Int("rack-mon", 0, "draw again with a chassis monitor this many slots wide")
	rackCap = flag.Int("rack-cap", 0, "slots per row (0 = ask the page)")
)

// readRack is evaluated in the page. Slots are derived the way moduleSlots
// does it, from the module's measured width against the slot pitch.
const readRack = `(function(){
  var scale = parseFloat(getComputedStyle(document.documentElement)
      .getPropertyValue('--kscale')) || 1;
  var slot = 140.24, gap = 2.0;               // moduleSlot / moduleGap at scale 1
  var pitch = (slot + gap) * scale;
  var out = [];
  document.querySelectorAll('.sect').forEach(function(m){
    var hdr = m.querySelector('.sect-hdr');
    if (!hdr) return;
    var w = m.offsetWidth;
    var n = w > 0 ? Math.max(1, Math.round((w + gap*scale)/pitch)) : 0;
    if (w === 0 && m.style.display === 'none') n = 0;
    out.push({k: hdr.textContent.trim().toLowerCase(), s: n,
             c: m.getAttribute('data-cat') || ''});
  });
  return JSON.stringify(out);
})()`

func runRack() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rack:", err)
		os.Exit(1)
	}
	plain, err := drawLiveRack(c, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rack:", err)
		os.Exit(1)
	}
	fmt.Println("AS IT IS")
	fmt.Print(plain)
	if *rackMon > 0 {
		withMon, err := drawLiveRack(c, *rackMon)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rack:", err)
			os.Exit(1)
		}
		fmt.Printf("\nWITH A %d-SLOT MONITOR IN EVERY BAY\n", *rackMon)
		fmt.Print(withMon)
	}
}

// drawLiveRack reads the panel and draws its bays. Shared by `uitool rack`,
// which prints it, and `uitool tui`, which puts it above the controls.
//
// What comes off the page is only what has to be MEASURED — a module's name,
// how many slots it takes at the interface scale in use, and the category a
// model card belongs to. The sections and the bays are then computed by the
// same code the rack itself runs, so the drawing cannot disagree with it.
func drawLiveRack(c *cdp.Client, mon int) (string, error) {
	s, _ := c.Eval(readRack).(string)
	var mods []struct {
		K string `json:"k"`
		S int    `json:"s"`
		C string `json:"c"`
	}
	if err := json.Unmarshal([]byte(s), &mods); err != nil {
		return "", fmt.Errorf("reading the panel: %w", err)
	}
	if len(mods) == 0 {
		return "", errors.New("the page reported no modules")
	}
	keys := make([]string, len(mods))
	cats := make([]string, len(mods))
	slots := make([]int, len(mods))
	for i, m := range mods {
		keys[i], slots[i], cats[i] = m.K, m.S, m.C
	}
	capacity := rackCapacity(c)
	var monitors map[string]int
	if mon > 0 {
		monitors = map[string]int{}
		for i, k := range keys {
			monitors[attractor.DrawSectionOf(k, cats, i)] = mon
		}
	}
	return attractor.DrawRackFrom(keys, cats, slots, capacity, monitors), nil
}

// rackCapacity is how many slots a row holds, asked of the page because it
// depends on the interface scale in use.
func rackCapacity(c *cdp.Client) int {
	if *rackCap > 0 {
		return *rackCap
	}
	if v, ok := c.Eval(`(function(){
	  var f=document.querySelector('.rack-frame');
	  var scale=parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--kscale'))||1;
	  return f ? Math.max(1, Math.round(f.clientWidth/((140.24+2)*scale))) : 0})()`).(float64); ok && v > 0 {
		return int(v)
	}
	return 12
}
