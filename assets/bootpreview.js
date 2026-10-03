/* The boot preview: something to watch while the rack loads.

   The page takes seconds to come up, nearly all of it the Go program building
   the panel, and until now those seconds were a black screen. This draws the
   rack's own kind of picture in the meantime -- a strange attractor, a map or
   a figure, one at a time in a random order, each held for a few seconds --
   in plain JavaScript, so it is running before the WebAssembly has even
   arrived. Only models that need no audio: there is no sound yet.

   Two places, as the pointer decides. Over the page, the pointer becomes the
   preview: a CSS cursor image, which the browser moves with the mouse even
   while the main thread is busy. Anywhere else, the preview is drawn in the
   middle of the page instead. That one is drawn by a worker on an
   OffscreenCanvas, because the Go program holds the main thread for long
   stretches while it builds, and a picture drawn from the main thread would
   stand still exactly when it is wanted. The cursor's picture can only change
   when the main thread is free, so between those moments it holds still --
   still a picture of the model, and still following the hand.

   Both draw the same model at the same moment without talking: the order is
   shuffled from one seed and the model is chosen by the clock.

   The Go program calls __crBootDone when the rack is up; the preview fades
   out after the first frames have been drawn, and is gone. */
(function () {
  'use strict';
  if (window.__crBootDone) return;

  // models is shared with the worker by its source text, so it may use
  // nothing from outside itself.
  function models() {
    function flow(name, f, p0, dt, steps) {
      return { name: name, kind: 'flow', f: f, p0: p0, dt: dt, steps: steps || 4 };
    }
    function map2(name, g, p0) {
      return { name: name, kind: 'map', g: g, p0: p0 };
    }
    return [
      flow('Lorenz', function (x, y, z) { return [10 * (y - x), x * (28 - z) - y, x * y - 8 / 3 * z]; }, [0.1, 0, 0], 0.005),
      flow('Rössler', function (x, y, z) { return [-y - z, x + 0.2 * y, 0.2 + z * (x - 5.7)]; }, [0.1, 0, 0], 0.012),
      flow('Aizawa', function (x, y, z) {
        return [(z - 0.7) * x - 3.5 * y, 3.5 * x + (z - 0.7) * y,
          0.6 + 0.95 * z - z * z * z / 3 - (x * x + y * y) * (1 + 0.25 * z) + 0.1 * z * x * x * x];
      }, [0.1, 0, 0], 0.01),
      flow('Thomas', function (x, y, z) {
        var b = 0.208186;
        return [Math.sin(y) - b * x, Math.sin(z) - b * y, Math.sin(x) - b * z];
      }, [0.1, 0, 0], 0.06),
      flow('Halvorsen', function (x, y, z) {
        var a = 1.89;
        return [-a * x - 4 * y - 4 * z - y * y, -a * y - 4 * z - 4 * x - z * z, -a * z - 4 * x - 4 * y - x * x];
      }, [-1.48, -1.51, 2.04], 0.005),
      flow('Chen', function (x, y, z) { return [35 * (y - x), (28 - 35) * x - x * z + 28 * y, x * y - 3 * z]; }, [-10, 0, 37], 0.002, 12),
      flow('Dadras', function (x, y, z) { return [y - 3 * x + 2.7 * y * z, 1.7 * y - x * z + z, 2 * x * y - 9 * z]; }, [1, 1, 0], 0.006),
      flow('Sprott B', function (x, y, z) { return [y * z, x - y, 1 - x * y]; }, [0.1, 0, 0], 0.02),
      flow('Lissajous', null, null, 0),
      map2('Clifford', function (x, y) { return [Math.sin(-1.4 * y) + Math.cos(-1.4 * x), Math.sin(1.6 * x) + 0.7 * Math.cos(1.6 * y)]; }, [0.1, 0.1]),
      map2('de Jong', function (x, y) { return [Math.sin(1.4 * y) - Math.cos(-2.3 * x), Math.sin(2.4 * x) - Math.cos(-2.1 * y)]; }, [0.1, 0.1])
    ];
  }

  // preview draws the models on a canvas. It is shared with the worker by its
  // source text too. seed and t0 make every copy pick the same model at the
  // same time; trail is how many points a flow's trail keeps.
  function preview(ctx, seed, t0, trail, caption) {
    var ms = models(), HOLD = 3600, FADE = 450;
    // The order: ms shuffled by a small seeded generator.
    var order = ms.map(function (_, i) { return i; }), s = seed >>> 0;
    function rnd() { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296; }
    for (var i = order.length - 1; i > 0; i--) { var j = Math.floor(rnd() * (i + 1)), t = order[i]; order[i] = order[j]; order[j] = t; }

    var cur = -1, m = null, pts = [], state = null, box = null, born = 0;

    // fit runs a model on past its transient and measures it, so each is
    // drawn to fill the canvas whatever its own scale.
    function start(k, now) {
      cur = k; m = ms[order[k % order.length]]; pts = []; born = now;
      if (m.kind === 'map') {
        state = m.p0.slice();
        for (var i = 0; i < 100; i++) state = m.g(state[0], state[1]);
        box = measure(function () { state = m.g(state[0], state[1]); return [state[0], state[1], 0]; }, 4000);
      } else if (m.f) {
        state = m.p0.slice();
        for (var n = 0; n < 3000; n++) step();
        box = measure(function () { step(); return state; }, 6000);
        // The trail whole from the start, not grown a few points a frame.
        for (var q = 0; q < trail; q++) { step(); pts.push(state[0], state[1], state[2]); }
      } else {
        state = [0]; box = { c: [0, 0, 0], r: 1.05 };
      }
    }
    function step() {
      // Midpoint (RK2): stable enough at these steps, and cheap.
      var x = state[0], y = state[1], z = state[2], h = m.dt;
      var k = m.f(x, y, z);
      var k2 = m.f(x + k[0] * h / 2, y + k[1] * h / 2, z + k[2] * h / 2);
      state = [x + k2[0] * h, y + k2[1] * h, z + k2[2] * h];
      if (!isFinite(state[0]) || Math.abs(state[0]) > 1e4) state = m.p0.slice();
    }
    function measure(next, n) {
      var lo = [1e9, 1e9, 1e9], hi = [-1e9, -1e9, -1e9];
      for (var i = 0; i < n; i++) {
        var p = next();
        for (var a = 0; a < 3; a++) { if (p[a] < lo[a]) lo[a] = p[a]; if (p[a] > hi[a]) hi[a] = p[a]; }
      }
      var c = [(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2];
      var r = Math.max(hi[0] - lo[0], hi[1] - lo[1], hi[2] - lo[2]) / 2 || 1;
      return { c: c, r: r * 1.15 };
    }

    // done names the model under it, where there is room for a caption:
    // drawn with it, so the name is the picture's whichever thread draws.
    function done(w, h) {
      if (caption) {
        ctx.globalAlpha = 1;
        ctx.globalCompositeOperation = 'source-over';
        ctx.font = Math.round(h / 40) + 'px monospace';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#567';
        // Spaced out with hair spaces, as the panel's own legends are.
        ctx.fillText(('LOADING · ' + m.name).toUpperCase().split('').join(' '), w / 2, h - h / 60);
      }
      return m.name;
    }

    // frame draws the picture at time now (ms) on a w by h canvas.
    return function frame(now, w, h) {
      var k = Math.floor((now - t0) / HOLD);
      if (k !== cur) start(k, now);
      var age = now - born, life = HOLD - (now - t0 - k * HOLD);
      var alpha = Math.min(1, age / FADE, life / FADE);
      var size = Math.min(w, h), sc = size / 2 / box.r, cx = w / 2, cy = h / 2;
      ctx.clearRect(0, 0, w, h);
      ctx.globalAlpha = Math.max(0, alpha);
      ctx.globalCompositeOperation = 'lighter';
      ctx.lineWidth = Math.max(1.2, size / 170);
      ctx.lineJoin = 'round';

      if (m.kind === 'map') {
        // A map has no path between iterates: points, gathering.
        var per = trail > 600 ? 600 : 120, cap = trail > 600 ? 20000 : 2500;
        for (var i = 0; i < per && pts.length < cap; i++) { state = m.g(state[0], state[1]); pts.push(state[0], state[1]); }
        var d = Math.max(1, size / 300);
        for (var p = 0; p < pts.length; p += 2) {
          var u = p / pts.length;
          ctx.fillStyle = 'hsla(' + (200 + 140 * u) + ',100%,70%,' + (trail > 600 ? 0.7 : 0.95) + ')';
          ctx.fillRect(cx + (pts[p] - box.c[0]) * sc, cy - (pts[p + 1] - box.c[1]) * sc, d, d);
        }
        return done(w, h);
      }

      // A flow, or the figure: a trail, turning slowly about the vertical.
      if (m.f) {
        for (var n = 0; n < m.steps; n++) { step(); pts.push(state[0], state[1], state[2]); }
        if (pts.length > trail * 3) pts.splice(0, pts.length - trail * 3);
      } else {
        // Lissajous: a 3:2:5 figure whose phase drifts, drawn whole.
        pts = [];
        var ph = (now - born) / 2000;
        for (var q = 0; q <= trail; q++) {
          var t = q / trail * Math.PI * 2;
          pts.push(Math.sin(3 * t + ph), Math.sin(2 * t), Math.sin(5 * t + ph / 2));
        }
      }
      var th = (now - t0) / 5000, co = Math.cos(th), si = Math.sin(th);
      var tilt = 0.35, ct = Math.cos(tilt), st = Math.sin(tilt);
      var nPts = pts.length / 3, seg = Math.max(1, Math.ceil(nPts / 48));
      for (var a = 0; a + 1 < nPts; a += seg) {
        ctx.beginPath();
        for (var b = a; b <= Math.min(a + seg, nPts - 1); b++) {
          var X = pts[b * 3] - box.c[0], Y = pts[b * 3 + 1] - box.c[1], Z = pts[b * 3 + 2] - box.c[2];
          // Turn about z (the flows' long axis stands up), then tilt.
          var x1 = X * co - Y * si, y1 = X * si + Y * co;
          var sx = cx + x1 * sc, sy = cy - (Z * ct - y1 * st) * sc;
          if (b === a) ctx.moveTo(sx, sy); else ctx.lineTo(sx, sy);
        }
        var v = a / nPts;
        ctx.strokeStyle = 'hsla(' + (340 - 150 * v) + ',100%,' + (58 + 17 * v) + '%,' + (0.6 + 0.4 * v) + ')';
        ctx.stroke();
      }
      return done(w, h);
    };
  }

  var seed = (Math.random() * 4294967296) >>> 0, t0 = performance.now();
  var root = document.documentElement;

  // The layer: the whole window, over everything, so the pointer is the
  // preview wherever it is on the page. The page under it is not usable yet.
  var layer = document.createElement('div');
  layer.id = 'cr-boot';
  layer.setAttribute('aria-label', 'Loading');
  layer.style.cssText = 'position:fixed;inset:0;z-index:2147483646;cursor:progress;transition:opacity .35s;';
  var stage = document.createElement('div');
  stage.style.cssText = 'position:absolute;left:50%;top:50%;width:min(60vmin,520px);height:min(60vmin,520px);transform:translate(-50%,-50%);transition:opacity .3s;pointer-events:none;';
  var big = document.createElement('canvas');
  big.style.cssText = 'width:100%;height:100%;display:block;';
  stage.appendChild(big);
  layer.appendChild(stage);
  (document.body || root).appendChild(layer);

  var dpr = Math.min(2, window.devicePixelRatio || 1);
  var px = Math.round(Math.min(Math.min(innerWidth, innerHeight) * 0.6, 520) * dpr);
  big.width = big.height = px;

  // The page's preview: in a worker where there is one, so it keeps moving
  // while the main thread builds the rack; here otherwise.
  var worker = null, bigFrame = null;
  if (big.transferControlToOffscreen && window.Worker && window.Blob) {
    try {
      var src = models.toString() + '\n' + preview.toString() + '\n' +
        'var draw = null, on = true;\n' +
        'onmessage = function (e) {\n' +
        '  var d = e.data;\n' +
        '  if (d.canvas) {\n' +
        '    var c = d.canvas, x = c.getContext("2d");\n' +
        '    draw = preview(x, d.seed, d.t0 - performance.timeOrigin + d.origin, 3000, true);\n' +
        '    var tick = function () { if (on) draw(performance.now(), c.width, c.height); requestAnimationFrame(tick); };\n' +
        '    requestAnimationFrame(tick);\n' +
        '  }\n' +
        '  if (d.on !== undefined) on = d.on;\n' +
        '};\n';
      var off = big.transferControlToOffscreen();
      worker = new Worker(URL.createObjectURL(new Blob([src], { type: 'text/javascript' })));
      // The worker's clock starts at its own timeOrigin: t0 and the page's
      // origin go with the canvas so both clocks name the same moment.
      worker.postMessage({ canvas: off, seed: seed, t0: t0, origin: performance.timeOrigin }, [off]);
    } catch (e) {
      worker = null;
    }
  }
  if (!worker) bigFrame = preview(big.getContext('2d'), seed, t0, 3000, true);

  // The cursor: the same picture, small, made into a cursor image a few times
  // a second whenever the main thread has a moment.
  var CUR = 64, small = document.createElement('canvas');
  small.width = small.height = CUR;
  var smallFrame = preview(small.getContext('2d'), seed, t0, 420), lastCur = 0;

  // Over the page the pointer is the preview, and the middle of the page is
  // left clear; off it, the middle shows it.
  var over = false;
  function setOver(v) {
    if (v === over) return;
    over = v;
    stage.style.opacity = v ? '0' : '1';
    if (worker) worker.postMessage({ on: !v });
  }
  layer.addEventListener('pointermove', function () { setOver(true); });
  layer.addEventListener('pointerenter', function () { setOver(true); });
  document.addEventListener('pointerleave', function () { setOver(false); });
  document.addEventListener('mouseout', function (e) { if (!e.relatedTarget) setOver(false); });

  var running = true;
  function tick(now) {
    if (!running) return;
    if (bigFrame && !over) bigFrame(now, big.width, big.height);
    // Only while it is the cursor, and encoded off this thread (toBlob): a
    // PNG a few times a second from here was a sixth of a second of the
    // main thread over a boot, taken from the rack it is waiting for.
    if (over && !encoding && now - lastCur > 90 && small.toBlob) {
      lastCur = now;
      smallFrame(now, CUR, CUR);
      encoding = true;
      small.toBlob(function (b) {
        encoding = false;
        if (!b || !running) return;
        var url = URL.createObjectURL(b);
        layer.style.cursor = 'url(' + url + ') ' + CUR / 2 + ' ' + CUR / 2 + ', progress';
        if (curURL) URL.revokeObjectURL(curURL);
        curURL = url;
      });
    }
    requestAnimationFrame(tick);
  }
  var encoding = false, curURL = null;
  requestAnimationFrame(tick);

  // __crBootDone is the rack saying it is up: the preview fades once the
  // first frames are on the screen, and is gone.
  window.__crBootDone = function () {
    window.__crBootDone = function () {};
    requestAnimationFrame(function () {
      requestAnimationFrame(function () {
        layer.style.opacity = '0';
        layer.style.pointerEvents = 'none';
        setTimeout(function () {
          running = false;
          if (worker) worker.terminate();
          layer.remove();
        }, 400);
      });
    });
  };
})();
