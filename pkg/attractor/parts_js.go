//go:build js && wasm

package attractor

import "syscall/js"

// The parts catalog: every distinct part the rack is built from, once each,
// with how many there are and where. Open the page as /parts (the server
// sends it to /?parts). It is a question about the rack as a bill of
// materials — how many slightly different knobs, displays and buttons it
// takes — and the answer is only worth anything if it is complete, so the
// catalog turns the MODEL selector through every model and gathers what each
// one puts on the panel before it draws anything.
//
// Two parts are the same part when they are the same kind and the same
// rendered geometry: a knob's body sizes, its rings, its fine disc and the
// scale round it (ticks and positions, not the words printed there); a
// display's technology, size and character count; a button's kind and size.
// What they say is not part of it — the same display reading "σ" and "gain"
// is one part.
//
// Each part is drawn from a copy of its first instance with its computed
// style written onto every node, so it looks as it does in its module, not
// as the bare classes would make it outside one. The work is in JavaScript,
// started from here through the Function constructor as the arena check is
// (pkg/dom/arenacheck.go): walking a few thousand elements seventy times over
// is DOM reading, which costs Go/wasm far more than it costs the page.

// enablePartsCatalog starts the catalog if the page was opened with ?parts.
func enablePartsCatalog() {
	loc := js.Global().Get("location")
	if !loc.Truthy() || !js.Global().Get("URLSearchParams").New(loc.Get("search")).Call("has", "parts").Bool() {
		return
	}
	js.Global().Get("Function").New(partsJS).Invoke()
}

const partsJS = `
if (window.__partsCatalog) return;
window.__partsCatalog = true;
var sleep = function (ms) { return new Promise(function (r) { setTimeout(r, ms); }); };
var frames = function () { return new Promise(function (r) { requestAnimationFrame(function () { requestAnimationFrame(r); }); }); };
var R = Math.round;
var KINDS = [
  ['knob', 'Knobs', '.knobstack, .knobwrap, .stepknob'],
  ['display', 'Displays', '.led, input.numin, .dmdwin, .u-step, .monread'],
  ['button', 'Buttons', 'button'],
  ['switch', 'Switches', 'input.sw, .twoway'],
  ['other', 'Other parts', '.gen-piano, input[type=color], canvas']
];
var KNOBSEL = KINDS[0][2];
var parts = {}, ids = new WeakMap(), nextID = 0, pk = 0, pseudoCSS = '';
var badge = document.createElement('div');
badge.style.cssText = 'position:fixed;left:8px;bottom:8px;z-index:100001;background:#000c;color:#9fe;padding:6px 10px;font:12px monospace;border:1px solid #365;border-radius:4px';
document.body.appendChild(badge);

function vis(e) {
  if (e.closest('#parts-cat')) return false;
  var r = e.getBoundingClientRect();
  if (r.width < 1 || r.height < 1) return false;
  var cs = getComputedStyle(e);
  return cs.visibility !== 'hidden' && cs.display !== 'none' && +cs.opacity > 0;
}
function size(e) { var r = e.getBoundingClientRect(); return R(r.width) + 'x' + R(r.height); }
function where(e) {
  var l = e.closest('[data-loc]'), s = e.closest('.sect');
  var h = s && s.querySelector('.sect-hdr');
  return (l ? l.getAttribute('data-loc') + ' ' : '') + (h ? h.textContent.trim() : '');
}

// The signature: kind, then what makes it a different part. fam is the same
// without the sizes, so near-duplicates sort next to each other.
function sig(kind, e) {
  var cs = getComputedStyle(e);
  if (kind === 'knob') {
    var bodies = [].map.call(e.querySelectorAll('.knob:not(.knob-fine)'), function (k) { return R(k.getBoundingClientRect().width); });
    if (e.classList.contains('stepknob')) bodies = [R(e.getBoundingClientRect().width)];
    bodies.sort(function (a, b) { return b - a; });
    var fine = e.querySelector('.knob-fine') ? ' +fine' : '';
    var cap = e.querySelector('.knob-cap') ? ' +cap' : '';
    var dials = [].map.call(e.querySelectorAll('.knob-dial'), function (d) {
      var t = d.querySelectorAll('.vdial-tick').length, l = d.querySelectorAll('.knob-dial-lab').length, w = d.querySelectorAll('.knob-dial-wave').length;
      return (d.classList.contains('value-dial') ? 'scale' : 'ring') + '(' + [t && t + ' ticks', l && l + ' positions', w && w + ' glyphs'].filter(Boolean).join(', ') + ')';
    });
    var dsz = [].map.call(e.querySelectorAll('.knob-dial'), function (d) { return R(d.getBoundingClientRect().width); });
    var kind2 = e.classList.contains('stepknob') ? 'step knob' : bodies.length > 1 ? bodies.length + '-ring knob' : 'knob';
    var fam = kind2 + fine + cap + (dials.length ? ' ' + dials.join(' ') : ' no scale');
    return { fam: fam, sig: fam + ' | body ' + bodies.join('/') + (dsz.length ? ' dial ' + dsz.join('/') : '') };
  }
  if (kind === 'display') {
    var tech = e.classList.contains('dmdwin') ? 'dot matrix' + (e.classList.contains('dmdv') ? ' vertical' : '') : /DSEG/.test(cs.fontFamily) ? '7-segment' : 'text';
    var chars = e.getAttribute('data-chars') || '';
    var f = tech + (chars ? ' ' + chars + ' chars' : '');
    return { fam: f, sig: f + ' | ' + size(e) + (tech === '7-segment' ? ' font ' + cs.fontSize : '') };
  }
  if (kind === 'button') {
    var bk = e.classList.contains('trio-btn') ? 'lit button' : e.classList.contains('rst') ? 'reset' : (e.className || 'button');
    return { fam: bk, sig: bk + ' | ' + size(e) + ' font ' + cs.fontSize };
  }
  var name = e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.split(' ')[0] : '');
  return { fam: name, sig: name + ' | ' + size(e) };
}

// snap copies e with its computed style inlined on every node, so the copy
// looks the same anywhere.
function snap(e) {
  var c = e.cloneNode(true);
  var src = [e].concat([].slice.call(e.querySelectorAll('*'))), dst = [c].concat([].slice.call(c.querySelectorAll('*')));
  src.forEach(function (s, i) {
    var d = dst[i], cs = getComputedStyle(s), t = '';
    for (var j = 0; j < cs.length; j++) t += cs[j] + ':' + cs.getPropertyValue(cs[j]) + ';';
    d.setAttribute('style', t);
    d.removeAttribute('id');
    if ('value' in s && s.tagName === 'INPUT') d.value = s.value;
    ['before', 'after'].forEach(function (p) {
      var ps = getComputedStyle(s, '::' + p);
      if (!ps.content || ps.content === 'none' || ps.content === 'normal') return;
      if (!d.hasAttribute('data-pk')) d.setAttribute('data-pk', 'p' + (++pk));
      var u = '';
      for (var j = 0; j < ps.length; j++) u += ps[j] + ':' + ps.getPropertyValue(ps[j]) + ';';
      pseudoCSS += '#parts-cat [data-pk="' + d.getAttribute('data-pk') + '"]::' + p + '{' + u + '}\n';
    });
    if (s.tagName === 'CANVAS') { try { d.getContext('2d').drawImage(s, 0, 0); } catch (err) {} }
  });
  c.style.position = 'relative'; c.style.left = c.style.top = c.style.right = c.style.bottom = 'auto';
  c.style.margin = '0'; c.style.transform = 'none';
  return c;
}

// Positions, not parts: every module, cell and switch by a name that does
// not change with the model (its id, or its module and legend), and the
// models it is on the panel for. A part the catalog counts once can still
// come and go at one place — the Trail knob is the same knob as fifty others.
// The bank, Parameters and the readout line are left out: changing with the
// model is what they are for.
var seenAt = {};
function posName(e) {
  var s = e.closest('.sect'), h = s && s.querySelector('.sect-hdr');
  var mod = h ? h.textContent.trim() : (s && s.id) || '?';
  if (e.classList.contains('sect')) return 'module ' + mod;
  if (e.id) return mod + ' #' + e.id;
  var l = e.querySelector('.plabel,.u-lbl,.axlbl,.ledlbl,.dmdlbl') || e;
  var t = (l.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 24);
  return t ? mod + ' / ' + t : null;
}
function positions(model) {
  var root = document.getElementById('controls-panel') || document.body;
  [].forEach.call(root.querySelectorAll('.sect, .pcell, .punit, .swline'), function (e) {
    if (e.closest('.catbank, .cathead, .pu, #params, #model-readouts, #parts-cat')) return;
    var n = posName(e);
    if (!n) return;
    var a = seenAt[n] || (seenAt[n] = { on: new Set() });
    if (vis(e)) a.on.add(model);
  });
}
function harvest(model) {
  var root = document.getElementById('controls-panel') || document.body;
  KINDS.forEach(function (k) {
    [].forEach.call(root.querySelectorAll(k[2]), function (e) {
      if (!vis(e)) return;
      if (k[0] === 'knob' && e.parentElement && e.parentElement.closest(KNOBSEL)) return;
      if (k[0] !== 'knob' && e.closest(KNOBSEL) && k[0] !== 'other') return;
      var s = sig(k[0], e), key = k[0] + '|' + s.sig, p = parts[key];
      if (!p) p = parts[key] = { kind: k[0], fam: s.fam, sig: s.sig, els: new Set(), models: new Set(), locs: [], copy: snap(e) };
      if (!ids.has(e)) ids.set(e, ++nextID);
      p.els.add(ids.get(e));
      p.models.add(model);
      var w = where(e);
      if (w && p.locs.indexOf(w) < 0 && p.locs.length < 6) p.locs.push(w);
    });
  });
}

function draw(nModels) {
  var cat = document.createElement('div');
  cat.id = 'parts-cat';
  cat.style.cssText = 'position:fixed;inset:0;z-index:100000;overflow:auto;background:#0e1114;color:#c8d0d8;font:12px/1.4 monospace;padding:16px 20px';
  var st = document.createElement('style');
  st.textContent = pseudoCSS + '#parts-cat .pc-tile{display:inline-flex;flex-direction:column;align-items:center;gap:6px;vertical-align:top;margin:0 10px 14px 0;padding:10px;background:#1a1f24;border:1px solid #2b333b;border-radius:6px;max-width:260px}' +
    '#parts-cat .pc-hold{display:flex;align-items:center;justify-content:center;min-width:40px;min-height:30px;padding:12px;background:#15191d;border-radius:4px}' +
    '#parts-cat .pc-cap{font-size:11px;color:#8fa0b0;text-align:center;word-break:break-word}#parts-cat .pc-cap b{color:#e6edf3;font-weight:600}' +
    '#parts-cat h2{font:600 15px sans-serif;color:#e6edf3;margin:18px 0 4px}#parts-cat h3{font:12px monospace;color:#7fd0a0;margin:10px 0 6px}';
  cat.appendChild(st);
  var all = Object.keys(parts).map(function (k) { return parts[k]; });
  var head = document.createElement('div');
  head.innerHTML = '<div style="font:600 18px sans-serif;color:#fff">Parts catalog</div>' +
    '<div>' + all.length + ' unique parts across ' + nModels + ' models: ' +
    KINDS.map(function (k) { return all.filter(function (p) { return p.kind === k[0]; }).length + ' ' + k[1].toLowerCase(); }).join(', ') +
    '. Grouped by family; each family lists its sizes. Close: Esc.</div>';
  cat.appendChild(head);
  // Positions that come and go with the model.
  var gone = Object.keys(seenAt).filter(function (n) { var k = seenAt[n].on.size; return k > 0 && k < nModels; }).sort();
  if (gone.length) {
    var gh = document.createElement('h2');
    gh.textContent = 'Comes and goes with the model — ' + gone.length + ' places on the panel';
    cat.appendChild(gh);
    var gl = document.createElement('div');
    gl.style.cssText = 'margin:0 0 8px 0;color:#d8b070';
    gone.forEach(function (n) {
      var d = document.createElement('div');
      var on = Array.from(seenAt[n].on);
      d.textContent = n + ' — shown on ' + on.length + ' of ' + nModels + ': ' + on.slice(0, 6).join(', ') + (on.length > 6 ? ', …' : '');
      gl.appendChild(d);
    });
    cat.appendChild(gl);
  }
  window.partsPositions = gone.map(function (n) { return { place: n, on: Array.from(seenAt[n].on) }; });
  // What the MODEL knob changes: a part on some models and not others, or a
  // part that is one size on some and another on the rest. The rack is one
  // instrument whatever the model, so each line here is a shift to look at.
  var moving = all.filter(function (p) { return p.models.size < nModels; });
  if (moving.length) {
    var mh = document.createElement('h2');
    mh.textContent = 'Changes with the model — ' + moving.length + ' parts are on some models only';
    cat.appendChild(mh);
    var ul = document.createElement('div');
    ul.style.cssText = 'margin:0 0 8px 0;color:#d8b070';
    moving.sort(function (a, b) { return a.locs.join() < b.locs.join() ? -1 : 1; }).forEach(function (p) {
      var li = document.createElement('div');
      var ms = Array.from(p.models);
      li.textContent = (p.locs[0] || '?') + ' — ' + p.sig + ' — on ' + ms.length + ': ' + ms.slice(0, 8).join(', ') + (ms.length > 8 ? ', …' : '');
      ul.appendChild(li);
    });
    cat.appendChild(ul);
  }
  KINDS.forEach(function (k) {
    var ps = all.filter(function (p) { return p.kind === k[0]; });
    if (!ps.length) return;
    var h = document.createElement('h2');
    var fams = {};
    ps.forEach(function (p) { (fams[p.fam] = fams[p.fam] || []).push(p); });
    h.textContent = k[1] + ' — ' + ps.length + ' parts in ' + Object.keys(fams).length + ' families';
    cat.appendChild(h);
    Object.keys(fams).sort().forEach(function (f) {
      var h3 = document.createElement('h3');
      h3.textContent = f + ' — ' + fams[f].length + (fams[f].length > 1 ? ' sizes' : ' size');
      cat.appendChild(h3);
      fams[f].sort(function (a, b) { return a.sig < b.sig ? -1 : 1; }).forEach(function (p) {
        var t = document.createElement('div'); t.className = 'pc-tile';
        var hold = document.createElement('div'); hold.className = 'pc-hold';
        hold.appendChild(p.copy);
        var cap = document.createElement('div'); cap.className = 'pc-cap';
        cap.innerHTML = '<b></b><br><span></span><br><span></span>';
        cap.querySelector('b').textContent = p.sig.split(' | ')[1] || p.sig;
        cap.querySelectorAll('span')[0].textContent = p.els.size + ' on the rack, in ' + p.models.size + ' of ' + nModels + ' models';
        cap.querySelectorAll('span')[1].textContent = p.locs.join(' · ');
        t.appendChild(hold); t.appendChild(cap);
        cat.appendChild(t);
      });
    });
  });
  document.body.appendChild(cat);
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') cat.remove(); });
  window.partsCatalog = all.map(function (p) { return { kind: p.kind, family: p.fam, sig: p.sig, instances: p.els.size, models: p.models.size, on: Array.from(p.models), where: p.locs }; });
}

(async function () {
  var sel;
  for (var i = 0; i < 200; i++) {
    sel = document.getElementById('mode-select');
    if (sel && sel.options.length && document.querySelector('#controls-panel .sect')) break;
    await sleep(100);
  }
  if (!sel) { badge.textContent = 'parts: no model selector'; return; }
  await sleep(1500);
  var start = sel.value, models = [].map.call(sel.options, function (o) { return o.value; });
  for (var m = 0; m < models.length; m++) {
    badge.textContent = 'parts: model ' + (m + 1) + '/' + models.length + ' ' + models[m] + ' — ' + Object.keys(parts).length + ' parts so far';
    if (sel.value !== models[m]) {
      sel.value = models[m];
      sel.dispatchEvent(new Event('change', { bubbles: true }));
      await sleep(500);
    }
    await frames();
    harvest(models[m]);
    positions(models[m]);
  }
  sel.value = start;
  sel.dispatchEvent(new Event('change', { bubbles: true }));
  await sleep(500);
  badge.remove();
  draw(models.length);
})();
`
