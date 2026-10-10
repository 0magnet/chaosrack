//go:build js && wasm

package attractor

import (
	"encoding/json"
	"math"
	"strings"
	"syscall/js"

	"github.com/0magnet/websh/progressive"
	"github.com/0magnet/websh/web"
	xterm "github.com/0magnet/xterm-go"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The progressive panel's page half (racktui/overlay.go): `rack`, typed in
// the page's own terminal, lays the page's own parts over its cells as that
// terminal's placements (OSC 7337 place) — the model's canvas behind
// everything, and the real panel in a window floating over it, at the size
// the page draws them. The cells say only where: how many a part covers
// depends on how big a cell is, and the parts stay the page's size however
// the terminal is zoomed. So the panel in a terminal is the page's interface,
// at an ordinary cell size.
//
// The parts are the parts, not copies: the canvas and the frame are moved
// into the placements, and back when they go. The frame goes inside a
// wrapper standing in for #controls-panel (.cp-ctx, the class the stylesheet
// reads as the panel, and the panel's own classes), under a title bar like
// the page's window's, which moves the window by moving its cells. The
// canvas's placement passes the mouse on to the page (Page), so the model
// turns by the page's own hand, at the page's own coordinates; the panel's
// keeps it.
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
		// The frame fits itself to its parent (fitFrameToWidth), which changes
		// whenever the overlay moves it: into a terminal's window and home.
		js.Global().Set("__rackRefit", js.FuncOf(func(js.Value, []js.Value) any {
			if f := dom.Doc.Call("querySelector", ".rack-frame"); f.Truthy() {
				fitFrameToWidth(f)
			}
			return nil
		}))
		js.Global().Set("__rackWindowAct", js.FuncOf(func(_ js.Value, a []js.Value) any {
			if windowHand != nil && len(a) == 3 {
				windowHand(racktui.WindowAct{Kind: a[0].String(), X: a[1].Int(), Y: a[2].Int()})
			}
			return nil
		}))
	}
	return f
}

// windowHand is where the window's title bar sends what is done to it: the
// panel running now, or nil.
var windowHand func(racktui.WindowAct)

// termRack is the rack as the panel in one terminal sees it: the page's own
// rack, laid over that terminal's cells at the page's size.
type termRack struct {
	inPageRack
	term *xterm.Terminal
}

var _ racktui.Native = termRack{}

// WindowCells is the page's own window over the controls — its size as the
// person last left it floating — in this terminal's cells.
func (r termRack) WindowCells() (w, h int) {
	if r.term == nil {
		return 0, 0
	}
	cw, ch := r.term.CellSize()
	if cw <= 0 || ch <= 0 {
		return 0, 0
	}
	return int(math.Round(layout.floatW / cw)), int(math.Round(layout.floatH / ch))
}

// OnWindow sends what is done to the window's title bar to f.
func (termRack) OnWindow(f func(racktui.WindowAct)) {
	rackOverlay()
	windowHand = f
}

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
	place := func(id string, r racktui.Rect, widget string, page bool) {
		if r.W <= 0 || r.H <= 0 {
			s.WriteString(progressive.Remove(id))
			return
		}
		s.WriteString(progressive.Place(id, progressive.Placement{Row: r.Y, Col: r.X, W: r.W, H: r.H, Widget: widget, Input: true, Page: page}))
	}
	place(overlaySceneID, l.SceneShown, overlaySceneWidget, true)
	place(overlayPanelID, l.Window, overlayPanelWidget, false)
	return s.String()
}

const rackOverlayJS = `
var doc = document, L = null, P = null, S = null, away = false;
var panel = doc.getElementById('controls-panel');
function ctxClass() { return 'cp-ctx ' + (panel ? panel.className : ''); }
if (panel) new MutationObserver(function () {
  if (P) P.wrap.className = ctxClass();
}).observe(panel, { attributes: true, attributeFilter: ['class'] });

function act(kind, x, y) { if (window.__rackWindowAct) window.__rackWindowAct(kind, x | 0, y | 0); }

// refit has the frame fit its new parent, as it fits the page's window.
function refit() { if (window.__rackRefit) window.__rackRefit(); }

// cell is one cell of the placement el in pixels, from the layout's cells.
function cell(el, r) {
  return [r && r.W ? el.clientWidth / r.W : 0, r && r.H ? el.clientHeight / r.H : 0];
}

// fit makes the title bar one cell tall, as the terminal's title row is.
function fit() {
  if (!P || !L || !L.Window || !L.Window.H) return;
  var h = cell(P.el, L.Window)[1];
  if (h > 0) P.bar.style.height = h + 'px';
}

// home puts an element back where it was taken from.
function home(it) {
  if (it.node.parentNode === it.parent) return;
  var next = it.next && it.next.parentNode === it.parent ? it.next : null;
  it.parent.insertBefore(it.node, next);
  refit();
}
function take(it) {
  return { node: it, parent: it.parentNode, next: it.nextSibling };
}

return {
  // view is the layout the terminal's panel drew last, as JSON, or null.
  view: function (j) { L = JSON.parse(j); fit(); },

  // hold gives the parts back to the page while the screen holding the
  // terminal is not in front, and takes them again when it is.
  hold: function (hidden) {
    away = hidden;
    if (hidden) {
      if (P) home(P.frame);
      if (S) home(S.canvas);
      return;
    }
    if (P && P.frame.node.parentNode !== P.wrap) { P.wrap.appendChild(P.frame.node); refit(); }
    if (S && S.canvas.node.parentNode !== S.el) S.el.appendChild(S.canvas.node);
  },

  // panel moves the rack's frame into el, under a title bar, and returns
  // what moves it back.
  panel: function (el) {
    var frame = doc.querySelector('.rack-frame');
    if (!frame || P) return function () {};
    el.style.zIndex = '1';
    el.style.display = 'flex';
    el.style.flexDirection = 'column';
    el.style.background = panel ? getComputedStyle(panel).backgroundColor : '#000';
    el.style.boxShadow = '0 4px 18px rgba(0,0,0,.55)';

    var bar = doc.createElement('div');
    bar.style.cssText = 'flex:none;display:flex;align-items:center;gap:6px;padding:0 6px 0 10px;' +
      'background:#0066ff;color:#fff;font:13px/1 system-ui,sans-serif;cursor:move;user-select:none;white-space:nowrap;overflow:hidden';
    var title = doc.createElement('span');
    title.textContent = 'chaosrack controls';
    title.style.flex = '1';
    bar.appendChild(title);
    [['–', 'hide', 'hide (the page\'s ▤ button brings it back)'], ['□', 'full', 'the whole terminal, or back']].forEach(function (b) {
      var k = doc.createElement('span');
      k.textContent = b[0]; k.title = b[2];
      k.style.cssText = 'cursor:pointer;padding:0 5px;font-size:15px';
      k.addEventListener('mousedown', function (e) { e.stopPropagation(); });
      k.addEventListener('click', function () { act(b[1], 0, 0); });
      bar.appendChild(k);
    });
    var body = doc.createElement('div');
    body.style.cssText = 'flex:1;min-height:0;overflow:auto;position:relative';
    var wrap = doc.createElement('div');
    wrap.className = ctxClass();
    // As wide as the window, as #controls-panel is in the page's window: the
    // frame fits itself to its parent's width (fitFrameToWidth), so it is
    // scaled here as it is there, and fills the terminal when the window does.
    wrap.style.cssText = 'margin:0;padding:0;background:transparent;width:100%;box-sizing:border-box';
    body.appendChild(wrap);
    el.appendChild(bar);
    el.appendChild(body);

    // The title bar moves the window: the placement follows the pointer,
    // and where it is let go the terminal puts the window, on whole cells.
    bar.addEventListener('mousedown', function (e) {
      if (e.button !== 0) return;
      e.preventDefault();
      var x0 = e.clientX, y0 = e.clientY, l0 = el.offsetLeft, t0 = el.offsetTop;
      function move(m) {
        el.style.left = (l0 + m.clientX - x0) + 'px';
        el.style.top = (t0 + m.clientY - y0) + 'px';
      }
      function up(m) {
        doc.removeEventListener('mousemove', move);
        doc.removeEventListener('mouseup', up);
        var c = cell(el, L && L.Window);
        if (c[0] > 0 && c[1] > 0) act('move', Math.round(el.offsetLeft / c[0]), Math.round(el.offsetTop / c[1]));
      }
      doc.addEventListener('mousemove', move);
      doc.addEventListener('mouseup', up);
    });
    bar.addEventListener('dblclick', function () { act('full', 0, 0); });

    var p = P = { el: el, bar: bar, wrap: wrap, frame: take(frame) };
    wrap.appendChild(frame);
    if (away) home(p.frame); else refit();
    fit();
    var ro = new ResizeObserver(fit);
    ro.observe(el);
    return function () {
      ro.disconnect();
      home(p.frame);
      if (P === p) P = null;
    };
  },

  // scene moves the model's canvas into el, behind the window: the model
  // the page draws, where the page draws it, turned by the page's own hand.
  scene: function (el) {
    var c = doc.getElementById('gocanvas');
    if (!c || S) return function () {};
    el.style.zIndex = '0';
    // The canvas is clear where nothing is drawn; the cells under it are not
    // the page's ground.
    el.style.background = '#000';
    var pos = c.style.position, left = c.style.left, top = c.style.top;
    c.style.position = 'absolute'; c.style.left = '0'; c.style.top = '0';
    var s = S = { el: el, canvas: take(c) };
    if (!away) el.appendChild(c);
    return function () {
      home(s.canvas);
      c.style.position = pos; c.style.left = left; c.style.top = top;
      if (S === s) S = null;
    };
  }
};
`
