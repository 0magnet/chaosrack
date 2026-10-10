// The rack's panel as a picture, for a front end that is not a browser.
//
// Evaluated in the page; defines window.__rackpic and nothing else. See
// rackpic.go for what the picture is and why it is taken this way.
(function () {
  // The version is the script's own hash, put in by rackpic.go: a page that
  // has an older one drops it, observer and all.
  var V = "__RACKPIC_VERSION__";
  if (window.__rackpic) {
    if (window.__rackpic.v === V) return;
    if (window.__rackpic.obs) window.__rackpic.obs.disconnect();
  }

  var R = {
    v: V,
    gen: 0,
    els: [],              // every element of the frame, by index, at this gen
    rects: [],            // their boxes, in frame pixels, for hit tests
    inputs: [],           // indexes of the inputs and selects, read every time
    texts: [],            // the text last sent for each of those
    at: new WeakMap(),    // element -> index
    dirty: new Set(),     // elements whose look changed since the last read
    rescan: new Set(),    // elements whose children changed
    full: true,           // the next read starts again
    frame: null,
    key: ""               // size and scale of the frame at the last full read
  };
  window.__rackpic = R;

  function frameEl() { return document.querySelector(".rack-frame"); }
  function kscale() {
    return parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--kscale")) || 1;
  }

  // A CSS color as 0xRRGGBB, or -1 for none (absent, or all but transparent).
  // A color with some transparency is mixed with black at its alpha: the panel
  // is dark, and that is near enough to what it is drawn over.
  function rgb(s) {
    var m = /rgba?\(([^)]+)\)/.exec(s || "");
    if (!m) return -1;
    var p = m[1].split(",").map(Number);
    var a = p.length === 4 ? p[3] : 1;
    if (a < 0.1) return -1;
    return ((p[0] * a) << 16) | ((p[1] * a) << 8) | (p[2] * a);
  }
  // The fill of a gradient: its second stop, which for the panel's knob
  // bodies (a highlight, the body, a shadow) is the body.
  function gradient(s) {
    var m = (s || "").match(/rgba?\([^)]+\)/g);
    if (!m) return -1;
    return rgb(m[Math.min(1, m.length - 1)]);
  }

  // conicTicks reads each repeating-conic-gradient(c 0deg, c Wdeg, transparent
  // Wdeg, transparent Pdeg) as [period P, width W, color].
  function conicTicks(img) {
    var out = [], parts = img.split("repeating-conic-gradient(").slice(1);
    for (var i = 0; i < parts.length; i++) {
      var c = /^(rgba?\([^)]+\)) ([\d.]+)deg/.exec(parts[i]);
      var ds = (parts[i].match(/([\d.]+)deg/g) || []).slice(0, 4);
      if (!c || ds.length < 4) continue;
      var col = rgb(c[1]), width = parseFloat(ds[1]), period = parseFloat(ds[3]);
      if (col >= 0 && period > 0) out.push([period, width, col]);
    }
    return out;
  }

  function box(e, fr, k) {
    var r = e.getBoundingClientRect();
    return [(r.left - fr.left) / k, (r.top - fr.top) / k, r.width / k, r.height / k];
  }
  function round1(v) { return Math.round(v * 10) / 10; }

  // textOf is what an element shows as text of its own: an input's value, a
  // select's choice, or the text directly inside it (not its children's).
  function textOf(e) {
    var tag = e.tagName;
    if (tag === "INPUT") {
      var ty = e.type;
      return ty === "checkbox" || ty === "radio" || ty === "range" || ty === "hidden" ? "" : e.value;
    }
    if (tag === "SELECT") {
      var o = e.selectedOptions && e.selectedOptions[0];
      return o ? o.textContent : "";
    }
    if (tag === "OPTION" || tag === "path" || tag === "svg" || tag === "STYLE" || tag === "SCRIPT") return "";
    var t = "";
    for (var n = e.firstChild; n; n = n.nextSibling) if (n.nodeType === 3) t += n.data;
    return t.trim();
  }

  // item is what one element contributes to the picture, or null.
  function item(e, fr, k) {
    var b = box(e, fr, k);
    if (b[2] < 0.5 || b[3] < 0.5) return null;
    var cs = getComputedStyle(e);
    if (cs.visibility === "hidden" || cs.display === "none" || parseFloat(cs.opacity) === 0) return null;
    var it = { x: round1(b[0]), y: round1(b[1]), w: round1(b[2]), h: round1(b[3]) };
    var any = false;

    var bg = rgb(cs.backgroundColor);
    var img = cs.backgroundImage, mask = cs.maskImage || cs.webkitMaskImage || "none";
    if (img.indexOf("conic-gradient") >= 0) {
      // Tick marks: a repeating conic gradient, a thin colored wedge every
      // so many degrees, masked to a ring. Sent as what they are.
      var ticks = conicTicks(img);
      if (ticks.length) {
        it.k = ticks;
        var mi = /rgba?\(0, 0, 0, 0\) ([\d.]+)%/.exec(mask);
        it.ki = mi ? round1(parseFloat(mi[1]) / 100) : 0.7;
        any = true;
      }
    } else if (bg < 0 && img.indexOf("gradient") >= 0 && (mask === "none" || e.classList.contains("knob-ptr"))) {
      bg = gradient(img);
    }
    if (bg >= 0) { it.b = bg; any = true; }
    if (cs.borderRadius.indexOf("50%") >= 0 || parseFloat(cs.borderRadius) >= Math.min(b[2], b[3]) * k / 2) it.r = 1;
    if (parseFloat(cs.borderTopWidth) >= 1 && cs.borderTopStyle !== "none") {
      var bo = rgb(cs.borderTopColor);
      if (bo >= 0 && bo !== bg) { it.o = bo; any = true; }
    }

    var t = textOf(e);
    // DSEG7, the LED displays' face, spells an unlit digit "!".
    if (t && cs.fontFamily.indexOf("DSEG") >= 0) t = t.replace(/!/g, " ");
    var dmdInk = -1;
    if (e.classList.contains("dmdwin")) {
      // A dot-matrix display draws its words in SVG dots; what it says is on
      // the SVG for screen readers, and its color is its lit dots'.
      var svg = e.querySelector("svg");
      t = svg ? svg.getAttribute("aria-label") || "" : "";
      // A marquee's label is the whole of what scrolls through it; the
      // display shows data-chars of it.
      var nch = parseInt(e.getAttribute("data-chars"), 10);
      if (nch > 0 && t.length > nch) t = t.slice(0, nch);
      t = t.replace(/\u00a0/g, " ").replace(/\s+$/, "");
      var on = e.querySelector(".dmd-on");
      if (on) dmdInk = rgb(getComputedStyle(on).fill);
      if (e.classList.contains("dmdv")) it.v = 1;
    }
    if (t) {
      var fg = dmdInk >= 0 ? dmdInk : rgb(cs.color);
      if (fg >= 0) {
        it.t = t.slice(0, 40);
        it.f = fg;
        if (cs.textAlign === "right" || cs.textAlign === "end") it.al = "r";
        else if (cs.textAlign === "center") it.al = "c";
        any = true;
      }
    }

    // A canvas is a picture of its own: the item says where it is, and
    // R.canvases samples it at the size a front end asks for.
    if (e.tagName === "CANVAS") { it.cv = 1; any = true; }

    if (e.classList.contains("knob-ptr")) {
      var a = parseFloat(e.style.getPropertyValue("--a"));
      if (!isNaN(a)) { it.a = a; any = true; }
    }
    return any ? it : null;
  }

  function hit(e, fr, k) {
    var b = box(e, fr, k);
    return b[2] < 0.5 || b[3] < 0.5 ? null : b;
  }

  function watch(f) {
    if (R.obs) R.obs.disconnect();
    R.obs = new MutationObserver(function (list) {
      for (var i = 0; i < list.length; i++) {
        var m = list[i];
        if (m.type === "childList") R.rescan.add(m.target);
        else if (m.type === "characterData") { if (m.target.parentElement) R.dirty.add(m.target.parentElement); }
        else if (m.attributeName === "aria-label") {
          // A display was rewritten: its words are the display's item.
          var w = m.target.closest && m.target.closest(".dmdwin");
          if (w) R.dirty.add(w);
        }
        else {
          // A class or a style can restyle everything under it: a small
          // subtree is read again element by element, a big one rescanned.
          var t = m.target;
          if (t.getElementsByTagName("*").length > 40) R.rescan.add(t);
          else { R.dirty.add(t); var d = t.getElementsByTagName("*"); for (var j = 0; j < d.length; j++) R.dirty.add(d[j]); }
        }
      }
    });
    R.obs.observe(f, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ["class", "style", "hidden", "aria-label"] });
  }

  // add indexes an element not seen before, or returns the index it has.
  function add(e) {
    var i = R.at.get(e);
    if (i !== undefined) return i;
    i = R.els.length;
    R.els.push(e);
    R.at.set(e, i);
    if (e.tagName === "INPUT" || e.tagName === "SELECT") { R.inputs.push(i); R.texts[i] = textOf(e); }
    return i;
  }

  R.picture = function () {
    var f = frameEl();
    if (!f) return JSON.stringify({ err: "the page has no rack panel" });
    var fr = f.getBoundingClientRect(), k = kscale();
    R.gen++;
    R.els = []; R.rects = []; R.inputs = []; R.texts = []; R.at = new WeakMap();
    R.dirty.clear(); R.rescan.clear(); R.full = false;
    R.frame = f; R.key = fr.width + "x" + fr.height + "@" + k;
    var items = [];
    var all = f.getElementsByTagName("*");
    for (var i = 0; i < all.length; i++) {
      var e = all[i], n = add(e);
      R.rects[n] = hit(e, fr, k);
      items[n] = item(e, fr, k);
    }
    watch(f);
    return JSON.stringify({ gen: R.gen, w: round1(fr.width / k), h: round1(fr.height / k), items: items, ctls: ctls(fr, k) });
  };

  // ctls is where each control's part is: the addressed cell it sits in, or
  // the control itself where it has no address.
  function ctls(fr, k) {
    var out = {};
    var l = window.rackctl && window.rackctl.list ? window.rackctl.list() : [];
    if (typeof l === "string") l = JSON.parse(l);
    l = Array.from(l || []);
    for (var i = 0; i < l.length; i++) {
      var id = l[i].id || l[i].ID, el = id && document.getElementById(id);
      if (!el) continue;
      var part = el.closest("[data-loc]") || el;
      var b = hit(part, fr, k);
      if (b) out[id] = b.map(round1);
    }
    return out;
  }

  // changes is what is different since gen: the items of the elements that
  // changed, null for one that went, or the whole picture when the layout
  // moved under it.
  R.changes = function (gen) {
    var f = frameEl();
    if (!f) return JSON.stringify({ err: "the page has no rack panel" });
    var fr = f.getBoundingClientRect(), k = kscale();
    if (R.full || gen !== R.gen || f !== R.frame || R.key !== fr.width + "x" + fr.height + "@" + k) {
      return JSON.stringify({ full: JSON.parse(R.picture()) });
    }
    var touched = new Set(R.dirty);
    R.rescan.forEach(function (t) {
      touched.add(t);
      var d = t.getElementsByTagName("*");
      for (var j = 0; j < d.length; j++) touched.add(d[j]);
    });
    R.dirty.clear(); R.rescan.clear();
    var items = {}, n = 0;
    // Gone: an element of the last picture taken out of the panel.
    touched.forEach(function (e) {
      if (!f.contains(e)) {
        var i = R.at.get(e);
        if (i !== undefined && R.rects[i]) { R.rects[i] = null; items[i] = null; n++; }
        return;
      }
      var i = add(e);
      R.rects[i] = hit(e, fr, k);
      items[i] = item(e, fr, k);
      n++;
    });
    // Typing into a field, or a program setting its value, is not a
    // mutation: the fields are read every time, and sent when they differ.
    for (var j = 0; j < R.inputs.length; j++) {
      var i = R.inputs[j], e = R.els[i];
      if (!e || i in items || !f.contains(e)) continue;
      var t = textOf(e);
      if (t !== R.texts[i]) { R.texts[i] = t; items[i] = item(e, fr, k); n++; }
    }
    return JSON.stringify({ gen: R.gen, items: n ? items : undefined });
  };

  // ── canvases ──

  // sample draws src (or the part sx, sy, sw, sh of it) into w x h pixels and
  // returns them as RGB, base64, row by row from the top. The scene's GL
  // context keeps its drawing buffer (glctx), so a WebGL canvas reads back.
  var off = null;
  function sample(src, sx, sy, sw, sh, w, h) {
    w = Math.max(1, Math.min(2048, w | 0)); h = Math.max(1, Math.min(2048, h | 0));
    if (!off) off = document.createElement("canvas");
    if (off.width !== w) off.width = w;
    if (off.height !== h) off.height = h;
    // No willReadFrequently: that keeps the small canvas on the CPU, so the
    // whole scene came back from the GPU to be shrunk there. Shrunk on the
    // GPU, only the small result is read back.
    var g = off.getContext("2d");
    g.imageSmoothingEnabled = true;
    g.imageSmoothingQuality = "high";
    g.fillStyle = "#000";
    g.fillRect(0, 0, w, h);
    try { g.drawImage(src, sx, sy, sw, sh, 0, 0, w, h); } catch (e) { return null; }
    var d = g.getImageData(0, 0, w, h).data, rgb = new Uint8Array(w * h * 3);
    for (var i = 0, j = 0; i < d.length; i += 4) { rgb[j++] = d[i]; rgb[j++] = d[i + 1]; rgb[j++] = d[i + 2]; }
    var s = "";
    for (var k = 0; k < rgb.length; k += 0x8000) s += String.fromCharCode.apply(null, rgb.subarray(k, k + 0x8000));
    return { w: w, h: h, px: btoa(s) };
  }

  // crop is the part of the scene a w x h window of pixels shape times as
  // tall as wide shows: the middle of it, cut to the window's shape.
  function crop(c, w, h, shape) {
    var want = (w / (h * (shape || 1))), have = c.width / c.height;
    var sx = 0, sy = 0, sw = c.width, sh = c.height;
    if (have > want) { sw = c.height * want; sx = (c.width - sw) / 2; }
    else { sh = c.width / want; sy = (c.height - sh) / 2; }
    return [sx, sy, sw, sh];
  }

  // scene is the model's canvas, the page's whole background, as w x h
  // pixels of the given shape (a pixel's height over its width): the middle
  // of it, cropped to that shape, as a window of another shape shows it.
  R.scene = function (w, h, shape) {
    var c = document.getElementById("gocanvas");
    if (!c || !c.width || !c.height) return JSON.stringify({ err: "the page has no scene" });
    var k = crop(c, w, h, shape);
    return JSON.stringify(sample(c, k[0], k[1], k[2], k[3], w, h) || { err: "the scene could not be read" });
  };

  // sceneAct does to the model what the pointer did at x, y of the scene as
  // scene(w, h, shape) last showed it: "down", "move" and "up" are a drag,
  // which turns the model, and "wheel" zooms it — the page's own handlers,
  // given the events a hand sends, at the same point of the canvas.
  R.sceneAct = function (w, h, shape, x, y, kind, delta) {
    var c = document.getElementById("gocanvas");
    if (!c || !c.width || !c.height) return JSON.stringify({ err: "the page has no scene" });
    var k = crop(c, w, h, shape), b = c.getBoundingClientRect();
    var o = { bubbles: true, cancelable: true, composed: true, view: window, button: 0,
      clientX: b.left + (k[0] + x / w * k[2]) * b.width / c.width,
      clientY: b.top + (k[1] + y / h * k[3]) * b.height / c.height };
    switch (kind) {
      case "wheel":
        o.deltaY = delta; o.deltaMode = 0;
        c.dispatchEvent(new WheelEvent("wheel", o));
        break;
      case "down":
        o.buttons = 1;
        c.dispatchEvent(new MouseEvent("mousedown", o));
        break;
      case "move":
        o.buttons = 1;
        c.dispatchEvent(new MouseEvent("mousemove", o));
        break;
      case "up":
        o.buttons = 0;
        c.dispatchEvent(new MouseEvent("mouseup", o));
        break;
    }
    return JSON.stringify({});
  };

  // canvases samples the canvases of the picture of generation gen that a
  // front end asks for, [[index, w, h], ...], each whole into w x h.
  R.canvases = function (gen, want) {
    if (gen !== R.gen) return JSON.stringify({ err: "the panel changed" });
    var out = {};
    for (var i = 0; i < want.length; i++) {
      var e = R.els[want[i][0]];
      if (!e || e.tagName !== "CANVAS" || !e.width || !e.height) continue;
      var img = sample(e, 0, 0, e.width, e.height, want[i][1], want[i][2]);
      if (img) out[want[i][0]] = img;
    }
    return JSON.stringify({ imgs: out });
  };

  // act does to the page what the pointer did to the picture, on the
  // deepest element under it: a click is the page's own click, a wheel its
  // own wheel. Nothing is simulated beyond the events a hand sends.
  R.act = function (gen, x, y, kind, delta) {
    if (gen !== R.gen) return JSON.stringify({ err: "the panel changed; try again" });
    var e = null;
    for (var i = R.els.length - 1; i >= 0; i--) {
      var b = R.rects[i];
      if (b && x >= b[0] && x < b[0] + b[2] && y >= b[1] && y < b[1] + b[3] && R.frame.contains(R.els[i])) { e = R.els[i]; break; }
    }
    if (!e) return JSON.stringify({});
    var fr = R.frame.getBoundingClientRect(), k = kscale();
    var o = { bubbles: true, cancelable: true, composed: true, view: window,
      clientX: fr.left + x * k, clientY: fr.top + y * k, button: 0 };
    if (kind === "wheel") {
      o.deltaY = delta; o.deltaMode = 0;
      e.dispatchEvent(new WheelEvent("wheel", o));
    } else {
      o.buttons = 1; o.pointerId = 1; o.pointerType = "mouse"; o.isPrimary = true;
      e.dispatchEvent(new PointerEvent("pointerdown", o));
      e.dispatchEvent(new MouseEvent("mousedown", o));
      o.buttons = 0;
      e.dispatchEvent(new PointerEvent("pointerup", o));
      e.dispatchEvent(new MouseEvent("mouseup", o));
      e.dispatchEvent(new MouseEvent("click", o));
    }
    return JSON.stringify({});
  };
})();
