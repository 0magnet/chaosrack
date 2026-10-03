//go:build js && wasm

package attractor

import "syscall/js"

// The manual's modules (manualmode_js.go): the rack's own, working, moved
// out of the rack into the manual where each is described, and from there
// into a window of their own and back. Not pictures of them: a knob turned
// in the manual is the knob, and the model and the meters answer it.
//
// A module is dressed by the stylesheet partly through what it stands in —
// the panel's sizes (--mm), its knob face, the frame and the opening around
// it. So each one moves inside a wrapper standing in for those: copies of
// its ancestors' classes and inline style, none of their ids (the panel's
// is a class, .cp-ctx, that the stylesheet treats as the panel), nothing
// positioned, scrolled or transformed. A change to the panel's classes, a
// new knob face, is copied to every wrapper as it happens.
//
// Laid out at the rack's own size and then zoomed to fit where it is, which
// keeps a knob's drag and a pin's click where the eye says they are. Each
// in the manual carries its addresses: a tag at every control's top left,
// over the module and through it (it takes no pointer), and the entry
// below lights the tag it is read with.
//
// In JavaScript, as the parts catalog is: moving, measuring and drawing a
// few hundred elements is DOM work, which costs Go/wasm far more.

// manualLive is the page's half of the manual's modules, made the first time
// it is asked for.
func manualLive() js.Value {
	f := js.Global().Get("__manualLive")
	if !f.Truthy() {
		f = js.Global().Get("Function").New(manualLiveJS).Invoke()
		js.Global().Set("__manualLive", f)
	}
	return f
}

const manualLiveJS = `
var doc = document, LETTERS = 'abcdefghijklmnopqrstuvwxyz';
var bays = [], mods = {}, wraps = {}, holds = [];
var panel = doc.getElementById('controls-panel');
function rel(r, b) { return { x: r.left - b.left, y: r.top - b.top, w: r.width, h: r.height }; }
function svg(tag, attrs) {
  var e = doc.createElementNS('http://www.w3.org/2000/svg', tag);
  for (var k in attrs) e.setAttribute(k, attrs[k]);
  return e;
}
function anchor(loc) { return 'c-' + loc.replace(/\./g, '-'); }
// The wrapper's classes: the panel's, and the one the stylesheet reads as
// the panel's.
function ctxClass() { return 'cp-ctx mlive-ctx ' + panel.className; }

// The panel's classes, as they change, on every wrapper.
new MutationObserver(function () {
  for (var k in wraps) wraps[k].className = ctxClass();
}).observe(panel, { attributes: true, attributeFilter: ['class'] });

return {
  // measure records the live rack's bays and modules, before any is moved:
  // each module's name, address and natural width, and where it stands in
  // its bay, in the bay's own unscaled pixels.
  measure: function () {
    bays = []; mods = {};
    [].filter.call(doc.querySelectorAll('#controls-panel .runit'), function (u) { return u.offsetParent; }).forEach(function (u, i) {
      var ur = u.getBoundingClientRect(), s = ur.width / (u.offsetWidth || 1);
      var b = { w: u.offsetWidth, h: u.offsetHeight, mods: [] };
      [].forEach.call(u.querySelectorAll('.sect[data-mloc]'), function (m) {
        if (!m.offsetParent) return;
        var mr = m.getBoundingClientRect(), h = m.querySelector('.sect-hdr');
        var mod = { el: m, loc: m.getAttribute('data-mloc'), name: h ? h.textContent : '', w: m.offsetWidth, h: m.offsetHeight,
          x: (mr.left - ur.left) / s, y: (mr.top - ur.top) / s };
        b.mods.push(mod);
        mods[mod.loc] = mod;
      });
      bays[i + 1] = b;
    });
    return bays.length - 1;
  },

  // wrapper is module loc in its wrapper, made the first time: the module
  // leaves the rack then, for good.
  wrapper: function (loc) {
    if (wraps[loc]) return wraps[loc];
    var m = mods[loc];
    if (!m) return null;
    var ctx = doc.createElement('div');
    ctx.className = ctxClass();
    ctx.style.cssText = panel.style.cssText;
    var s = ctx.style;
    s.position = 'relative'; s.inset = 'auto'; s.left = s.top = s.right = s.bottom = 'auto';
    s.width = 'auto'; s.height = 'auto'; s.maxHeight = 'none'; s.overflow = 'visible';
    s.margin = '0'; s.padding = '0'; s.background = 'transparent'; s.display = 'inline-block';
    s.transform = 'none'; s.zIndex = 'auto';
    var at = ctx;
    // The frame, the unit and the opening, as shallow copies.
    var chain = [];
    for (var p = m.el.parentElement; p && p !== panel; p = p.parentElement) chain.unshift(p);
    chain.forEach(function (p) {
      var c = p.cloneNode(false);
      c.removeAttribute('id');
      var cs = c.style;
      cs.position = 'relative'; cs.transform = 'none'; cs.margin = '0'; cs.height = 'auto';
      cs.overflow = 'visible'; cs.left = cs.top = 'auto';
      // As wide as the module, not as the rack: the opening is sized from a
      // variable the frame carries.
      cs.width = 'max-content'; cs.minWidth = '0'; cs.maxWidth = 'none'; cs.flex = 'none';
      cs.setProperty('--open-w', 'auto');
      at.appendChild(c);
      at = c;
    });
    m.el.style.flex = 'none';
    at.appendChild(m.el);
    ctx.setAttribute('data-mloc', loc);
    wraps[loc] = ctx;
    return ctx;
  },

  // naturalWidth is module loc's width at the rack's own size.
  naturalWidth: function (loc) { return mods[loc] ? mods[loc].w : 0; },

  // fit zooms module loc's wrapper to width, never above its own size.
  fit: function (loc, width) {
    var w = wraps[loc], m = mods[loc];
    if (!w || !m || width <= 0) return;
    w.style.zoom = String(Math.min(1, width / m.w));
  },

  // place puts every module whose placeholder is in root there, and fits it.
  place: function (root) {
    var self = this;
    [].forEach.call(root.querySelectorAll('.mlive[data-mloc]'), function (h) {
      var loc = h.getAttribute('data-mloc'), w = self.wrapper(loc);
      if (!w) return;
      var stage = doc.createElement('div');
      stage.className = 'mlive-stage';
      stage.appendChild(w);
      h.appendChild(stage);
      holds.push(h);
    });
    this.refit();
  },

  // home puts module loc back in its place in the manual.
  home: function (loc) {
    var h = doc.querySelector('.mlive[data-mloc="' + loc + '"]'), w = wraps[loc];
    if (!h || !w) return;
    h.querySelector('.mlive-stage').appendChild(w);
    h.classList.remove('away');
    this.refit(h);
  },

  // away marks module loc's place as empty: the module is in a window.
  away: function (loc) {
    var h = doc.querySelector('.mlive[data-mloc="' + loc + '"]');
    if (h) h.classList.add('away');
  },

  // refit fits the modules in the manual to its column (or only hold), and
  // redraws their addresses.
  refit: function (only) {
    var self = this;
    (only ? [only] : holds).forEach(function (h) {
      if (h.classList.contains('away')) return;
      var loc = h.getAttribute('data-mloc');
      self.fit(loc, h.clientWidth);
      self.callouts(h);
    });
  },

  // callouts draws module loc's addresses over it, in hold: a tag at each
  // control's top left, moved down past any tag already there.
  callouts: function (h) {
    var old = h.querySelector('.mlive-over');
    if (old) old.remove();
    var w = wraps[h.getAttribute('data-mloc')];
    if (!w || h.classList.contains('away')) return;
    var hr = h.getBoundingClientRect(), o = svg('svg', { class: 'mlive-over', width: hr.width, height: hr.height });
    var taken = [], seen = {};
    var cs = [].filter.call(w.querySelectorAll('[data-loc]'), function (c) {
      var l = c.getAttribute('data-loc');
      if (seen[l] || !c.getClientRects().length) return false;
      seen[l] = 1;
      return true;
    }).map(function (c) { return { loc: c.getAttribute('data-loc'), r: rel(c.getBoundingClientRect(), hr) }; });
    cs.sort(function (a, b) { return a.r.y - b.r.y || a.r.x - b.r.x; });
    cs.forEach(function (c) {
      var short = c.loc.split('.').slice(1).join('.');
      var x = Math.max(1, c.r.x), y = Math.max(10, c.r.y + 1), tw = 6.2 * short.length + 4;
      for (var i = 0; i < taken.length; i++) {
        var t = taken[i];
        if (x < t.x + t.w && t.x < x + tw && Math.abs(y - t.y) < 11) { y = t.y + 11; i = -1; }
      }
      taken.push({ x: x, y: y, w: tw });
      var g = svg('g', { class: 'mlive-call', 'data-loc': c.loc });
      g.appendChild(svg('rect', { x: x, y: y - 9, width: tw, height: 11, rx: 2, class: 'mlive-tag' }));
      var t = svg('text', { x: x + 2, y: y, class: 'mlive-loc' });
      t.textContent = short;
      g.appendChild(t);
      o.appendChild(g);
    });
    h.appendChild(o);
  },

  // maps draws each bay's map in root: its modules where they stand in it,
  // each named and addressed, and a link to where the manual has it.
  maps: function (root) {
    [].forEach.call(root.querySelectorAll('.mbaymap[data-bay]'), function (f) {
      var b = bays[+f.getAttribute('data-bay')];
      if (!b) return;
      var W = f.clientWidth || 860, s = W / b.w, H = Math.max(40, b.h * s);
      var o = svg('svg', { width: W, height: H, class: 'mbaymap-svg' });
      b.mods.forEach(function (m) {
        var a = svg('a', { href: '#' + anchor(m.loc) });
        a.appendChild(svg('rect', { x: m.x * s + 1, y: m.y * s + 1, width: Math.max(2, m.w * s - 2), height: Math.max(2, m.h * s - 2), rx: 3, class: 'mbaymap-mod' }));
        var t = svg('text', { x: m.x * s + 6, y: m.y * s + 16, class: 'mbaymap-loc' });
        t.textContent = m.loc;
        a.appendChild(t);
        var n = svg('text', { x: m.x * s + 6, y: m.y * s + 31, class: 'mbaymap-name' });
        n.textContent = m.name;
        a.appendChild(n);
        o.appendChild(a);
      });
      f.appendChild(o);
    });
  },

  // link lights a control's tag while its entry is read, and its entry
  // while the control is hovered in the manual.
  link: function (root) {
    var self = this;
    self.linkText(root);
    // A module redraws its tags when the pointer comes to it: the bank
    // reprograms itself with the model, and a knob's readout grows.
    holds.forEach(function (h) { h.addEventListener('mouseenter', function () { self.callouts(h); }); });
    var t;
    addEventListener('resize', function () { clearTimeout(t); t = setTimeout(function () { self.refit(); }, 150); });
  },

  // linkText lights a control's tag while its entry in root is read; root
  // is the page, or one module's text written again (manualRefresh).
  linkText: function (root) {
    [].forEach.call(root.querySelectorAll('dt[id^="c-"]'), function (dt) {
      var loc = dt.textContent;
      var on = function (v) {
        [].forEach.call(doc.querySelectorAll('.mlive-call[data-loc="' + loc + '"]'), function (g) { g.classList.toggle('hot', v); });
        dt.classList.toggle('hot', v);
      };
      dt.addEventListener('mouseenter', function () { on(true); });
      dt.addEventListener('mouseleave', function () { on(false); });
      var dd = dt.nextElementSibling;
      if (dd) { dd.addEventListener('mouseenter', function () { on(true); }); dd.addEventListener('mouseleave', function () { on(false); }); }
    });
  },

  // module is the live module with address loc, wherever it is now.
  module: function (loc) {
    return mods[loc] ? mods[loc].el : null;
  }
};
`
