//go:build js && wasm

package attractor

import (
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/0magnet/websh/progressive"
	"github.com/0magnet/websh/web"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The progressive panel's page half (racktui/overlay.go): `rack`, typed in
// the page's own terminal, lays the page's own parts over its cells as that
// terminal's placements (OSC 7337 place) — the model's canvas over the scene,
// and the real panel over the window, scaled so a cell is as wide as the
// page look's 2.4 panel pixels, lying on its own picture of itself.
// Zoom the terminal out and the cells shrink, and the panel with them, to the
// page's own size at websh's smallest cell.
//
// The panel is the panel, not a copy: the frame is moved into the placement,
// inside a wrapper standing in for #controls-panel (the class the stylesheet
// reads as the panel, .cp-ctx, and the panel's own classes, as the manual's
// live modules are), and back when the placement goes. Its knobs are worked
// by hand there; the scene keeps the terminal's mouse, which turns the model
// through the page look's own scene mouse.
//
// In JavaScript, as the manual's live modules are: moving and measuring DOM
// is what Go/wasm does worst.

const (
	overlaySceneWidget = "chaosrack-scene"
	overlayPanelWidget = "chaosrack-panel"
	overlaySceneID     = "rack-scene"
	overlayPanelID     = "rack-panel"
)

func init() {
	web.RegisterWidget(overlaySceneWidget, func(el js.Value) func() {
		u := rackOverlay().Call("scene", el)
		return func() { u.Invoke() }
	})
	web.RegisterWidget(overlayPanelWidget, func(el js.Value) func() {
		u := rackOverlay().Call("panel", el)
		return func() { u.Invoke() }
	})
}

// rackOverlay is the page's half of the overlay, made the first time.
func rackOverlay() js.Value {
	f := js.Global().Get("__rackOverlay")
	if !f.Truthy() {
		f = js.Global().Get("Function").New(rackOverlayJS).Invoke()
		js.Global().Set("__rackOverlay", f)
	}
	return f
}

// The panel turns the overlay on and off through it (racktui/overlay.go).
var _ racktui.Overlay = inPageRack{}

// Overlay lays the scene and the panel over l, or takes them away.
func (inPageRack) Overlay(l *racktui.Layout) string {
	if l == nil {
		rackOverlay().Call("view", "null")
		return progressive.Remove(overlayPanelID) + progressive.Remove(overlaySceneID)
	}
	b, err := json.Marshal(l)
	if err == nil {
		rackOverlay().Call("view", string(b))
	}
	var s strings.Builder
	place := func(id string, r racktui.Rect, widget string, input bool) {
		if r.W <= 0 || r.H <= 0 {
			s.WriteString(progressive.Remove(id))
			return
		}
		s.WriteString(progressive.Place(id, progressive.Placement{Row: r.Y, Col: r.X, W: r.W, H: r.H, Widget: widget, Input: input}))
	}
	place(overlaySceneID, l.SceneShown, overlaySceneWidget, false)
	place(overlayPanelID, l.Window, overlayPanelWidget, true)
	return s.String()
}

const rackOverlayJS = `
var doc = document, L = null, P = null;
var panel = doc.getElementById('controls-panel');
function ctxClass() { return 'cp-ctx ' + (panel ? panel.className : ''); }
function kscale() {
  return parseFloat(getComputedStyle(doc.documentElement).getPropertyValue('--kscale')) || 1;
}
if (panel) new MutationObserver(function () {
  if (P) P.wrap.className = ctxClass();
}).observe(panel, { attributes: true, attributeFilter: ['class'] });

// place scales and moves the panel so its pixel at the view's corner is at
// the window's, and a cell is PxPerCol x PxPerRow of its pixels: the page
// look's own picture of it, underneath, exactly.
function place() {
  if (!P || !L || !L.Window.W || !L.Window.H) return;
  // One scale both ways, from the cells' width: a cell is not always the
  // 2.4:5 the page look assumes (websh's shape changes a little with its
  // zoom), and a knob drawn round matters more than its last row lining up
  // with cells it covers anyway.
  var k = kscale();
  var s = P.el.clientWidth / L.Window.W / (L.PxPerCol * k);
  var tx = -L.ViewX * L.PxPerCol * k - P.frame.offsetLeft;
  var ty = -L.ViewY * L.PxPerRow * k - P.frame.offsetTop;
  P.wrap.style.transform = 'scale(' + s + ') translate(' + tx + 'px,' + ty + 'px)';
}

return {
  // view is the layout the terminal's panel drew last, as JSON, or null.
  view: function (j) { L = JSON.parse(j); place(); },

  // panel moves the rack's frame into el, and returns what moves it back.
  panel: function (el) {
    var frame = doc.querySelector('.rack-frame');
    if (!frame || P) return function () {};
    var wrap = doc.createElement('div');
    wrap.className = ctxClass();
    // Laid out at its own width, as it is in the panel, so moving it does
    // not reflow it.
    wrap.style.cssText = 'position:absolute;left:0;top:0;margin:0;padding:0;overflow:visible;' +
      'transform-origin:0 0;background:transparent;width:' + frame.offsetWidth + 'px';
    el.style.zIndex = '1';
    el.style.background = panel ? getComputedStyle(panel).backgroundColor : '#000';
    var p = P = { el: el, wrap: wrap, frame: frame, parent: frame.parentNode, next: frame.nextSibling };
    wrap.appendChild(frame);
    el.appendChild(wrap);
    var ro = new ResizeObserver(place);
    ro.observe(el);
    place();
    return function () {
      ro.disconnect();
      var next = p.next && p.next.parentNode === p.parent ? p.next : null;
      p.parent.insertBefore(p.frame, next);
      wrap.remove();
      if (P === p) P = null;
    };
  },

  // scene draws the model's canvas into el every frame, cropped as the page
  // look crops it — the middle of it, cut to the shape of the cells it is
  // sampled across — and of that, the part el covers: the cells beside the
  // window, whose own are left to the terminal.
  scene: function (el) {
    var src = doc.getElementById('gocanvas'), c = doc.createElement('canvas');
    c.style.cssText = 'position:absolute;inset:0;width:100%;height:100%';
    el.style.zIndex = '0';
    el.appendChild(c);
    var g = c.getContext('2d'), raf = 0;
    function frame() {
      raf = requestAnimationFrame(frame);
      if (!src || !src.width || !src.height) return;
      var dpr = window.devicePixelRatio || 1;
      var w = Math.max(1, Math.round(el.clientWidth * dpr)), h = Math.max(1, Math.round(el.clientHeight * dpr));
      if (c.width !== w) c.width = w;
      if (c.height !== h) c.height = h;
      // The whole of what is sampled, in el's pixels: as wide as its cells
      // over el's, as tall likewise.
      var all = L && L.Scene.W && L.SceneShown.W ? L.Scene : null;
      var fw = all ? w * all.W / L.SceneShown.W : w, fh = all ? h * all.H / L.SceneShown.H : h;
      var ox = all ? (L.SceneShown.X - all.X) / all.W : 0, oy = all ? (L.SceneShown.Y - all.Y) / all.H : 0;
      var want = fw / fh, have = src.width / src.height, sx = 0, sy = 0, sw = src.width, sh = src.height;
      if (have > want) { sw = src.height * want; sx = (src.width - sw) / 2; }
      else { sh = src.width / want; sy = (src.height - sh) / 2; }
      // el's part of that.
      sx += ox * sw; sy += oy * sh;
      sw *= w / fw; sh *= h / fh;
      g.fillStyle = '#000';
      g.fillRect(0, 0, w, h);
      g.drawImage(src, sx, sy, sw, sh, 0, 0, w, h);
    }
    frame();
    return function () { cancelAnimationFrame(raf); c.remove(); };
  }
};
`
