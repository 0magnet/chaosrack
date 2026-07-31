// Subcommand golden: the correctness (oracle) counterpart to the monkey. Where
// the monkey fuzzes and only catches "the app broke", uigolden drives a fixed
// matrix of KNOWN states (mode + canonical settings, via permalink) and checks
// that each is actually RIGHT:
//
//   - render sanity: a non-audio model mode draws SOMETHING, and its drawn
//     extent is neither collapsed to a dot nor blown up to fill the screen
//   - permalink round-trip: restoring a state and re-serializing it is
//     idempotent (every setting survives save→restore)
//   - golden image: the render matches a validated reference screenshot
//     (regression), within a tolerance that absorbs attractor-trajectory jitter
//
// Run it against an open tab (Chromium/Brave with --remote-debugging-port). It
// hides the control panel before each screenshot so the golden is the model
// only.
//
//	uitool golden -capture   # write/refresh reference images (validate them by eye!)
//	uitool golden            # check current renders + round-trip against references
//
// Goldens live in cmd/uigolden/testdata/goldens and are committed once a human
// has confirmed they look correct — the oracle is only as good as the golden.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/wasm-stuff/internal/cdp"
)

var (
	capture  = flag.Bool("capture", false, "write/refresh golden images instead of checking")
	goldTol  = flag.Float64("tol", 0.10, "golden mismatch threshold (fraction of pixels differing)")
	settleMs = flag.Int("settle", 2200, "ms to let a state settle/fill before screenshotting")
	goldDir  = flag.String("goldens", "cmd/uitool/testdata/goldens", "golden image directory")
)

// state is one canonical UI state, addressed by its permalink hash.
type state struct {
	name    string
	hash    string
	blankOK bool    // audio modes render blank without a source → skip the blank oracle
	noGold  bool    // don't golden-diff (non-deterministic / audio) — still round-trip
	tol     float64 // per-state golden tolerance override (0 ⇒ the -tol flag)
}

var states = []state{
	// Lorenz gets a wider tolerance: the pose is pinned, but the screenshot
	// catches a moving 20k-point trail window whose phase depends on frame
	// timing, and the two-lobe orbit self-overlaps less than the dense
	// attractors — so run-to-run jitter sits right at the default 10%.
	{name: "lorenz", hash: "#lorenz&ar=0&rot=25,40,0", tol: 0.18},
	{name: "aizawa", hash: "#aizawa&ar=0&rot=25,40,0"},
	{name: "thomas", hash: "#thomas&ar=0&rot=25,40,0"},
	{name: "rossler", hash: "#rossler&ar=0&rot=25,40,0"},
	{name: "cube", hash: "#cube&ar=0&rot=25,40,0"},
	{name: "torus", hash: "#torus&ar=0&rot=25,40,0"},
	{name: "magnetosphere", hash: "#magnetosphere&ar=0&rot=25,40,0"},
	{name: "lissajou", hash: "#lissajou&ar=0&rot=25,40,0"},
	{name: "spectrogram", hash: "#spectrogram&ar=0", blankOK: true, noGold: true},
	{name: "xy", hash: "#xy&ar=0", blankOK: true, noGold: true},
	{name: "fvf", hash: "#fvf&ar=0", blankOK: true, noGold: true},
}

func runGolden() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "uigolden:", err)
		os.Exit(1)
	}
	if *capture {
		_ = os.MkdirAll(*goldDir, 0o750) // Create below reports a usable error anyway
		fmt.Println("uigolden: CAPTURE mode — validate the written images by eye before committing")
	}
	// Normalize persisted layout state before comparing anything: a prior
	// monkey run (or the user) may have left a different interface scale,
	// dock edge, or float geometry in localStorage — all of which survive
	// the per-state reloads and shift every screenshot against the goldens.
	c.Eval(`Object.keys(localStorage).filter(function(k){return k.indexOf('wasmstuff-')===0;}).forEach(function(k){localStorage.removeItem(k);})`)
	fails := 0
	for _, s := range states {
		problems := run(c, s)
		status := "ok"
		if len(problems) > 0 {
			status = "FAIL"
			fails += len(problems)
		}
		fmt.Printf("  %-14s %s\n", s.name, status)
		for _, p := range problems {
			fmt.Println("      -", p)
		}
	}
	// leave the panel visible again
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p){p.style.display='';p.style.zIndex='';}})()`)
	if *capture {
		fmt.Println("uigolden: goldens written to", *goldDir)
		return
	}
	fmt.Printf("\nuigolden: %d problem(s) across %d states\n", fails, len(states))
	if fails > 0 {
		os.Exit(1)
	}
}

// run visits one state and returns any problems found.
func run(c *cdp.Client, s state) []string {
	var probs []string

	// Navigate: set the hash and hard-reload so the app restores from it.
	c.Eval(fmt.Sprintf(`(function(){location.hash=%q;})()`, s.hash))
	c.Reload(time.Duration(*settleMs) * time.Millisecond)
	// Belt-and-suspenders: ensure it's not auto-rotating (so the hash is stable).
	c.Eval(`(function(){var a=document.getElementById('auto-rotate');if(a&&a.checked){a.checked=false;a.dispatchEvent(new Event('change'));}})()`)
	hash1, _ := c.Eval(`location.hash`).(string)

	// Screenshot with the panel hidden → the model only.
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='none';})()`)
	time.Sleep(250 * time.Millisecond)
	img, err := c.Screenshot()
	c.Eval(`(function(){var p=document.getElementById('controls-panel');if(p)p.style.display='';})()`)
	if err != nil || img == nil {
		return []string{"screenshot failed: " + fmt.Sprint(err)}
	}
	full := img.Bounds()

	// Render-sanity oracle (skip audio modes, which are blank without a source).
	if !s.blankOK {
		frac := cdp.BrightFrac(img, full)
		bbox := cdp.BrightBBox(img, full)
		switch {
		case frac < 0.0008:
			probs = append(probs, fmt.Sprintf("BLANK render (bright=%.4f%%)", frac*100))
		case bbox.Dx() < full.Dx()/12 && bbox.Dy() < full.Dy()/12:
			probs = append(probs, fmt.Sprintf("COLLAPSED to a dot (%dx%d px)", bbox.Dx(), bbox.Dy()))
		case bbox.Dx() > full.Dx()*98/100 && bbox.Dy() > full.Dy()*98/100 && frac > 0.5:
			probs = append(probs, fmt.Sprintf("EXPLODED / fills screen (bright=%.1f%%)", frac*100))
		}
	}

	// Golden image oracle.
	if !s.noGold {
		path := filepath.Join(*goldDir, s.name+".png")
		tol := *goldTol
		if s.tol > 0 {
			tol = s.tol
		}
		if *capture {
			savePNG(path, img)
		} else if gold, err := loadPNG(path); err != nil {
			probs = append(probs, "no golden ("+path+"); run -capture")
		} else if d := cdp.DiffFrac(img, gold, 40); d > tol {
			probs = append(probs, fmt.Sprintf("GOLDEN mismatch %.1f%% > %.0f%%", d*100, tol*100))
		}
	}

	// Permalink round-trip idempotency: reload from the app-normalised hash and
	// confirm it re-serializes to the same thing (every setting survived).
	c.Reload(time.Duration(*settleMs) * time.Millisecond)
	c.Eval(`(function(){var a=document.getElementById('auto-rotate');if(a&&a.checked){a.checked=false;a.dispatchEvent(new Event('change'));}})()`)
	hash2, _ := c.Eval(`location.hash`).(string)
	if diff := hashDiff(hash1, hash2); diff != "" {
		probs = append(probs, "ROUND-TRIP not idempotent: "+diff)
	}

	// Tooltip audit: every interactive element carries (or inherits) a title,
	// and no two titles collide — the "unique, descriptive tooltips" rule.
	if r, _ := c.Eval(tooltipAuditJS).(string); r != "" && r != "ok" {
		probs = append(probs, "TOOLTIPS: "+r)
	}
	return probs
}

// tooltipAuditJS reports duplicated titles and interactive elements with no
// title anywhere in their ancestry. "" / "ok" means clean.
const tooltipAuditJS = `(function(){
  var dups=[],missing=0;var seen={};
  document.querySelectorAll('[title]').forEach(function(e){
    var t=e.getAttribute('title'); if(!t)return;
    if(seen[t]===1)dups.push(t.slice(0,60)); seen[t]=(seen[t]||0)+1;
  });
  var sel='input:not([type=hidden]),select,button,.knob,.led,.numin,.sect-hdr,.swsec-hdr,.plabel';
  document.querySelectorAll(sel).forEach(function(e){
    if(e.closest('[style*="display:none"],[style*="display: none"]'))return;
    var st=getComputedStyle(e); if(st.display==='none'||st.visibility==='hidden')return;
    if(e.title||e.closest('[title]'))return;
    missing++;
  });
  if(!dups.length&&!missing)return 'ok';
  return (dups.length?dups.length+' duplicated ('+dups.slice(0,3).join(' | ')+')':'')+(missing?' '+missing+' untitled':'');
})()`

// ── permalink hash compare (numeric-tolerant) ─────────────────────────────────

func parseHash(h string) map[string]string {
	h = strings.TrimPrefix(h, "#")
	m := map[string]string{}
	if h == "" {
		return m
	}
	// first token is the bare mode name (no '=')
	for i, tok := range strings.Split(h, "&") {
		if tok == "" {
			continue
		}
		if kv := strings.SplitN(tok, "=", 2); len(kv) == 2 {
			m[kv[0]] = kv[1]
		} else if i == 0 {
			m["_mode"] = tok
		} else {
			m[tok] = ""
		}
	}
	return m
}

// rtIgnore are permalink keys that are intentionally dynamic — the pose churns
// under auto-rotate and is re-randomized on a fresh load ("varied view each
// time" is a deliberate feature), so they can't be part of an idempotency
// check. The round-trip oracle verifies the DETERMINISTIC settings survive
// save→restore; pose fidelity is a separate concern (a still-view permalink,
// auto-rotate off, is expected to restore exactly — checked elsewhere).
var rtIgnore = map[string]bool{"rot": true, "drag": true, "dq": true}

// hashDiff returns "" if the two permalinks are equivalent (numbers within a
// small tolerance, everything else exact, dynamic pose keys ignored), else a
// description of the first differences.
func hashDiff(a, b string) string {
	ma, mb := parseHash(a), parseHash(b)
	var diffs []string
	keys := map[string]bool{}
	for k := range ma {
		keys[k] = true
	}
	for k := range mb {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		if rtIgnore[k] {
			continue
		}
		va, oka := ma[k]
		vb, okb := mb[k]
		if !oka {
			diffs = append(diffs, k+" added="+vb)
			continue
		}
		if !okb {
			diffs = append(diffs, k+" dropped="+va)
			continue
		}
		if !valEquiv(va, vb) {
			diffs = append(diffs, fmt.Sprintf("%s %s→%s", k, va, vb))
		}
	}
	if len(diffs) > 3 {
		diffs = append(diffs[:3], fmt.Sprintf("(+%d more)", len(diffs)-3))
	}
	return strings.Join(diffs, ", ")
}

// valEquiv compares two values: comma-separated numbers within tolerance, else
// exact string match.
func valEquiv(a, b string) bool {
	if a == b {
		return true
	}
	as, bs := strings.Split(a, ","), strings.Split(b, ",")
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		fa, ea := strconv.ParseFloat(as[i], 64)
		fb, eb := strconv.ParseFloat(bs[i], 64)
		if ea != nil || eb != nil {
			return false // not both numeric and not equal as strings
		}
		if math.Abs(fa-fb) > 0.05+0.001*math.Abs(fa) {
			return false
		}
	}
	return true
}

// ── png io ────────────────────────────────────────────────────────────────────
