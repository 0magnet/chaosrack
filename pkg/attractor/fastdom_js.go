//go:build js && wasm

package attractor

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/skirt"
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
  return {
    // scopeStroke walks n floats of the buffer as x,y pairs, the scope's
    // sweep: beginPath through stroke stays on this side, so the whole sweep
    // is one crossing. See scopefast_js.go.
    scopeStroke: function (ctx, pts, n) {
      if (n < 4) return;
      ctx.beginPath();
      ctx.moveTo(pts[0], pts[1]);
      for (var i = 2; i < n; i += 2) ctx.lineTo(pts[i], pts[i + 1]);
      ctx.stroke();
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
        // The border and not the padding: a ring may reach a little past
        // its cell (skirtCellGapPx), and the padding is what absorbs that.
        // Charged for it, Grid was milled two slots wide for a ring eight
        // tenths of a pixel over.
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
    // rackSig is the rack's shape: each shown module's name and width, in
    // order (rackUnmoved).
    rackSig: function () {
      var ms = doc.querySelectorAll("#controls-panel .sect"), out = [];
      for (var i = 0; i < ms.length; i++) {
        var w = ms[i].offsetWidth;
        if (!w) continue;
        var h = ms[i].querySelector(".sect-hdr");
        out.push((h ? h.textContent : ms[i].id) + ":" + w);
      }
      return out.join(",");
    },
    // The rack's bays, as it shows them: its units in order, bay 1 first
    // (the number on each one's left ear).
    bays: function () {
      return [].filter.call(doc.querySelectorAll("#controls-panel .runit"), function (u) { return u.offsetParent; });
    },
    // bayOf is the bay el is in, counting from 1, or 0 for none.
    bayOf: function (el) {
      var u = el && el.closest ? el.closest(".runit") : null;
      return u ? this.bays().indexOf(u) + 1 : 0;
    },
    // mostVisibleBay is the bay with the most of itself in the window, or 0.
    mostVisibleBay: function () {
      var us = this.bays(), best = 0, area = 0, vh = window.innerHeight, vw = window.innerWidth;
      for (var i = 0; i < us.length; i++) {
        var r = us[i].getBoundingClientRect();
        var a = Math.max(0, Math.min(r.bottom, vh) - Math.max(r.top, 0)) * Math.max(0, Math.min(r.right, vw) - Math.max(r.left, 0));
        if (a > area) { area = a; best = i + 1; }
      }
      return best;
    },
    // bayShown is how much of bay n is in the window, 0 to 1.
    bayShown: function (n) {
      var u = this.bays()[n - 1];
      if (!u) return 0;
      var r = u.getBoundingClientRect(), vh = window.innerHeight;
      return r.height ? Math.max(0, Math.min(r.bottom, vh) - Math.max(r.top, 0)) / r.height : 0;
    },
    // bayManual is bay n's manual, as JSON: each module shown on it, its
    // header's name and explanation, and each of its addressed cells with
    // what its tooltip says, in address order: down each column, then across.
    // Each comes with the keys the manual may have it under (docs.go): the
    // data-doc it was built with, then the ids in it, nearest first.
    bayManual: function (n) {
      var u = this.bays()[n - 1];
      if (!u) return "[]";
      var out = [], ms = u.querySelectorAll(".sect");
      for (var i = 0; i < ms.length; i++) {
        if (ms[i].offsetParent) out.push(this.moduleInfo(ms[i]));
      }
      return JSON.stringify(out);
    },
    // moduleManual is one module's part of the manual, as JSON, wherever the
    // module is: in the rack, on the manual page or in a window.
    moduleManual: function (m) { return JSON.stringify(this.moduleInfo(m)); },
    // moduleInfo is module m as the manual reads it.
    moduleInfo: function (m) {
      var num = function (loc) { var p = loc.split("."); return ((parseInt(p[1], 10) || 0) * 8 + (parseInt(p[2], 10) || 0)) * 32 + (p[3] ? p[3].charCodeAt(0) - 96 : 0); };
      var inner = function (c, loc) {
        var sels = [".knob[title]", "select[title]", "input[title]", "button[title]", ".led[title]", "[title]"];
        for (var i = 0; i < sels.length; i++) {
          var es = c.querySelectorAll(sels[i]);
          for (var j = 0; j < es.length; j++) {
            var s = es[j].getAttribute("title") || "";
            if (s.indexOf(loc + " · ") === 0) s = s.slice(loc.length + 3);
            s = s.replace(/^(?:[^/\n]{1,40} \/ )+/, ""); // a knob's "Display / speed / knob / " path
            if (s && s !== loc) return s;
          }
        }
        return "";
      };
      var keys = function (el) {
        var ks = [], add = function (k) { if (k && ks.indexOf(k) < 0) ks.push(k); };
        add(el.getAttribute("data-doc"));
        add(el.id);
        var ds = el.querySelectorAll("[data-doc],[id]");
        for (var i = 0; i < ds.length && ks.length < 8; i++) {
          add(ds[i].getAttribute("data-doc"));
          add(ds[i].id);
        }
        return ks;
      };
      var h = m.querySelector(".sect-hdr"), cells = [], seen = {};
      var cs = m.querySelectorAll("[data-loc]");
      for (var j = 0; j < cs.length; j++) {
        var c = cs[j], loc = c.getAttribute("data-loc");
        if (seen[loc] || !c.getClientRects().length) continue;
        seen[loc] = 1;
        var t = c.getAttribute("title") || "";
        if (t.indexOf(loc + " · ") === 0) t = t.slice(loc.length + 3);
        // A cell with no tooltip of its own (its title is its address) is
        // described by what is in it: its knob or selector first.
        if (!t || t === loc) t = inner(c, loc);
        if (t) cells.push({l: loc, t: t, k: keys(c)});
      }
      cells.sort(function (a, b) { return num(a.l) - num(b.l); });
      var r = m.getAttribute("data-mloc") || "";
      var tip = h ? (h.getAttribute("title") || "") : "";
      if (r && tip.indexOf(r + " · ") === 0) tip = tip.slice(r.length + 3);
      var mk = [];
      if (h && h.getAttribute("data-doc")) mk.push(h.getAttribute("data-doc"));
      if (m.id) mk.push(m.id, m.id.replace(/-module$/, ""));
      return {h: h ? h.textContent : "", t: tip, r: r, c: cells, k: mk};
    },
    // lightLive marks the cells of model live, and every other model's not:
    // its own, and the shared ones whose list names it. With model empty
    // (the rack powered down) nothing is live.
    lightLive: function (model) {
      var cs = doc.querySelectorAll(".punit[data-mode]");
      for (var i = 0; i < cs.length; i++) {
        var c = cs[i], live = false;
        if (model) {
          live = c.getAttribute("data-mode") === model;
          var f = c.getAttribute("data-bank-for");
          if (!live && f) live = (" " + f + " ").indexOf(" " + model + " ") >= 0;
        }
        c.classList.toggle("live", live);
      }
    },
    // bankShow shows, in each of parts (a bank bay's head and bank), the
    // cells of model and hides the rest: its step cell and its positions, and
    // the shared and unassigned positions whose list names it. unassigned is
    // the hover of an unassigned position that is showing. syncBankCells did
    // this from Go, a few calls a cell over two hundred and fifty of them,
    // and it was a fifth of a model change.
    bankShow: function (parts, model, unassigned) {
      for (var p = 0; p < parts.length; p++) {
        var part = parts[p], i, c, on;
        var qs = [[".stepcell[data-mode]", "data-mode"], [".bankcell[data-bank]", "data-bank"]];
        for (var q = 0; q < qs.length; q++) {
          var cs = part.querySelectorAll(qs[q][0]);
          for (i = 0; i < cs.length; i++) {
            c = cs[i];
            c.style.display = c.getAttribute(qs[q][1]) === model ? "" : "none";
          }
        }
        var bs = part.querySelectorAll("[data-bank-for]");
        for (i = 0; i < bs.length; i++) {
          c = bs[i];
          on = (" " + c.getAttribute("data-bank-for") + " ").indexOf(" " + model + " ") >= 0;
          c.style.display = on ? "" : "none";
          if (on && c.classList.contains("bankblank")) c.title = unassigned;
        }
      }
    },
    // mxRetarget relabels the Mod matrix's rows as Go decided
    // (modMxRetarget): each entry is a row's index, its legend's dots
    // (markup), its tooltip and the control it targets ("" for none), and
    // its pins' tooltips. The rows are the matrix's legends in order, and
    // each has n pins after it.
    mxRetarget: function (n, payloadJSON) {
      var legs = doc.querySelectorAll("#mod-matrix .mxleg"), pins = doc.querySelectorAll("#mod-matrix .mxpin");
      var P = JSON.parse(payloadJSON);
      for (var k = 0; k < P.length; k++) {
        var r = P[k], leg = legs[r.i];
        if (!leg) continue;
        leg.innerHTML = r.svg;
        leg.title = r.title;
        if (r.col) leg.setAttribute("data-col", r.col); else leg.removeAttribute("data-col");
        for (var j = 0; j < n; j++) {
          var p = pins[r.i * n + j];
          if (!p) continue;
          p.classList.toggle("mxpin-none", !r.col);
          if (!r.col) { p.removeAttribute("title"); p.removeAttribute("data-col"); continue; }
          p.title = r.pins[j];
          p.setAttribute("data-col", r.col);
        }
      }
    },
    // controlCells is every control cell of every module, in the order
    // readPanelCells counts them, for buildControlModel: the cells, and one
    // row per module ("S" and its header) and per cell ("C", its flags --
    // b blank, r rotation, p palette -- its id and its data-bank), as JSON.
    controlCells: function () {
      var sects = doc.querySelectorAll(".modules .sect"), cells = [], lines = [];
      for (var i = 0; i < sects.length; i++) {
        var h = sects[i].querySelector(".sect-hdr");
        lines.push(["S", h ? h.textContent : ""]);
        var cs = sects[i].querySelectorAll(".pcell, .punit");
        for (var j = 0; j < cs.length; j++) {
          var c = cs[j], cl = c.classList;
          cells.push(c);
          lines.push(["C", (cl.contains("bankblank") ? "b" : "") + (cl.contains("axrot") ? "r" : "") +
            (cl.contains("pal-cell") ? "p" : ""), c.id, c.getAttribute("data-bank") || ""]);
        }
      }
      return { cells: cells, info: JSON.stringify(lines) };
    },
    // trioParams names the parameter of each column of buttons under root
    // that sel matches, "" for a column outside any position, joined by
    // newlines. trioApply then gives them what Go decided (syncTriosIn).
    trioParams: function (root, sel) {
      var cols = root.querySelectorAll(sel), out = [];
      for (var i = 0; i < cols.length; i++) {
        var c = cols[i].closest("[data-param]");
        out.push(c ? c.getAttribute("data-param") : "");
      }
      return out.join("\n");
    },
    // trioApply lights, labels and explains the columns trioParams named,
    // in the same order: P maps a parameter (or "" for none) to its live
    // flag, and per button whether it is on, dead, its legend and its
    // tooltip (null leaves the tooltip as it is).
    trioApply: function (root, sel, payloadJSON) {
      var cols = root.querySelectorAll(sel), P = JSON.parse(payloadJSON);
      for (var i = 0; i < cols.length; i++) {
        var c = cols[i].closest("[data-param]");
        var e = P[c ? c.getAttribute("data-param") : ""] || P[""];
        if (!e) continue;
        cols[i].classList.toggle("trio-live", e.live);
        var bs = cols[i].querySelectorAll(".trio-btn");
        for (var j = 0; j < bs.length; j++) {
          var b = bs[j];
          b.classList.toggle("trio-on", !!(e.on && e.on[j]));
          b.classList.toggle("trio-dead", !!(e.dead && e.dead[j]));
          var lamp = b.querySelector(".trio-lamp");
          if (lamp && j < e.keys.length && lamp.textContent !== e.keys[j]) lamp.textContent = e.keys[j];
          if (e.tips && e.tips[j] != null) b.title = e.tips[j];
        }
      }
    },
    // skirtStacks are the knob stacks with legend rings under root: one
    // stack, the stacks inside an element, or with no root the whole page.
    // The read and the write are handed the same root, so they agree on
    // which stack an entry is.
    skirtStacks: function (root) {
      if (!root) return doc.querySelectorAll(".has-dial");
      if (root.classList && root.classList.contains("has-dial")) return [root];
      return root.querySelectorAll(".has-dial");
    },
    // skirtRead measures every legend ring under root: the grip each one
    // sits on, the cell it has to stay inside, and the box of every legend
    // engraved on it. On the whole panel that is two hundred and fifty
    // stacks and near seven hundred legends, which from Go was a quarter of
    // a model change.
    //
    // It decides nothing. skirt.Fit and the radii stay in Go, where they are
    // tested; this hands them their inputs and skirtWrite takes the answers.
    // skirtSig is what a stack's fit depends on: the sizes of its knobs, the
    // width of the cell it stands in, and its legends and their angles. The
    // same inputs fit the same way, so a stack whose signature is the one it
    // was last fitted with is left as it is (skirtRead).
    skirtSig: function (st) {
      // The fonts too: a legend measured in the fallback face before the web
      // font arrived is the wrong width, and the fit has to be done again.
      var f = doc.fonts, s = (f ? f.status + f.size : "") + "|", i, ks = st.querySelectorAll(".knob, .knob-ring");
      for (i = 0; i < ks.length; i++) s += ks[i].offsetWidth + ",";
      var cell = st.closest(".pcell");
      s += "|" + (cell ? cell.clientWidth : 0) + "|";
      var ls = st.querySelectorAll(".knob-dial-lab");
      for (i = 0; i < ls.length; i++) s += ls[i].getAttribute("data-deg") + ":" + ls[i].textContent + ";";
      return s;
    },
    skirtRead: function (root) {
      var stacks = this.skirtStacks(root), sigs = [], i, j, k, w;
      // Which stacks need fitting at all, from their inputs alone: a model
      // change rebuilds a few rings, and fitting all two hundred and fifty
      // again was a sixth of it.
      for (i = 0; i < stacks.length; i++) {
        var sg = this.skirtSig(stacks[i]);
        sigs.push(stacks[i].__skirtSig === sg ? null : sg);
      }
      // Legends are measured at the stylesheet's size, not at whatever the
      // last fit shrank them to: measured shrunk, the next fit came out
      // different, and a ring alternated between two sizes a pixel apart on
      // every panel rebuild, so every model change moved it. Cleared first,
      // all of them, so the reads below cost one layout.
      for (i = 0; i < stacks.length; i++) {
        if (sigs[i] === null) continue;
        var old = stacks[i].querySelectorAll(".knob-dial-lab");
        for (j = 0; j < old.length; j++) if (old[j].style.fontSize) old[j].style.fontSize = "";
      }
      var win = globalThis.window || globalThis, out = [];
      for (i = 0; i < stacks.length; i++) {
        var st = stacks[i];
        if (sigs[i] === null) { out.push({k: true}); continue; }
        st.__skirtPending = sigs[i];
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
    skirtWrite: function (root, payloadJSON) {
      var stacks = this.skirtStacks(root), P = JSON.parse(payloadJSON);
      for (var i = 0; i < P.length && i < stacks.length; i++) {
        var st = stacks[i], ent = P[i];
        if (ent.keep) continue;
        // Fitted: what it was fitted with is its signature now (skirtSig).
        st.__skirtSig = st.__skirtPending;
        if (ent.grip && ent.bi >= 0) {
          var dk = st.querySelectorAll(":scope > .knob, :scope > .knob-ring");
          if (ent.bi < dk.length) {
            var s = "scale(" + ent.grip + ")";
            // A ring is centered with translate(-50%,-50%); a bare scale over
            // that is a knob half its width down and to the right.
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
      var sects = doc.querySelectorAll(".modules .sect");
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
    tipRead: function (mark) {
      var cells = this.cells(), out = [], i, j;
      for (i = 0; i < cells.length; i++) {
        var c = cells[i], e;
        // Stamped already, and nothing in it rebuilt since: a model change
        // replaces a few cells' parts, and restamping all six hundred was a
        // third of it. A part without a title is a part the stamp has not
        // reached (a knob rebuilt inside a cell that stayed). Only a model
        // change's pass marks and skips (mark): some cells get their help
        // after the passes of the boot, and a later full pass is what fixes
        // them.
        if (mark && c.__tipDone && !c.querySelector(".knob:not([title]), .led:not([title]), .numin:not([title]), button:not([title])")) {
          out.push({done: true});
          continue;
        }
        if (mark) c.__tipDone = true;
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
          // A label display says its name in aria-label: its text is dots.
          if (prev && prev.classList && prev.classList.contains("ledlbl")) own = prev.getAttribute("aria-label") || prev.textContent;
          rec.leds.push([own, l.getAttribute("data-help") || "", l.title || ""]);
        }
        var sels = c.querySelectorAll("select");
        for (j = 0; j < sels.length; j++) rec.sel.push(sels[j].title || "");
        rec.nk = c.querySelectorAll(".knobsel").length;
        var nums = c.querySelectorAll(".numin");
        for (j = 0; j < nums.length; j++) {
          rec.nums.push(!!(nums[j].classList && nums[j].classList.contains("u-step")));
        }
        // The value readout's own description, where the markup wrote one:
        // remembered in data-help, or still in its title before the stamp
        // replaces it (vhn says it is new and has to be remembered).
        rec.vh = ""; rec.vhn = false;
        // The hidden slider's own sentence, where the markup gave it one.
        e = c.querySelector("input[type=range]");
        rec.rt = ""; rec.rtn = false;
        if (e) {
          rec.rt = e.getAttribute("data-help") || "";
          if (!rec.rt) {
            var rtt = (e.getAttribute("title") || "").replace(/^(?:[0-9S]+\.\d+\.\d+[a-z]? · )+/, "").trim();
            if (rtt && rtt.indexOf(" / slider") < 0) { rec.rt = rtt; rec.rtn = true; }
          }
        }
        // And the cell's own "Name — what it does", where it has one.
        rec.ct = (c.getAttribute("title") || "").replace(/^(?:[0-9S]+\.\d+\.\d+[a-z]? · )+/, "").trim();
        var vn = c.querySelector(".numin:not(.u-step)");
        if (vn) {
          rec.vh = vn.getAttribute("data-help") || "";
          if (!rec.vh) {
            var vt = (vn.getAttribute("title") || "").replace(/^(?:[0-9S]+\.\d+\.\d+[a-z]? · )+/, "").trim();
            if (vt && vt.indexOf(" / value field") < 0 && vt.indexOf(" / step-size field") < 0) {
              rec.vh = vt; rec.vhn = true;
            }
          }
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
    },
    // designate gives every control on the rack an address,
    // bay.column.row: the bay's number (the one on its ear), the slot the
    // control stands in, 1 to 12 from the bay's left edge (one slot being
    // the narrowest module), and the row, 1 to 3 from the top. So an address
    // says where in the bay a control is, as a part number on a drawing
    // does, and a module that moves keeps its controls' rows. Two or more
    // controls in one cell are lettered, top to bottom and then left to
    // right: 3.2.1.a, 3.2.1.b.
    //
    // The columns and rows are the bay's slots and rows, measured: a control
    // is in the slot and the row it covers at least half of, or half of
    // itself, whichever is less, so a knob a little off its slot's center is
    // still in one slot. A part covering several
    // cells — a monitor's screen, the scope's tube, a pin matrix, the
    // keybed — takes its top-left cell, ahead of the controls that sit on
    // it.
    //
    // A module's own address is bay.column, its first column.
    //
    // A control is the box around one actuator — a knob, a switch, a
    // button, a selector — and what goes with it: the outermost cell that
    // holds it where the panel has one, or else the label it is in. Its
    // readouts, legend and reset are inside it and share its address.
    //
    // Every tooltip inside a control is prefixed with the address, and a
    // control with none gets the address alone. The address is only in the
    // tooltips: printed on the panel it was clutter over every control.
    //
    // Reads first, for the whole rack, then writes.
    //
    // A bay's slot pitch is measured from the bay: its open width is cap
    // slots less one seam, and gapRatio is the seam's share of a pitch. The
    // pitch computed from the layout's scale (pitch0, kept for a bay that
    // cannot be measured) ran a few percent wide at some zooms, and a
    // module ending in slot 12 was addressed as ending in 11.
    designate: function (frame, pitch0, cap, gapRatio, rows) {
      var pitch = pitch0;
      // A readout is a control too, for addressing: a module of nothing but
      // readouts (Timing) was a module with no addresses at all.
      var ACT = ".knob:not(.knob-fine), input.sw, button:not(.rst), select, .pslot, .led, input[type=text]";
      var CELL = ".punit, .pcell, .knobstack, .selwrap, .swline, .rec-swrow";
      // Displays that are parts of their own, addressed as themselves.
      var PART = ".monbezel, .scope-tube, .mxgrid, .keys-bed";
      // A row of switches inside a bigger cell (Screen and the model's
      // switches under a monitor): each is a control of its own, not the
      // monitor's cell.
      var OWNROW = ".monsw";
      var PREFIX = /^(?:[0-9S]+\.\d+(?:\.\d+(?:\.[a-z])?)? · )+/;
      var LETTERS = "abcdefghijklmnopqrstuvwxyz";
      var i, j, k;
      function shown(e) { return e.getClientRects().length > 0; }
      function actuators(box) {
        var a = box.querySelectorAll(ACT), out = [];
        for (var n = 0; n < a.length; n++) if (shown(a[n])) out.push(a[n]);
        return out;
      }
      // span is the first position, 1-based, that a..b covers on a scale of
      // n positions of size step starting at o.
      function span(a, b, o, step, n) {
        var need = Math.min(step, b - a) / 2;
        for (var p = 1; p <= n; p++) {
          var ov = Math.min(b, o + p * step) - Math.max(a, o + (p - 1) * step);
          if (ov >= need) return p;
        }
        return Math.min(n, Math.max(1, Math.floor(((a + b) / 2 - o) / step) + 1));
      }
      var found = [], units = [], modWrites = [], basePitch = 0;
      // What is addressed: every numbered bay, by its modules.
      var opens = frame.querySelectorAll(".runit-open");
      for (i = 0; i < opens.length; i++) {
        var open = opens[i], ear = open.parentNode && open.parentNode.firstElementChild;
        var bay = ear && ear.getAttribute && ear.getAttribute("data-bay");
        if (!bay) continue;
        var mods = [], kids = open.children;
        for (j = 0; j < kids.length; j++) {
          if (kids[j].classList.contains("sect") && kids[j].offsetWidth > 0) mods.push(kids[j]);
        }
        var ow = open.getBoundingClientRect().width;
        units.push({bay: bay, mods: mods, pitch: ow / (cap - gapRatio)});
        // The slot pitch at the rack's own size, for a module drawn elsewhere.
        if (!basePitch && open.offsetWidth) basePitch = ow / (cap - gapRatio) / (ow / open.offsetWidth);
      }
      // A module taken out of the rack (the manual page, manuallive_js.go) is
      // addressed where it is, as a bay of one: its slots at its own zoom, and
      // the address it was given in the rack.
      var moved = doc.querySelectorAll(".mlive-ctx[data-mloc]");
      for (i = 0; i < moved.length; i++) {
        var mm0 = moved[i].querySelector(".sect");
        if (!mm0 || !mm0.getClientRects().length || !mm0.offsetWidth) continue;
        var zz = mm0.getBoundingClientRect().width / mm0.offsetWidth, ml = moved[i].getAttribute("data-mloc");
        units.push({bay: ml.split(".")[0], mods: [mm0], pitch: (basePitch || pitch0) * zz, loc: ml});
      }
      for (i = 0; i < units.length; i++) {
        var u = units[i], bay = u.bay, mods = u.mods;
        if (!mods.length) continue;
        pitch = u.pitch > 0 ? u.pitch : pitch0;
        // A bay opens with its head in its first slot, so slot 1 starts
        // where the first module does.
        var x0 = mods[0].getBoundingClientRect().left;
        // Rows are the bay's, under the module headers: the first module's
        // header bottom to its foot, in rows of equal height.
        var m0 = mods[0].getBoundingClientRect(), h0 = mods[0].querySelector(":scope > .sect-hdr");
        var y0 = h0 ? h0.getBoundingClientRect().bottom : m0.top;
        var rowH = (m0.bottom - y0) / rows;
        for (j = 0; j < mods.length; j++) {
          var m = mods[j], acts = actuators(m), seen = new Set(), cells = [];
          // A module is addressed by the first column of the bay it stands in.
          // One drawn away from the rack (the manual) keeps the address it has
          // there, and its controls the columns they have there.
          var mr0 = m.getBoundingClientRect();
          var col0 = span(mr0.left, mr0.left + Math.min(pitch, mr0.width), x0, pitch, cap);
          var mloc = u.loc || (bay + "." + col0);
          var shift = u.loc ? (parseInt(u.loc.split(".")[1], 10) || 1) - col0 : 0;
          // The module's own address, on its tooltip and its header's, so
          // hovering anywhere on it that is not a control says where it is.
          // Written with the controls', after every read: a write between two
          // reads made the browser restyle the page for the second.
          if (m.classList.contains("sect")) modWrites.push({ m: m, loc: mloc });
          for (k = 0; k < acts.length; k++) {
            var c = null, p = acts[k];
            while (p && p !== m && !p.matches(OWNROW)) {
              if (p.matches(CELL)) c = p;
              p = p.parentElement;
            }
            // No cell: the label it is in (a switch and its word), or itself.
            // Not the largest box holding it alone, which for a switch under
            // a screen was the whole monitor.
            if (!c) {
              c = acts[k].closest("label");
              if (!c || !m.contains(c) || actuators(c).length !== 1) c = acts[k];
            }
            // A readout in a column of readouts is a control of its own: a
            // cell of readouts with no knob (Timing, Distortion, Loudness)
            // would otherwise give every readout in it one address.
            if (c !== acts[k] && acts[k].matches(".led") && !c.querySelector(".knob") && c.querySelectorAll(".led").length > 1) c = acts[k];
            if (seen.has(c)) continue;
            seen.add(c);
            var r = c.getBoundingClientRect();
            if (r.width > 0 && r.height > 0) cells.push({el: c, r: r});
          }
          var parts = m.querySelectorAll(PART);
          for (k = 0; k < parts.length; k++) {
            if (!shown(parts[k]) || seen.has(parts[k])) continue;
            seen.add(parts[k]);
            // First, so the controls on a part (a readout on a screen) are
            // addressed after it and keep their own addresses.
            cells.unshift({el: parts[k], r: parts[k].getBoundingClientRect(), part: true});
          }
          for (k = 0; k < cells.length; k++) {
            var e = cells[k];
            e.row = span(e.r.top, e.r.bottom, y0, rowH, rows);
            e.col = span(e.r.left, e.r.right, x0, pitch, cap);
          }
          // Down each column, then across. In one slot and row, a part
          // before what sits on it, then top to bottom, and a row of switches
          // (tops within a few pixels of one another) left to right.
          cells.sort(function (a, b) {
            return a.col - b.col || a.row - b.row || (b.part ? 1 : 0) - (a.part ? 1 : 0) ||
              (Math.abs(a.r.top - b.r.top) > 4 ? a.r.top - b.r.top : a.r.left - b.r.left);
          });
          // The address: the bay's column the control stands in, a slot across
          // from the bay's left, and its row, 1 to 3. Two or more in one cell
          // are lettered.
          var at = {};
          for (k = 0; k < cells.length; k++) {
            var pos = (cells[k].col + shift) + "." + cells[k].row;
            cells[k].pos = pos;
            (at[pos] = at[pos] || []).push(cells[k]);
          }
          for (k = 0; k < cells.length; k++) {
            var same = at[cells[k].pos];
            cells[k].loc = bay + "." + cells[k].pos + (same.length > 1 ? "." + LETTERS.charAt(same.indexOf(cells[k])) : "");
            found.push(cells[k]);
          }
        }
      }
      for (i = 0; i < modWrites.length; i++) {
        var mw = modWrites[i], mm = mw.m, hdr = mm.querySelector(":scope > .sect-hdr");
        var tip = ((hdr && hdr.getAttribute("title")) || mm.getAttribute("title") || (hdr && hdr.textContent) || "").replace(PREFIX, "");
        mm.setAttribute("data-mloc", mw.loc);
        mm.setAttribute("title", mw.loc + " · " + tip);
        if (hdr) hdr.setAttribute("title", mw.loc + " · " + tip);
      }
      for (i = 0; i < found.length; i++) {
        var f = found[i], el = f.el, loc = f.loc;
        el.setAttribute("data-loc", loc);
        var ts = el.querySelectorAll("[title]");
        for (j = 0; j < ts.length; j++) {
          // Only what can be hovered. A hidden select's title is where
          // other code reads a control's NAME from (a selector knob is
          // titled from its select), and an address in it would end up in
          // the middle of that tooltip.
          if (!shown(ts[j])) continue;
          ts[j].setAttribute("title", loc + " · " + ts[j].getAttribute("title").replace(PREFIX, ""));
        }
        // A control with no tooltip of its own gets its address as one,
        // remembered as ours so the next pass replaces rather than prefixes.
        var own = el.getAttribute("title");
        if (own === null || own === "" || own === el.getAttribute("data-loctitle")) {
          el.setAttribute("title", loc);
          el.setAttribute("data-loctitle", loc);
        } else {
          el.setAttribute("title", loc + " · " + own.replace(PREFIX, ""));
        }
      }
      return found.length;
    }
  };
})()`

var fastHelper js.Value

// fastDOM is the JS helper, evaluated the first time it is asked for.
//
// There is no Go path beside it. Each pass here used to keep one for a page
// served with a Content-Security-Policy that forbids eval, and no page this
// rack is served from sets one: the fallbacks were a second copy of every
// pass, run by nothing.
func fastDOM() js.Value {
	if !fastHelper.Truthy() {
		fastHelper = js.Global().Call("eval", fastSource)
	}
	return fastHelper
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

// fitModuleParts sets every module in f to the slots its widest part needs,
// with the reads and the writes separated by the decision instead of
// alternating with it.
//
// Reading a module's dials and then setting its minimum, module by module,
// made the browser recompute style and layout for the whole rack between
// every pair — measured, 113 forced layouts in one model change, 282ms of
// layout and 295ms of style against 694ms of script. Everything is measured
// first, Go decides, and the answers go back in one write pass.
func fitModuleParts(f js.Value) (changed bool) {
	h := fastDOM()
	raw := h.Call("moduleParts", f, "sect").String()
	var parts []*modulePart
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		return false
	}
	vals := make([]*string, len(parts))
	for i, p := range parts {
		if p == nil {
			continue
		}
		// One slot is the floor already — .sect carries min-width:--mod-w —
		// so a module that fits says nothing, rather than saying the same
		// thing twice in two places that can drift apart.
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
		return false
	}
	h.Call("setMinWidths", f, "sect", string(b))
	return changed
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
	Keep   bool            `json:"k"` // fitted already with these inputs (skirtSig)
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
	Keep  bool             `json:"keep,omitempty"`
}

// layoutSkirtsIn sizes the skirts on every knob under root — one stack, the
// stacks in an element, or with root undefined the whole page — nesting each
// stack's rings outward.
//
// Outward in DOM order, because a concentric control carries concentric
// skirts: the inner knob's positions are engraved inside the outer knob's,
// and each ring has to clear not just the grip but everything already
// placed around it.
//
// The measuring and the applying are JavaScript (skirtRead, skirtWrite) and
// the fitting is here, where skirt.Fit is tested: from Go the measuring was
// two hundred and fifty round trips a pass.
func layoutSkirtsIn(root js.Value) {
	h := fastDOM()
	raw := h.Call("skirtRead", root).String()
	var stacks []skirtStackRead
	if err := json.Unmarshal([]byte(raw), &stacks); err != nil {
		return
	}
	gap := skirtGapPx()
	out := make([]skirtStackWrite, 0, len(stacks))
	for _, s := range stacks {
		if s.Keep {
			out = append(out, skirtStackWrite{Keep: true})
			continue
		}
		// Not laid out yet — a detached subtree, a module switched out, a
		// panel not yet shown. Estimate rather than bail: a ring that is
		// never laid out has no positions at all and its legends sit on the
		// origin in a heap.
		clearance := s.Grip
		if clearance <= 0 {
			clearance = estGripRadiusPx()
		}
		w := skirtStackWrite{BI: s.Big, Ring: s.IsRing, Dials: make([]skirtDialWrite, 0, len(s.Dials))}
		for di, d := range s.Dials {
			labs := make([]skirt.Label, 0, len(d.Labs))
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
				labs = append(labs, skirt.Label{W: lw, H: lh, Deg: deg})
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
			minGrip := clearance
			if di == 0 {
				minGrip = clearance * skirt.MinGripFrac
			}
			room := 0.0
			if d.CellW > 0 {
				room = (d.CellW - d.Pad + skirtCellGapPx*layout.scale) / 2
			}
			if room > 0 {
				room -= gap
			}
			useGrip, scale := skirt.Fit(clearance, minGrip, gap, room, labs)
			if scale < 1 {
				labs = skirt.ScaleLabels(labs, scale)
				dw.Font = pxStr(skirtLabelBasePx * layout.scale * scale)
			}
			if useGrip < clearance && useGrip > 0 {
				dw.Box = "" // set below; the grip is the stack's, not the dial's
				w.Grip = strconv.FormatFloat(useGrip/clearance, 'f', 3, 64)
			}
			clearance = useGrip
			r := skirt.Radius(clearance, gap, labs)
			o := skirt.Outer(r, labs)
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
			clearance = o
		}
		out = append(out, w)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	h.Call("skirtWrite", root, string(b))
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

// tipBatch is tooltip stamping batched into one pass.
type tipBatch struct {
	queue []tipStamp
	cell  int

	// reads is the panel as the last read pass found it, indexed the same way
	// the stamps are.
	reads []cellRead
}

var tips tipBatch

// queueStamp collects a tooltip write, for flushStamps to make.
//
// The whole annotate pass is a few thousand of these — ten or so per control
// over some three hundred controls — and each one from Go was a
// querySelectorAll whose NodeList and every element in it arrived as a
// js.Value with a finalizer attached. Measured, two thirds of the pass was
// runtime.addspecial. Collected and sent once, it is a single crossing.
func queueStamp(sel, title string, nth int) {
	tips.queueAttr(sel, "title", title, nth)
}

// queueAttr is queueStamp for an attribute other than the tooltip.
func (t *tipBatch) queueAttr(sel, attr, value string, nth int) {
	t.queue = append(t.queue, tipStamp{Cell: t.cell, Sel: sel, Title: value, Nth: nth, Attr: attr})
}

// flushStamps applies every queued tooltip in one crossing.
func (t *tipBatch) flushStamps() {
	q := t.queue
	t.queue = t.queue[:0]
	if len(q) == 0 {
		return
	}
	h := fastDOM()
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
	Done  bool      `json:"done"` // stamped already, and unchanged since (tipRead)
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
	VH    string    `json:"vh"`   // the value readout's own description, if the markup wrote one
	VHNew bool      `json:"vhn"`  // …found in its title, and not yet kept in data-help
	CT    string    `json:"ct"`   // the cell's own title, without its address
	RT    string    `json:"rt"`   // the hidden slider's authored title, if it has one
	RTNew bool      `json:"rtn"`  // …found in its title, and not yet kept in data-help
}

// readPanelCells measures the whole panel's cells in one crossing.
func (t *tipBatch) readPanelCells(h js.Value) {
	t.reads = t.reads[:0]
	if err := json.Unmarshal([]byte(h.Call("tipRead", owed.modelChange).String()), &t.reads); err != nil {
		t.reads = t.reads[:0]
	}
}

// cellFacts is what the read pass found for this control's cell. The pass
// enumerates the cells exactly as buildControlModel does, so every control
// has an entry; an empty one stands in only if the read itself failed.
func (c *Control) cellFacts() *cellRead {
	if c.tipIdx < 0 || c.tipIdx >= len(tips.reads) {
		return &cellRead{}
	}
	return &tips.reads[c.tipIdx]
}
