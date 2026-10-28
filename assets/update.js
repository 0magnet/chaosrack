// Keeps an open tab on the newest build.
//
// The deployed page fetches its wasm from assets/, which the CI bot rebuilds
// after a push, and nothing told an open tab that had happened: it ran the
// build it loaded until someone reloaded it, and a reload inside the host's
// ten-minute cache could still bring back the old binary. Beside each wasm is
// built-from.txt, the stamp of the sources it was built from, which changes
// in the same commit as the binary.
//
// So the page reads the stamp before it fetches the wasm and asks for the
// wasm by it (?v=stamp), a URL no cache holds an older binary under. It reads
// the stamp again every few minutes and whenever the tab comes back into
// view, and when the stamp has changed it reloads. A reload keeps the rack as
// it was (the controls are in the address's hash, the arrangement in
// localStorage) and loses what is running: the trajectory, and the sound,
// which the browser holds back in a new page until a click. So it reloads:
//   - hidden and silent: at once;
//   - in view and silent: once the page has gone a minute without input,
//     after ten seconds' notice that can be put off;
//   - with audio running: not by itself. The notice offers it, and it happens
//     when the audio stops (the rack suspends its context when nothing uses it).
// A single-file page carries its binary and fetches no stamp, and a page with
// no stamp beside its wasm (opened from a disk, or an older server) reads
// none: either way nothing further is done.
window.__crUpdate = (function () {
	'use strict';
	var EVERY = 5 * 60e3, IDLE = 60e3, NOTICE = 10, PUT_OFF = 30 * 60e3, BOOT_WAIT = 5e3;
	var stampURL = '', build = '', pending = false;
	var lastInput = Date.now(), putOffUntil = 0, timer = 0, note = null;

	// Every AudioContext the page makes, to tell whether it is making sound.
	// Wrapped here because this runs before the Go program makes its own.
	var contexts = [];
	['AudioContext', 'webkitAudioContext'].forEach(function (name) {
		var C = window[name];
		if (typeof C !== 'function' || typeof Proxy !== 'function') return;
		window[name] = new Proxy(C, {construct: function (t, args, nt) {
			var c = Reflect.construct(t, args, nt);
			contexts.push(typeof WeakRef === 'function' ? new WeakRef(c) : {deref: function () { return c; }});
			return c;
		}});
	});
	function audible() {
		return contexts.some(function (r) { var c = r.deref(); return c && c.state === 'running'; });
	}
	['pointerdown', 'pointermove', 'keydown', 'wheel'].forEach(function (t) {
		addEventListener(t, function () { lastInput = Date.now(); }, {capture: true, passive: true});
	});

	// read fetches the stamp past every cache: no-store for the browser's, a
	// query the host's CDN has not seen for its own.
	function read(ms) {
		var ac = typeof AbortController === 'function' ? new AbortController() : null;
		var t = ac && ms ? setTimeout(function () { ac.abort(); }, ms) : 0;
		return fetch(stampURL + '?t=' + Date.now(), {cache: 'no-store', signal: ac && ac.signal}).then(function (r) {
			if (!r.ok) throw new Error('built-from.txt: ' + r.status);
			return r.text();
		}).then(function (s) {
			clearTimeout(t);
			s = s.trim();
			if (!/^[0-9a-f]{8,64}$/.test(s)) throw new Error('built-from.txt holds no stamp');
			return s;
		});
	}

	function check() {
		if (pending) { decide(); return; }
		read(0).then(function (s) {
			if (s !== build) {
				console.log('[update] a new build: ' + build + ' → ' + s);
				pending = true;
				decide();
			}
		}, function () {});
	}

	function decide() {
		clearTimeout(timer);
		if (!pending) return;
		var now = Date.now();
		if (audible()) {
			show('reload (stops the sound)');
			timer = setTimeout(decide, 30e3);
			return;
		}
		if (document.hidden) { location.reload(); return; }
		if (now < putOffUntil) {
			hide();
			timer = setTimeout(decide, putOffUntil - now);
			return;
		}
		var wait = lastInput + IDLE - now;
		if (wait > 0) {
			show('reload');
			timer = setTimeout(decide, wait);
			return;
		}
		countdown(NOTICE);
	}

	function countdown(left) {
		if (Date.now() - lastInput < IDLE || audible()) { decide(); return; }
		if (left <= 0) { location.reload(); return; }
		show('reloading in ' + left + ' s');
		timer = setTimeout(function () { countdown(left - 1); }, 1000);
	}

	// show puts up the notice: "new build", what will happen or can be done,
	// and a way to put it off. Spans, not links: an href="#" would rewrite the
	// hash the rack keeps its controls in.
	function show(what) {
		if (!note) {
			note = document.createElement('div');
			note.style.cssText = 'position:fixed;bottom:10px;right:10px;z-index:30;font:12px monospace;' +
				'letter-spacing:1px;color:#8cf;background:rgba(0,0,0,.8);border:1px solid #2a6a9a;padding:4px 8px;user-select:none;';
			note.innerHTML = 'new build · <span data-a="reload" style="cursor:pointer;text-decoration:underline"></span>' +
				' · <span data-a="later" style="cursor:pointer;color:#778">not now</span>';
			note.addEventListener('click', function (e) {
				var a = e.target.getAttribute('data-a');
				if (a === 'reload') location.reload();
				if (a === 'later') { putOffUntil = Date.now() + PUT_OFF; decide(); }
			});
			document.body.appendChild(note);
		}
		note.querySelector('[data-a=reload]').textContent = what;
		note.style.display = '';
	}
	function hide() { if (note) note.style.display = 'none'; }

	return {
		// url is the address to fetch the wasm at wasmURL by: its stamp
		// added, or wasmURL as it is when there is no stamp to read. Reading
		// one also starts the watch for the next.
		url: function (wasmURL) {
			stampURL = wasmURL.replace(/[^/]*$/, '') + 'built-from.txt';
			return read(BOOT_WAIT).then(function (s) {
				build = s;
				setInterval(check, EVERY);
				document.addEventListener('visibilitychange', function () {
					if (document.hidden) decide(); else check();
				});
				return wasmURL + '?v=' + s;
			}, function () { return wasmURL; });
		},
		// stamp is the build the page is running, '' when it read none: the
		// meters worker asks for its own files by it.
		stamp: function () { return build; },
	};
})();
