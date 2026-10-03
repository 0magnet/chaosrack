//go:build js && wasm

package dom

import "syscall/js"

// The arena check: a released listener still attached to the page.
//
// An arena releases every func it holds, and that is only right when every one
// of them was attached to something the rebuild then replaces. A listener the
// build hung on a PERSISTENT element — one outside what is rebuilt — is freed
// while the element keeps it, and the next event on that element throws "call
// to released function". The sweep dial did exactly that for months: the
// select it listened on lived outside its arena. Nothing showed until a fuzzer
// happened to scroll that one knob after that one rebuild.
//
// This finds it at the rebuild instead of at the event. With `arenacheck` in
// the page's query string, addEventListener is wrapped to remember which
// element and event each function went to; after every arena rebuild, each
// function it released is looked up, and one still attached to an element on
// the page is reported on console.error and kept in globalThis.__domArenaLeaks.
// A walk through every model then finds every such listener, clicked or not.
//
// Off, it costs one boolean test per rebuild.
var arenaCheck js.Value

const arenaCheckJS = `
var seen = new WeakMap(), P = EventTarget.prototype, add = P.addEventListener, rm = P.removeEventListener;
P.addEventListener = function (type, fn, opt) {
  if (typeof fn === 'function' && !fn.__goShared) {
    var l = seen.get(fn);
    if (!l) { l = []; seen.set(fn, l); }
    l.push([this, type]);
  }
  return add.call(this, type, fn, opt);
};
P.removeEventListener = function (type, fn, opt) {
  var l = typeof fn === 'function' && seen.get(fn);
  if (l) for (var i = l.length - 1; i >= 0; i--) if (l[i][0] === this && l[i][1] === type) l.splice(i, 1);
  return rm.call(this, type, fn, opt);
};
function name(el) {
  if (el === globalThis) return 'window';
  if (typeof document !== 'undefined' && el === document) return 'document';
  if (!el.tagName) return String(el);
  var sect = el.closest && el.closest('.sect'), hdr = sect && sect.querySelector('.sect-hdr');
  var up = hdr ? {id: '' , t: hdr.textContent.trim().slice(0, 24)} : el.parentElement && !el.id && el.parentElement.closest('[id]');
  return el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + (up ? (up.t ? ' in [' + up.t + ']' : ' in #' + up.id) : '') +
    (typeof el.className === 'string' && el.className.trim() ? '.' + el.className.trim().split(/\s+/).join('.') : '');
}
globalThis.__domArenaLeaks = [];
return function (fns) {
  var out = [];
  for (var i = 0; i < fns.length; i++) {
    var l = seen.get(fns[i]);
    if (!l) continue;
    for (var j = 0; j < l.length; j++) {
      var el = l[j][0];
      if (el === globalThis || el.isConnected === true || (typeof document !== 'undefined' && el === document)) out.push(name(el) + ' ' + l[j][1]);
    }
    seen.delete(fns[i]);
  }
  if (out.length) {
    globalThis.__domArenaLeaks.push.apply(globalThis.__domArenaLeaks, out);
    console.error('dom: released listener still attached: ' + out.join(', '));
  }
  return out.length;
};`

// enableArenaCheck installs the wrapper when the page asked for it. Before
// any listener it should see is added, which is why Init calls it.
func enableArenaCheck() {
	if arenaCheck.Truthy() {
		return
	}
	loc := js.Global().Get("location")
	if !loc.Truthy() || !js.Global().Get("URLSearchParams").New(loc.Get("search")).Call("has", "arenacheck").Bool() {
		return
	}
	arenaCheck = js.Global().Get("Function").New(arenaCheckJS).Invoke()
}

// releaseAll releases fns, and returns what checkReleased needs afterwards:
// the functions themselves, taken BEFORE the release, since a released
// js.Func no longer has a value to take.
func releaseAll(fns []js.Func) js.Value {
	var held js.Value
	if arenaCheck.Truthy() && len(fns) > 0 {
		held = js.Global().Get("Array").New()
		for _, f := range fns {
			held.Call("push", f.Value)
		}
	}
	for _, f := range fns {
		f.Release()
	}
	return held
}

// checkReleased reports any of held still attached to the page, once the
// rebuild that released them has run: before it, everything the rebuild is
// about to replace is still attached too.
func checkReleased(held js.Value) {
	if held.Truthy() {
		arenaCheck.Invoke(held)
	}
}
