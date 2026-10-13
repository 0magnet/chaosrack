//go:build js && wasm

package attractor

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
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
			if windowHand == nil || len(a) != 4 {
				return nil
			}
			act := racktui.WindowAct{Kind: a[0].String(), X: a[1].Int(), Y: a[2].Int(), Edge: a[3].String()}
			// Where the window is put is the page's own preference, as the
			// page's DOCK buttons save it: the next load opens it there.
			switch act.Kind {
			case "dock":
				lsSet("wasmstuff-dock", act.Edge)
			case "move":
				lsSet("wasmstuff-dock", "float")
			case "resize":
				// The size the window was dragged to, in pixels, kept where the
				// page keeps its own window's: the drawer's depth docked, the
				// floating window's size floating. WindowCells and DockCells
				// read them back when the panel lays the window out again.
				switch act.Edge {
				case "top", "bottom":
					layout.dockSizeH = float64(act.Y)
					lsSet("wasmstuff-dockH", strconv.Itoa(act.Y))
				case "left", "right":
					layout.dockSizeW = float64(act.X)
					lsSet("wasmstuff-dockW", strconv.Itoa(act.X))
				default:
					layout.floatW, layout.floatH = float64(act.X), float64(act.Y)
					layout.saveFloatGeom()
				}
			}
			windowHand(act)
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
	// taken is a panel started while another held the page's parts.
	taken bool
}

// PartsTaken reports whether another panel had the page's parts when this
// one started (racktui.Sharer).
func (r termRack) PartsTaken() bool { return r.taken }

// partsHeld counts the panels holding the page's parts: its one canvas and
// its one panel can be laid over one terminal at a time.
var (
	partsMu   sync.Mutex
	partsHeld int
)

// holdParts takes the page's parts for a panel, or reports that another has
// them. A panel drawing in cells (--plain) takes nothing.
func holdParts() (release func(), ok bool) {
	if racktui.Plain() {
		return func() {}, true
	}
	partsMu.Lock()
	defer partsMu.Unlock()
	if partsHeld > 0 {
		return func() {}, false
	}
	partsHeld++
	return func() {
		partsMu.Lock()
		partsHeld--
		partsMu.Unlock()
	}, true
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

// Dock is where the page docks its controls: the edge its DOCK buttons last
// chose, the bottom when none has been.
func (termRack) Dock() string {
	if s, ok := lsGet("wasmstuff-dock"); ok {
		return s
	}
	return "bottom"
}

// DockCells is the page's drawer against edge, as its resize bar last left
// it, in this terminal's cells: across the top or the bottom it is a row
// taller, for the title row.
func (r termRack) DockCells(edge string) int {
	if r.term == nil {
		return 0
	}
	cw, ch := r.term.CellSize()
	if cw <= 0 || ch <= 0 {
		return 0
	}
	if edge == "top" || edge == "bottom" {
		return int(math.Round(layout.dockSizeH/ch)) + 1
	}
	return int(math.Round(layout.dockSizeW / cw))
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
// held reports whether el is laid over the instrument's terminal, whose
// screen "away" is about.
function held(el) { return !!(el && el.closest && el.closest('#instrument')); }
var panel = doc.getElementById('controls-panel');
// ctxClass is the panel's classes, docked where the terminal's window is:
// the stylesheet lays the modules out by the dk- edge.
function ctxClass() {
  var dk = 'dk-' + ((L && L.Dock) || 'float');
  return 'cp-ctx ' + (panel ? panel.className : '').replace(/\bdk-\w+/g, '') + ' ' + dk;
}
if (panel) new MutationObserver(function () {
  if (P) P.wrap.className = ctxClass();
}).observe(panel, { attributes: true, attributeFilter: ['class'] });

function act(kind, x, y, edge) { if (window.__rackWindowAct) window.__rackWindowAct(kind, x | 0, y | 0, edge || ''); }

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

// docked shows where the window is docked: the panel's classes for that edge,
// and its DOCK button lit, as the page's are.
function docked() {
  if (!P) return;
  var cls = ctxClass();
  if (P.wrap.className !== cls) { P.wrap.className = cls; refit(); }
  var at = (L && L.Dock) || 'float';
  P.docks.forEach(function (d) { d.k.style.background = d.edge === at ? 'rgba(255,255,255,.28)' : ''; });
  // Floating, every edge and corner sizes the window; docked, only the edge
  // facing the model does, as a drawer's.
  var inner = { bottom: 'n', top: 's', left: 'e', right: 'w' }[at];
  P.grips.forEach(function (g) { g.el.style.display = !inner || g.dir === inner ? '' : 'none'; });
  P.bar.style.cursor = at === 'top' || at === 'bottom' ? 'ns-resize' : inner ? 'default' : 'move';
}

// gripAt is where each of the window's edges and corners is grabbed, by
// compass direction: each sizes the window from its side. They reach a
// little past the window, so an edge is easy to find.
var gripAt = {
  n: 'left:8px;right:8px;top:-3px;height:7px;cursor:ns-resize',
  s: 'left:8px;right:8px;bottom:-3px;height:7px;cursor:ns-resize',
  e: 'top:8px;bottom:8px;right:-3px;width:7px;cursor:ew-resize',
  w: 'top:8px;bottom:8px;left:-3px;width:7px;cursor:ew-resize',
  ne: 'top:-3px;right:-3px;width:11px;height:11px;cursor:nesw-resize',
  nw: 'top:-3px;left:-3px;width:11px;height:11px;cursor:nwse-resize',
  se: 'bottom:-3px;right:-3px;width:14px;height:14px;cursor:nwse-resize;' +
    'background:linear-gradient(135deg,transparent 55%,rgba(255,255,255,.5) 55%)',
  sw: 'bottom:-3px;left:-3px;width:11px;height:11px;cursor:nesw-resize'
};

// size drags the dir sides (some of n, s, e, w) of the window el from the
// press e, kept inside the terminal. Let go, the window goes back to its
// cells and the size is sent: the page keeps it as its own window's, as its
// resize bar does (the next load opens it so), and the terminal lays the
// window out again on whole cells, so a drag too small to change a cell
// snaps back. Sizing from the top or the left moves the window too.
function size(el, e, dir) {
  if (e.button !== 0) return;
  e.preventDefault(); e.stopPropagation();
  var at = (L && L.Dock) || 'float', c = cell(el, L && L.Window);
  var host = el.offsetParent || el.parentNode, W = host.clientWidth, H = host.clientHeight;
  var x0 = e.clientX, y0 = e.clientY;
  var l0 = el.offsetLeft, t0 = el.offsetTop, w0 = el.offsetWidth, h0 = el.offsetHeight;
  var st = el.style, was = [st.left, st.top, st.width, st.height];
  var n = dir.indexOf('n') >= 0, s = dir.indexOf('s') >= 0;
  var w = dir.indexOf('w') >= 0, ea = dir.indexOf('e') >= 0;
  function box(m) {
    var dx = m.clientX - x0, dy = m.clientY - y0, ww = w0, hh = h0;
    if (ea) ww = Math.min(w0 + dx, W - l0);
    if (w) ww = Math.min(w0 - dx, l0 + w0);
    if (s) hh = Math.min(h0 + dy, H - t0);
    if (n) hh = Math.min(h0 - dy, t0 + h0);
    ww = Math.max(160, ww); hh = Math.max(80, hh);
    return [w ? l0 + w0 - ww : l0, n ? t0 + h0 - hh : t0, ww, hh];
  }
  function move(m) {
    if (m.buttons === 0) return; // no button held: not this drag
    var b = box(m);
    st.left = b[0] + 'px'; st.top = b[1] + 'px'; st.width = b[2] + 'px'; st.height = b[3] + 'px';
  }
  function up(m) {
    doc.removeEventListener('mousemove', move);
    doc.removeEventListener('mouseup', up);
    var b = box(m);
    st.left = was[0]; st.top = was[1]; st.width = was[2]; st.height = was[3];
    if (b[2] === w0 && b[3] === h0) return;
    // Docked across the top or the bottom the page's drawer is the body
    // alone; the title row is the terminal's.
    var h = at === 'top' || at === 'bottom' ? b[3] - P.bar.offsetHeight : b[3];
    act('resize', b[2], h, at);
    if (at === 'float' && (n || w) && c[0] > 0 && c[1] > 0) {
      act('move', Math.round(b[0] / c[0]), Math.round(b[1] / c[1]));
    }
  }
  doc.addEventListener('mousemove', move);
  doc.addEventListener('mouseup', up);
}

return {
  // view is the layout the terminal's panel drew last, as JSON, or null.
  view: function (j) { L = JSON.parse(j); fit(); docked(); },

  // hold gives the parts back to the page while the instrument's screen is
  // not in front, and takes them again when it is. Only parts laid over the
  // instrument's terminal: a panel run in another of the page's terminals
  // keeps its parts whichever screen is in front — "away" was the
  // instrument's, and that panel's window came up empty while it was.
  hold: function (hidden) {
    away = hidden;
    var p = P && held(P.el), s = S && held(S.el);
    if (hidden) {
      if (p) home(P.frame);
      if (s) home(S.canvas);
      return;
    }
    if (p && P.frame.node.parentNode !== P.wrap) { P.wrap.appendChild(P.frame.node); refit(); }
    if (s && S.canvas.node.parentNode !== S.el) S.el.appendChild(S.canvas.node);
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
    // The page's DOCK buttons, which are the shell's and stay with it: here
    // they dock the terminal's window, against the terminal's edges.
    var docks = [];
    var lbl = doc.createElement('span');
    lbl.textContent = 'DOCK';
    lbl.style.cssText = 'font-size:10px;letter-spacing:1px;opacity:.75;margin-right:2px';
    bar.appendChild(lbl);
    [['↑', 'top'], ['↓', 'bottom'], ['←', 'left'], ['→', 'right'], ['⧉', 'float']].forEach(function (b) {
      var k = doc.createElement('span');
      k.textContent = b[0];
      k.title = b[1] === 'float' ? 'float: a window over the model' : 'dock to the ' + b[1];
      k.style.cssText = 'cursor:pointer;padding:0 4px;border-radius:3px;font-size:13px';
      k.addEventListener('mousedown', function (e) { e.stopPropagation(); });
      k.addEventListener('click', function () { act('dock', 0, 0, b[1]); });
      bar.appendChild(k);
      docks.push({ k: k, edge: b[1] });
    });
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

    // The title bar moves the floating window: the placement follows the
    // pointer, and where it is let go the terminal puts the window, on whole
    // cells. Docked it stays put: across the top or the bottom the bar is
    // the drawer's handle, pulled in and out; at a side it is only a bar
    // (the ⧉ button floats it).
    bar.addEventListener('mousedown', function (e) {
      if (e.button !== 0) return;
      var at = (L && L.Dock) || 'float';
      if (at === 'bottom') return size(el, e, 'n');
      if (at === 'top') return size(el, e, 's');
      if (at !== 'float') return;
      e.preventDefault();
      var x0 = e.clientX, y0 = e.clientY, l0 = el.offsetLeft, t0 = el.offsetTop;
      var moved = false;
      function move(m) {
        if (m.buttons === 0) return; // no button held: not this drag
        if (Math.abs(m.clientX - x0) + Math.abs(m.clientY - y0) > 3) moved = true;
        if (!moved) return;
        el.style.left = (l0 + m.clientX - x0) + 'px';
        el.style.top = (t0 + m.clientY - y0) + 'px';
      }
      function up(m) {
        doc.removeEventListener('mousemove', move);
        doc.removeEventListener('mouseup', up);
        // A click on the title is no move: docked, it would float it.
        if (!moved) return;
        var c = cell(el, L && L.Window);
        if (c[0] > 0 && c[1] > 0) act('move', Math.round(el.offsetLeft / c[0]), Math.round(el.offsetTop / c[1]));
      }
      doc.addEventListener('mousemove', move);
      doc.addEventListener('mouseup', up);
    });
    bar.addEventListener('dblclick', function () { act('full', 0, 0); });

    // The edges and corners size the window (size); docked() shows the
    // ones that do where it is. They reach past the window, which must not
    // clip them; the body scrolls the panel inside it.
    el.style.overflow = 'visible';
    var grips = Object.keys(gripAt).map(function (dir) {
      var g = doc.createElement('div');
      g.title = 'drag to size the window';
      g.style.cssText = 'position:absolute;z-index:3;' + gripAt[dir];
      g.addEventListener('mousedown', function (e) { size(el, e, dir); });
      el.appendChild(g);
      return { el: g, dir: dir };
    });

    var p = P = { el: el, bar: bar, wrap: wrap, docks: docks, grips: grips, frame: take(frame) };
    wrap.appendChild(frame);
    if (away && held(el)) home(p.frame); else refit();
    fit();
    docked();
    // A window docked or floated is a new width, which the frame fits.
    var ro = new ResizeObserver(function () { fit(); refit(); });
    ro.observe(el);
    // The page's go · tinygo switch is fixed in the top right corner, over
    // everything: a window docked at the top or the right would have its
    // buttons under it. Where the window is, the switch steps aside.
    var sw = doc.getElementById('wasm-switch');
    function clear() {
      if (!sw) return;
      var a = el.getBoundingClientRect(), b = sw.getBoundingClientRect();
      var over = a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;
      sw.style.visibility = over ? 'hidden' : '';
    }
    var mo = new MutationObserver(clear);
    mo.observe(el, { attributes: true, attributeFilter: ['style'] });
    clear();
    return function () {
      ro.disconnect();
      mo.disconnect();
      if (sw) sw.style.visibility = '';
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
    if (!(away && held(el))) el.appendChild(c);
    return function () {
      home(s.canvas);
      c.style.position = pos; c.style.left = left; c.style.top = top;
      if (S === s) S = null;
    };
  }
};
`
