package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/internal/cdp"
)

// Auditing the panel stylesheet.
//
// panel.css is a single 1400-line file that every module's layout comes out
// of, and it had grown two faults that no amount of reading catches:
//
//   - Rules that style nothing. Thirty of them, for features that had been
//     rewritten (the scope as a module rather than a unit) or never wired up
//     (the dock positions, whose only surviving mention was a comment saying
//     the stylesheet selected on them).
//
//   - Rules arguing with each other. A third of the declarations carry
//     !important, and the reason is a chain: .sect>.row says every module row
//     is display:flex, .vmrow has to shout display:grid!important to take it
//     back, and everything after that needs !important to beat THAT. Half the
//     flex properties still set on .vmrow do nothing at all, because it is a
//     grid.
//
// Neither is visible in a diff and both make every subsequent change need a
// browser to confirm. So they are measured here instead: run against the
// live panel, walk every model, and report what never matched and what is
// fighting.
//
// This is deliberately not a linter with a fixed rule list. The one thing it
// can say with authority is "no element in any mode matches this", and that
// takes a running page.

// cssSelUsed is how a selector is judged. Interaction pseudo-classes are
// stripped before matching, because :hover cannot be reproduced by walking
// modes, and a pseudo-ELEMENT cannot be matched by querySelectorAll at all.
var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssBlock   = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	// Only the pseudo-classes that cannot be reproduced by walking modes.
	// NOT :checked or :indeterminate — querySelectorAll matches those, and
	// they are how a switch's own styling is written.
	cssPseudo = regexp.MustCompile(`:(hover|active|focus|focus-visible|focus-within|disabled)\b`)
	// A keyframe stop ("0%", "50%") is a block with a brace, not a selector.
	cssStop = regexp.MustCompile(`^-?[0-9.]+%(\s*,\s*-?[0-9.]+%)*$`)
)

func runCSS() {
	const path = "pkg/attractor/panel.css"
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	txt := cssComment.ReplaceAllString(string(src), "")

	type rule struct{ sel, body string }
	var rules []rule
	for _, m := range cssBlock.FindAllStringSubmatch(txt, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") || cssStop.MatchString(sel) {
			continue
		}
		rules = append(rules, rule{sel, m[2]})
	}

	// ── What is fighting: !important, and properties set more than once for
	// the same selector. Both are pure text and need no browser.
	bang := strings.Count(txt, "!important")
	props := map[string]map[string]int{} // selector -> property -> times set
	for _, r := range rules {
		for _, s := range strings.Split(r.sel, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if props[s] == nil {
				props[s] = map[string]int{}
			}
			for _, d := range strings.Split(r.body, ";") {
				if i := strings.Index(d, ":"); i > 0 {
					props[s][strings.TrimSpace(d[:i])]++
				}
			}
		}
	}
	type clash struct {
		sel   string
		prop  string
		times int
	}
	var clashes []clash
	for s, ps := range props {
		for p, n := range ps {
			if n > 1 {
				clashes = append(clashes, clash{s, p, n})
			}
		}
	}
	sort.Slice(clashes, func(i, j int) bool {
		if clashes[i].times != clashes[j].times {
			return clashes[i].times > clashes[j].times
		}
		return clashes[i].sel < clashes[j].sel
	})

	fmt.Printf("%s: %d rules, %d !important (%d%% of rules)\n",
		path, len(rules), bang, 100*bang/max(1, len(rules)))

	if len(clashes) > 0 {
		fmt.Printf("\nSET MORE THAN ONCE FOR THE SAME SELECTOR (%d):\n", len(clashes))
		for i, c := range clashes {
			if i == 25 {
				fmt.Printf("  ... and %d more\n", len(clashes)-25)
				break
			}
			fmt.Printf("  %-44s %-22s x%d\n", c.sel, c.prop, c.times)
		}
	}

	// ── What styles nothing. Needs the live panel.
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n(no browser on :%d matching %q — skipping the dead-rule pass)\n", *cdpPort, *target)
		return
	}

	seen := map[string]bool{}
	var sels []string
	for _, r := range rules {
		for _, s := range strings.Split(r.sel, ",") {
			s = strings.TrimSpace(s)
			// A pseudo-element is invisible to querySelectorAll, so it can
			// never be judged this way and must not be reported as dead.
			if s == "" || strings.Contains(s, "::") || cssStop.MatchString(s) {
				continue
			}
			// Strip only when there is no functional pseudo to break: taking
			// :checked out of :not(:checked) leaves an empty :not(), which is
			// a selector error reported against a stylesheet that was fine.
			t := s
			if !strings.Contains(s, "(") {
				t = cssPseudo.ReplaceAllString(s, "")
			}
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			sels = append(sels, t)
		}
	}
	js, err := json.Marshal(sels)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode selectors:", err)
		return
	}
	probe := `(sels=>{window.__hit=window.__hit||{};for(const s of sels){if(window.__hit[s])continue;
 try{ if(document.querySelectorAll(s).length) window.__hit[s]=1; }catch(e){ window.__hit[s]='BAD'; }}})(` + string(js) + `)`

	c.Eval(probe)
	n := 0
	if v, ok := c.Eval(`(()=>{const s=document.getElementById('mode-select');return s?s.options.length:0;})()`).(float64); ok {
		n = int(v)
	}
	for i := 0; i < n; i++ {
		c.Eval(fmt.Sprintf(`(()=>{const s=document.getElementById('mode-select');if(s&&s.options[%d]){s.selectedIndex=%d;s.dispatchEvent(new Event('change',{bubbles:true}));}})()`, i, i))
		time.Sleep(90 * time.Millisecond)
		if i%7 == 0 {
			c.Eval(probe)
		}
	}
	c.Eval(probe)

	hit := map[string]any{}
	raw := fmt.Sprintf("%v", c.Eval(`JSON.stringify(window.__hit)`))
	if err := json.Unmarshal([]byte(raw), &hit); err != nil {
		// Without the tally there is nothing to say about dead rules, and
		// saying it anyway would report every selector as dead.
		fmt.Fprintln(os.Stderr, "read match tally:", err)
		return
	}

	var dead, bad []string
	for _, s := range sels {
		switch hit[s] {
		case nil:
			dead = append(dead, s)
		case "BAD":
			bad = append(bad, s)
		}
	}
	fmt.Printf("\nSELECTORS across %d modes: %d tested, %d matched, %d never matched, %d invalid\n",
		n, len(sels), len(sels)-len(dead)-len(bad), len(dead), len(bad))
	for _, s := range bad {
		fmt.Println("  INVALID:", s)
	}
	if len(dead) > 0 {
		fmt.Println("\nNEVER MATCHED — check each against the Go before deleting: a state")
		fmt.Println("class the walk never entered (a drag, a knob style, a dock position)")
		fmt.Println("looks exactly like a dead one here.")
		for _, s := range dead {
			fmt.Println("  ", s)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
