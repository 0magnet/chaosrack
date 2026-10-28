// Runtime diagnostics, for `chaosrack mem` and rackctl.mem().
//
// What the Go program cannot see about itself: how often it crosses into
// JavaScript, how many JavaScript values it is holding, and how often the
// page misses its frames. Each was found the hard way, by a preload written
// for the occasion: a finalizer per JS object handed to Go set off TinyGo's
// collections, and handles Go held kept Blink's heap from shrinking.
//
// Always on, because it is cheap: a counter per bridge import, one increment
// a call; the long frames the browser already reports. With ?diag in the
// address it also counts what crosses by name (each Get, Set and Call's
// property) and the DOM calls that make Blink objects, which costs a string
// decode a call and is for looking, not for leaving on.
window.__crDiag = (function () {
	'use strict';
	var detail = /[?&]diag(=|&|$)/.test(location.search);
	var calls = {};  // bridge import, without "syscall/js.", to calls
	var names = {};  // with ?diag: "get width", "call drawArrays", "set textContent"
	var blink = {};  // with ?diag: DOM calls that make Blink objects
	var stalls = {kind: '', n: 0, ms: 0, worst: 0};
	var goInst = null;
	var dec = new TextDecoder();

	function bump(m, k) { m[k] = (m[k] || 0) + 1; }

	// memory is the instance's linear memory, once it is running: the Go
	// build exports it as mem, TinyGo as memory.
	function memory() {
		var ex = goInst && goInst._inst && goInst._inst.exports;
		var m = ex && (ex.mem || ex.memory);
		return m ? m.buffer : null;
	}

	// nameOf reads the property or method name an import was given. TinyGo
	// passes (ptr, len) as arguments; the Go build passes one stack pointer and
	// the arguments sit after it, the name's at +16 and +24.
	function nameOf(args, at) {
		var buf = memory();
		if (!buf) return '?';
		var ptr, len;
		if (args.length === 1) {
			var dv = new DataView(buf);
			var sp = args[0] >>> 0;
			ptr = dv.getUint32(sp + 16, true);
			len = dv.getUint32(sp + 24, true);
		} else {
			ptr = args[at] >>> 0;
			len = args[at + 1] >>> 0;
		}
		if (len > 64 || ptr + len > buf.byteLength) return '?';
		return dec.decode(new Uint8Array(buf, ptr, len));
	}

	// The imports whose name argument is worth reading, and where it is for
	// TinyGo (after the result address, for the ones that store a result).
	var named = {valueGet: ['get', 1], valueSet: ['set', 1], valueTryGet: ['get', 2],
		valueTrySet: ['set', 2], valueCall: ['call', 2], valueDelete: ['delete', 1]};

	function wrap(go) {
		goInst = go;
		var g = go.importObject && (go.importObject.gojs || go.importObject.go);
		if (!g) return;
		Object.keys(g).forEach(function (k) {
			var f = g[k];
			if (typeof f !== 'function' || k.indexOf('syscall/js.') !== 0) return;
			var short = k.slice(11);
			var n = named[short];
			g[k] = detail && n ? function () {
				bump(calls, short);
				bump(names, n[0] + ' ' + nameOf(arguments, n[1]));
				return f.apply(this, arguments);
			} : function () {
				bump(calls, short);
				return f.apply(this, arguments);
			};
		});
	}

	// Frames the page did not draw in time, as the browser measures them: a
	// long animation frame where it has them (Chromium 123), else a long task.
	function watchStalls() {
		if (typeof PerformanceObserver !== 'function') return;
		var types = PerformanceObserver.supportedEntryTypes || [];
		var type = types.indexOf('long-animation-frame') >= 0 ? 'long-animation-frame'
			: types.indexOf('longtask') >= 0 ? 'longtask' : '';
		if (!type) return;
		stalls.kind = type;
		try {
			new PerformanceObserver(function (list) {
				list.getEntries().forEach(function (e) {
					stalls.n++;
					stalls.ms += e.duration;
					if (e.duration > stalls.worst) stalls.worst = e.duration;
				});
			}).observe({type: type, buffered: true});
		} catch (e) { stalls.kind = ''; }
	}

	// The DOM calls that make objects in Blink's own heap, counted with
	// ?diag. Everything here is garbage the moment it is replaced, which is
	// what Blink's collector is left to clean up.
	function watchBlink() {
		function method(proto, m, key) {
			var f = proto && proto[m];
			if (typeof f !== 'function') return;
			proto[m] = function () { bump(blink, key || m); return f.apply(this, arguments); };
		}
		function setter(proto, p, key) {
			var d = Object.getOwnPropertyDescriptor(proto, p);
			if (!d || !d.set) return;
			Object.defineProperty(proto, p, {configurable: true, enumerable: d.enumerable, get: d.get,
				set: function (v) { bump(blink, key); d.set.call(this, v); }});
		}
		setter(Node.prototype, 'textContent', 'set textContent');
		setter(Element.prototype, 'innerHTML', 'set innerHTML');
		setter(Element.prototype, 'className', 'set className');
		method(EventTarget.prototype, 'dispatchEvent');
		method(Element.prototype, 'getBoundingClientRect');
		method(Element.prototype, 'setAttribute');
		method(window, 'getComputedStyle');
		method(CSSStyleDeclaration.prototype, 'setProperty', 'style.setProperty');
		['measureText', 'getImageData', 'createImageData', 'createLinearGradient', 'createRadialGradient',
			'drawImage', 'fillText'].forEach(function (m) {
			method(CanvasRenderingContext2D.prototype, m, '2d ' + m);
		});
	}

	// handles is what Go holds: the live entries in wasm_exec's table of
	// values, by kind. Each keeps its JS object, and for a DOM object its
	// Blink object, alive until Go finalizes the handle.
	function handles() {
		var vals = goInst && goInst._values;
		if (!vals) return null;
		var by = {}, live = 0;
		for (var i = 0; i < vals.length; i++) {
			var v = vals[i];
			if (v === null || v === undefined) continue;
			live++;
			var t = typeof v;
			var k = (t === 'object' || t === 'function') && v.constructor && v.constructor.name ? v.constructor.name : t;
			by[k] = (by[k] || 0) + 1;
		}
		var kinds = Object.keys(by).map(function (k) { return {name: k, n: by[k]}; });
		kinds.sort(function (a, b) { return b.n - a.n; });
		return {live: live, table: vals.length, kinds: kinds.slice(0, 12)};
	}

	function counts(m) {
		return Object.keys(m).map(function (k) { return {name: k, n: m[k]}; })
			.sort(function (a, b) { return b.n - a.n; });
	}

	watchStalls();
	if (detail) watchBlink();

	return {
		wrap: wrap,
		// snapshot is everything counted so far. The counts only grow:
		// `chaosrack mem --watch` turns them into rates.
		snapshot: function () {
			return {detail: detail, calls: counts(calls), names: counts(names).slice(0, 30),
				blink: counts(blink), handles: handles(), stalls: stalls};
		}
	};
})();
