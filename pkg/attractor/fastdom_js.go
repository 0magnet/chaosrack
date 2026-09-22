//go:build js && wasm

package attractor

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// The passes that touch every element, written in JavaScript.
//
// A js.Value holding a JS object or string carries a runtime finalizer so the
// reference table entry can be released, and attaching one
// (runtime.addspecial) is the most expensive thing this package does. A
// js.Value holding a number carries none. That is the whole cost model, and
// it is not the one the code was written against.
//
// Measured on a model change, Brave, 2026-09-21: 78 offsetLeft/offsetWidth
// reads cost 170ms from Go and 0.7ms written in JavaScript, and a forced
// layout of the whole ten-thousand-element page is 0.3ms. Sixty-three per
// cent of a switch was syscall/js.valueGet plus runtime.addspecial — the
// trip, not the work.
//
// So a pass that reads or writes one property on each of many elements
// belongs here, and Go crosses once with the decisions already made. The
// division is deliberate: JavaScript measures and applies, Go decides. What
// stays in Go is everything with a rule behind it — which section a module
// is in, how the bays are packed, how a legend ring is fitted, which panel
// has been milled wider than it is today. None of that is faster in
// JavaScript and all of it is testable in Go.
const fastSource = `(function () {
  var doc = globalThis.document;
  function set(el, attr, v) {
    if (attr === "title") { el.title = v; return; }
    el.setAttribute(attr, v);
  }
  var RUNSUFFIX = " — one of the sections this bay carries. " +
    "See docs/signal-flow.md: the bays run in signal order, and a " +
    "control sits in the same row as the thing it affects.";
  return {
    // bayLabels silkscreens each section's name over the modules it covers.
    //
    // Clear every bay, then measure every bay, then draw — the three are
    // separated because measuring is a read and drawing is a write, and
    // interleaving them made the browser re-lay the page out once per bay.
    //
    // A run is given as an offset and a count into its bay's own modules,
    // which is the order they were just appended in, so no element has to
    // be handed across from Go to name it.
    bayLabels: function (frame, openCls, moduleCls, specJSON) {
      var spec = JSON.parse(specJSON);
      var opens = frame.querySelectorAll("." + openCls);
      var old = frame.querySelectorAll("." + openCls + " > .runit-label");
      var i, j;
      for (i = 0; i < old.length; i++) old[i].remove();
      var plans = [];
      for (i = 0; i < spec.length && i < opens.length; i++) {
        var open = opens[i], kids = open.children, mods = [];
        for (j = 0; j < kids.length; j++) {
          if (kids[j].classList && kids[j].classList.contains(moduleCls)) mods.push(kids[j]);
        }
        var runs = spec[i];
        for (j = 0; j < runs.length; j++) {
          var run = runs[j], a = null, z = null;
          for (var n = run.f; n < run.f + run.c && n < mods.length; n++) {
            // Zero width is what a module switched out looks like: it packs
            // at no width and keeps its place, and has no position to
            // measure a label against.
            if (mods[n].offsetWidth <= 0) continue;
            if (!a) a = mods[n];
            z = mods[n];
          }
          if (!a || !z) continue;
          var left = a.offsetLeft, width = z.offsetLeft + z.offsetWidth - left;
          if (width <= 0) continue;
          plans.push([open, run.s, run.t, left, width]);
        }
      }
      for (i = 0; i < plans.length; i++) {
        var p = plans[i], el = doc.createElement("div");
        el.className = "runit-label";
        el.dataset.section = p[1];
        el.textContent = p[2];
        el.title = p[2] + RUNSUFFIX;
        el.style.left = Math.round(p[3]) + "px";
        el.style.width = Math.round(p[4]) + "px";
        p[0].appendChild(el);
      }
      return plans.length;
    },
    // moduleParts measures what is bolted to each module: the widest legend
    // ring on it, the panel border it may not reach past, and the minimum
    // it is carrying now. A read pass and nothing else — the widths go back
    // in setMinWidths, once Go has decided them.
    moduleParts: function (frame, moduleCls) {
      var ms = frame.querySelectorAll("." + moduleCls);
      var win = globalThis.window || globalThis, out = [];
      for (var i = 0; i < ms.length; i++) {
        var m = ms[i];
        if (m.style && m.style.display === "none") { out.push(null); continue; }
        var ds = m.querySelectorAll(".knob-dial"), widest = 0;
        for (var j = 0; j < ds.length; j++) {
          var w = ds[j].offsetWidth;
          if (w > widest) widest = w;
        }
        var cs = win.getComputedStyle(m);
        var edge = (parseFloat(cs.borderLeftWidth) || 0) + (parseFloat(cs.borderRightWidth) || 0);
        out.push([widest, edge, m.style.minWidth || ""]);
      }
      return JSON.stringify(out);
    },
    // setMinWidths writes the answers back, and reads nothing.
    setMinWidths: function (frame, moduleCls, valsJSON) {
      var ms = frame.querySelectorAll("." + moduleCls), vals = JSON.parse(valsJSON);
      for (var i = 0; i < ms.length && i < vals.length; i++) {
        if (vals[i] === null) continue;
        ms[i].style.minWidth = vals[i];
      }
    },
    // skirtRead measures every legend ring on the panel: the grip each one
    // sits on, the cell it has to stay inside, and the box of every legend
    // engraved on it. Two hundred and fifty stacks and near seven hundred
    // legends, which from Go was a quarter of a model change.
    //
    // It decides nothing. skirtFit and the radii stay in Go, where they are
    // tested; this hands them their inputs and skirtWrite takes the answers.
    skirtRead: function () {
      var stacks = doc.querySelectorAll(".has-dial");
      var win = globalThis.window || globalThis, out = [], i, j, k, w;
      for (i = 0; i < stacks.length; i++) {
        var st = stacks[i];
        // The grip is the largest knob anywhere in the stack, which is what
        // the first ring has to clear.
        var ks = st.querySelectorAll(".knob, .knob-ring"), grip = 0;
        for (j = 0; j < ks.length; j++) { w = ks[j].offsetWidth; if (w / 2 > grip) grip = w / 2; }
        // The one a grip shrink would scale is the largest DIRECT child,
        // which is not always the same element.
        var dk = st.querySelectorAll(":scope > .knob, :scope > .knob-ring");
        var big = -1, bigW = 0, isRing = false;
        for (j = 0; j < dk.length; j++) {
          w = dk[j].offsetWidth;
          if (w > bigW) { bigW = w; big = j; isRing = dk[j].classList.contains("knob-ring"); }
        }
        var dials = st.querySelectorAll(":scope > .knob-dial"), ds = [];
        for (j = 0; j < dials.length; j++) {
          var d = dials[j], cell = d.closest(".pcell"), cw = 0, pad = 0;
          if (cell) {
            cw = cell.clientWidth;
            if (cw > 0) {
              var cs = win.getComputedStyle(cell);
              pad = (parseFloat(cs.paddingLeft) || 0) + (parseFloat(cs.paddingRight) || 0);
            }
          }
          var ls = d.querySelectorAll(".knob-dial-lab"), labs = [];
          for (k = 0; k < ls.length; k++) {
            labs.push([ls[k].getAttribute("data-deg"), ls[k].offsetWidth,
                       ls[k].offsetHeight, ls[k].textContent]);
          }
          ds.push({w: cw, p: pad, l: labs});
        }
        out.push({g: grip, b: big, r: isRing, d: ds});
      }
      return JSON.stringify(out);
    },
    // skirtWrite applies what Go decided, and reads nothing.
    skirtWrite: function (payloadJSON) {
      var stacks = doc.querySelectorAll(".has-dial"), P = JSON.parse(payloadJSON);
      for (var i = 0; i < P.length && i < stacks.length; i++) {
        var st = stacks[i], ent = P[i];
        if (ent.grip && ent.bi >= 0) {
          var dk = st.querySelectorAll(":scope > .knob, :scope > .knob-ring");
          if (ent.bi < dk.length) {
            var s = "scale(" + ent.grip + ")";
            if (ent.ring) s = "translate(-50%,-50%) " + s;
            dk[ent.bi].style.transform = s;
            dk[ent.bi].style.transformOrigin = "center center";
          }
        }
        var dials = st.querySelectorAll(":scope > .knob-dial");
        for (var j = 0; j < ent.d.length && j < dials.length; j++) {
          var d = dials[j], e = ent.d[j];
          if (!e.li || !e.li.length) continue;
          var ls = d.querySelectorAll(".knob-dial-lab");
          if (e.box) { d.style.width = e.box; d.style.height = e.box; }
          for (var k = 0; k < e.li.length; k++) {
            var el = ls[e.li[k]];
            if (!el) continue;
            if (e.font) el.style.fontSize = e.font;
            el.style.left = e.pos[k][0];
            el.style.top = e.pos[k][1];
          }
          if (e.circle) {
            var c = d.querySelector(".knob-ring-circle");
            if (c) { c.style.width = e.circle; c.style.height = e.circle; }
          }
        }
      }
    },
    // tipWrite puts every tooltip on in one pass.
    //
    // The cells are enumerated exactly as buildControlModel does it — each
    // panel module in document order, then its .pcell and .punit children —
    // so an instruction can name a cell by number instead of the Go side
    // handing an element across for each of a few thousand writes.
    // panelCells is the cell list both tooltip passes work from, enumerated
    // exactly as buildControlModel does it: each panel module in document
    // order, then its .pcell and .punit children.
    cells: function () {
      var sects = doc.querySelectorAll(".modules .sect:not(.template-mod)");
      var out = [], i, j;
      for (i = 0; i < sects.length; i++) {
        var cs = sects[i].querySelectorAll(".pcell, .punit");
        for (j = 0; j < cs.length; j++) out.push(cs[j]);
      }
      return out;
    },
    // tipRead is everything the annotate pass needs to know about a cell to
    // work out its tooltips: which kind of control it is, the labels it is
    // named from, and the readouts and selectors it carries.
    //
    // One crossing for the whole panel. Each of these was a querySelector or
    // a classList test from Go, a few per cell over some three hundred
    // cells, and a js.Value holding an element or a string carries a
    // finalizer while one holding a number does not.
    tipRead: function () {
      var cells = this.cells(), out = [], i, j;
      for (i = 0; i < cells.length; i++) {
        var c = cells[i], e;
        var rec = {id: c.id || "", ax: false, pal: false, lbl: "", axl: "",
                   rid: "", leds: [], sel: [], nk: 0, nums: []};
        if (c.classList) {
          rec.ax = c.classList.contains("axrot");
          rec.pal = c.classList.contains("pal-cell");
        }
        e = c.querySelector(".plabel, .u-lbl");
        if (e) rec.lbl = e.textContent;
        e = c.querySelector(".toprow .plabel");
        if (e) rec.axl = e.textContent;
        e = c.querySelector("input[type=range]");
        if (e) rec.rid = e.id || "";
        var leds = c.querySelectorAll(".led:not(.pal-hex)");
        for (j = 0; j < leds.length; j++) {
          var l = leds[j], own = "";
          var prev = l.previousElementSibling;
          if (prev && prev.classList && prev.classList.contains("ledlbl")) own = prev.textContent;
          rec.leds.push([own, l.getAttribute("data-help") || "", l.title || ""]);
        }
        var sels = c.querySelectorAll("select");
        for (j = 0; j < sels.length; j++) rec.sel.push(sels[j].title || "");
        rec.nk = c.querySelectorAll(".knobsel").length;
        var nums = c.querySelectorAll(".numin");
        for (j = 0; j < nums.length; j++) {
          rec.nums.push(!!(nums[j].classList && nums[j].classList.contains("u-step")));
        }
        out.push(rec);
      }
      return JSON.stringify(out);
    },
    tipWrite: function (payload) {
      var cells = this.cells();
      // cell US selector US value US nth US attribute RS, per stamp.
      var recs = payload.split("\x1e"), n = 0, i, j;
      for (i = 0; i < recs.length; i++) {
        if (!recs[i]) continue;
        var f = recs[i].split("\x1f");
        if (f.length < 5) continue;
        var cell = cells[+f[0]];
        if (!cell) continue;
        var els = cell.querySelectorAll(f[1]), nth = +f[3], attr = f[4];
        if (nth >= 0) {
          if (els[nth]) { set(els[nth], attr, f[2]); n++; }
          continue;
        }
        for (j = 0; j < els.length; j++) { set(els[j], attr, f[2]); n++; }
      }
      return n;
    }
  };
})()`

var (
	fastHelper js.Value
	fastTried  bool
)

// fastDOM is the JS helper, or a zero Value if this page will not evaluate it.
//
// Tried once. A page served with a Content-Security-Policy that forbids eval
// makes the call throw, which reaches Go as a panic; the recover is the whole
// point, because every caller keeps its Go path and the panel still works,
// only slower.
func fastDOM() (v js.Value) {
	if fastTried {
		return fastHelper
	}
	fastTried = true
	defer func() {
		if recover() != nil {
			fastHelper, v = js.Value{}, js.Value{}
		}
	}()
	fastHelper = js.Global().Call("eval", fastSource)
	return fastHelper
}

// bayLabelRun is one section's stretch inside a bay, as the JS pass wants it:
// where it starts among that bay's own modules, how many it covers, and what
// is silkscreened over them.
type bayLabelRun struct {
	S string `json:"s"` // section key, for the color
	T string `json:"t"` // the title
	F int    `json:"f"` // first module, within the bay
	C int    `json:"c"` // how many
}

// layoutBayLabels draws every bay's labels in one crossing. Reports whether
// it ran; false means the caller should use its own pass.
func layoutBayLabels(frame js.Value, units [][]int, items []packItem) bool {
	h := fastDOM()
	if !h.Truthy() || !frame.Truthy() {
		return false
	}
	spec := make([][]bayLabelRun, 0, len(units))
	for _, idx := range units {
		runs := make([]bayLabelRun, 0, 4)
		for _, r := range sectionRuns(items, idx) {
			title := sectionTitleOf(r.Section)
			if title == "" || r.Count < 1 {
				continue
			}
			runs = append(runs, bayLabelRun{S: r.Section, T: title, F: r.From, C: r.Count})
		}
		spec = append(spec, runs)
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return false
	}
	h.Call("bayLabels", frame, unitOpenCls, "sect", string(b))
	return true
}

// modulePart is one module as the measuring pass found it: the widest legend
// ring bolted to it, the border it may not reach past, and the minimum width
// it is carrying now. A module that is switched out is a nil entry — it has
// no size to measure and nothing to decide.
type modulePart struct {
	Widest float64
	Edge   float64
	Min    string
}

// fitModulesFast is fitModulesToTheirParts with the reads and the writes
// separated by the decision, instead of alternating with it.
//
// Reading a module's dials and then setting its minimum, module by module,
// made the browser recompute style and layout for the whole rack between
// every pair — measured, 113 forced layouts in one model change, 282ms of
// layout and 295ms of style against 694ms of script. Everything is measured
// first, Go decides, and the answers go back in one write pass.
func fitModulesFast(h js.Value, f js.Value) (changed bool, ok bool) {
	raw := h.Call("moduleParts", f, "sect").String()
	var parts []*modulePart
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		return false, false
	}
	vals := make([]*string, len(parts))
	for i, p := range parts {
		if p == nil {
			continue
		}
		want := ""
		if n := slotsForWidthPx(p.Widest + p.Edge); p.Widest > 0 && n > 1 {
			want = pxStr(slotsWidthPx(n))
		}
		if p.Min != want {
			changed = true
		}
		w := want
		vals[i] = &w
	}
	b, err := json.Marshal(vals)
	if err != nil {
		return false, false
	}
	h.Call("setMinWidths", f, "sect", string(b))
	return changed, true
}

// UnmarshalJSON reads the three-element array the JS pass sends.
func (p *modulePart) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return nil
	}
	if err := json.Unmarshal(raw[0], &p.Widest); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[1], &p.Edge); err != nil {
		return err
	}
	return json.Unmarshal(raw[2], &p.Min)
}

// ── Skirts ────────────────────────────────────────────────────────────────

// What the JS read pass sends back about one legend on a ring.
type skirtLabRead struct {
	Deg  string
	W, H float64
	Text string
}

// UnmarshalJSON reads the four-element array the read pass sends.
func (l *skirtLabRead) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 4 {
		return nil
	}
	// The angle is an attribute and may be absent, which is how a tick label
	// that is not one of ours is told apart; null decodes to "".
	_ = json.Unmarshal(raw[0], &l.Deg) //nolint:errcheck // absent means "not ours", handled below
	if err := json.Unmarshal(raw[1], &l.W); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[2], &l.H); err != nil {
		return err
	}
	return json.Unmarshal(raw[3], &l.Text)
}

type skirtDialRead struct {
	CellW float64        `json:"w"`
	Pad   float64        `json:"p"`
	Labs  []skirtLabRead `json:"l"`
}

type skirtStackRead struct {
	Grip   float64         `json:"g"`
	Big    int             `json:"b"`
	IsRing bool            `json:"r"`
	Dials  []skirtDialRead `json:"d"`
}

type skirtDialWrite struct {
	Li     []int       `json:"li"`
	Pos    [][2]string `json:"pos"`
	Box    string      `json:"box,omitempty"`
	Font   string      `json:"font,omitempty"`
	Circle string      `json:"circle,omitempty"`
}

type skirtStackWrite struct {
	Grip  string           `json:"grip,omitempty"`
	BI    int              `json:"bi"`
	Ring  bool             `json:"ring,omitempty"`
	Dials []skirtDialWrite `json:"d"`
}

// layoutSkirtsFast is layoutSkirts with the measuring and the applying moved
// to JavaScript and the fitting left here.
//
// The arithmetic below is layoutSkirtsIn and layoutOneSkirt unchanged — the
// same clear-and-gap chain outward through a concentric stack, the same
// skirtFit, the same estimates when a label cannot be measured yet. What has
// gone is the two hundred and fifty round trips per pass.
func layoutSkirtsFast(h js.Value) bool {
	raw := h.Call("skirtRead").String()
	var stacks []skirtStackRead
	if err := json.Unmarshal([]byte(raw), &stacks); err != nil {
		return false
	}
	gap := skirtGapPx()
	out := make([]skirtStackWrite, 0, len(stacks))
	for _, s := range stacks {
		// Not laid out yet — a detached subtree, a module switched out, a
		// panel not yet shown. Estimate rather than bail: a ring that is
		// never laid out has no positions at all and its legends sit on the
		// origin in a heap.
		clear := s.Grip
		if clear <= 0 {
			clear = estGripRadiusPx()
		}
		w := skirtStackWrite{BI: s.Big, Ring: s.IsRing, Dials: make([]skirtDialWrite, 0, len(s.Dials))}
		for di, d := range s.Dials {
			labs := make([]skirtLabel, 0, len(d.Labs))
			kept := make([]int, 0, len(d.Labs))
			for li, l := range d.Labs {
				deg, err := strconv.ParseFloat(l.Deg, 64)
				if err != nil {
					continue // not one of ours (the angle dial's tick labels)
				}
				lw, lh := l.W, l.H
				if lw <= 0 || lh <= 0 {
					lw, lh = estLabelBoxPx(l.Text)
				}
				labs = append(labs, skirtLabel{W: lw, H: lh, Deg: deg})
				kept = append(kept, li)
			}
			dw := skirtDialWrite{Li: kept}
			if len(labs) == 0 {
				w.Dials = append(w.Dials, dw)
				continue
			}
			// A ring outside another one has no grip to take room from, so
			// its floor is the radius it already has and the legend carries
			// the whole reduction.
			minGrip := clear
			if di == 0 {
				minGrip = clear * skirtMinGripFrac
			}
			room := 0.0
			if d.CellW > 0 {
				room = (d.CellW - d.Pad + skirtCellGapPx*panelScale) / 2
			}
			if room > 0 {
				room -= gap
			}
			useGrip, scale := skirtFit(clear, minGrip, gap, room, labs)
			if scale < 1 {
				labs = skirtScaleLabels(labs, scale)
				dw.Font = pxStr(skirtLabelBasePx * panelScale * scale)
			}
			if useGrip < clear && useGrip > 0 {
				dw.Box = "" // set below; the grip is the stack's, not the dial's
				w.Grip = strconv.FormatFloat(useGrip/clear, 'f', 3, 64)
			}
			clear = useGrip
			r := skirtRadius(clear, gap, labs)
			o := skirtOuter(r, labs)
			// The box has to contain the labels, or the element that exists
			// to hold them is the thing clipping them.
			box := 2 * (o + gap)
			dw.Box = pxStr(box)
			dw.Circle = pxStr(2 * r)
			dw.Pos = make([][2]string, 0, len(labs))
			for _, l := range labs {
				x := box/2 + r*math.Sin(l.Deg*math.Pi/180)
				y := box/2 - r*math.Cos(l.Deg*math.Pi/180)
				dw.Pos = append(dw.Pos, [2]string{pxStr(x), pxStr(y)})
			}
			w.Dials = append(w.Dials, dw)
			clear = o
		}
		out = append(out, w)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return false
	}
	h.Call("skirtWrite", string(b))
	return true
}

// ── Tooltips ──────────────────────────────────────────────────────────────

// tipStamp is one queued tooltip write: a selector inside one control's cell,
// the title to put on what it matches, and optionally which single match.
//
// The cell is named by its place in buildControlModel's enumeration rather
// than by handing the element across, because the JS side walks the same two
// selectors in the same order and arrives at the same list.
type tipStamp struct {
	Cell  int
	Sel   string
	Title string
	Nth   int    // -1 for every match
	Attr  string // "title", or the attribute a readout memoizes into
}

var (
	tipQueue    []tipStamp
	tipBatching bool
	tipCell     int
)

// queueStamp collects a tooltip write instead of performing it. Reports
// whether it did; false means the caller writes it itself.
//
// The whole annotate pass is a few thousand of these — ten or so per control
// over some three hundred controls — and each one from Go was a
// querySelectorAll whose NodeList and every element in it arrived as a
// js.Value with a finalizer attached. Measured, two thirds of the pass was
// runtime.addspecial. Collected and sent once, it is a single crossing.
func queueStamp(sel, title string, nth int) bool {
	return queueAttr(sel, "title", title, nth)
}

// queueAttr is queueStamp for an attribute other than the tooltip.
func queueAttr(sel, attr, value string, nth int) bool {
	if !tipBatching {
		return false
	}
	tipQueue = append(tipQueue, tipStamp{Cell: tipCell, Sel: sel, Title: value, Nth: nth, Attr: attr})
	return true
}

// flushStamps applies every queued tooltip in one crossing.
func flushStamps() {
	q := tipQueue
	tipQueue, tipBatching = tipQueue[:0], false
	if len(q) == 0 {
		return
	}
	h := fastDOM()
	if !h.Truthy() {
		return
	}
	// A flat delimited string rather than JSON. There are a few thousand of
	// these and every one carries a sentence; encoding each as its own JSON
	// array cost more in Go than the writes it was saving.
	var b strings.Builder
	b.Grow(len(q) * 64)
	for _, s := range q {
		b.WriteString(strconv.Itoa(s.Cell))
		b.WriteByte(0x1f)
		b.WriteString(s.Sel)
		b.WriteByte(0x1f)
		b.WriteString(s.Title)
		b.WriteByte(0x1f)
		b.WriteString(strconv.Itoa(s.Nth))
		b.WriteByte(0x1f)
		b.WriteString(s.Attr)
		b.WriteByte(0x1e)
	}
	h.Call("tipWrite", b.String())
}

// ledRead is one readout as the JS pass found it: its own label where it has
// one, the description the markup gave it, and its current tooltip.
type ledRead struct {
	Own   string
	Help  string
	Title string
}

// UnmarshalJSON reads the three-element array the read pass sends.
func (l *ledRead) UnmarshalJSON(b []byte) error {
	var raw []string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) == 3 {
		l.Own, l.Help, l.Title = raw[0], raw[1], raw[2]
	}
	return nil
}

// cellRead is everything the annotate pass needs to know about one control
// cell, read once for the whole panel instead of a few queries per cell.
type cellRead struct {
	ID    string    `json:"id"`
	AxRot bool      `json:"ax"`
	Pal   bool      `json:"pal"`
	Label string    `json:"lbl"`  // .plabel, .u-lbl — the control's own name
	Axis  string    `json:"axl"`  // .toprow .plabel — X, Y or Z
	RID   string    `json:"rid"`  // the hidden slider's id, which carries the help
	LEDs  []ledRead `json:"leds"` // .led:not(.pal-hex), in order
	Sels  []string  `json:"sel"`  // each select's title, for naming its knob
	NKnob int       `json:"nk"`   // how many selector knobs
	Nums  []bool    `json:"nums"` // .numin, true where it is a step field
}

// tipReads is the panel as the last read pass found it, indexed the same way
// the stamps are. Empty when there was no read pass.
var tipReads []cellRead

// readPanelCells measures the whole panel's cells in one crossing.
func readPanelCells(h js.Value) bool {
	raw := h.Call("tipRead").String()
	tipReads = tipReads[:0]
	return json.Unmarshal([]byte(raw), &tipReads) == nil
}

// cellFacts is what the read pass found for this control, or nil when there
// was none and the caller should ask the DOM itself.
func (c *Control) cellFacts() *cellRead {
	if !tipBatching || c.tipIdx < 0 || c.tipIdx >= len(tipReads) {
		return nil
	}
	return &tipReads[c.tipIdx]
}
