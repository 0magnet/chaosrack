# Desk, Contain and the rack bay

The window-manager modes and the rack frame. [← README](../README.md)

**Desk** (Console → Window) puts a window manager over the scene:
[desk](https://github.com/0magnet/desk) windows floating above the canvas,
with a [websh](https://github.com/0magnet/websh) shell and a file manager in
them, while the model keeps integrating underneath and stays tunable from every
knob on the rack. There is no second panel — the desk's own launcher is a bar
at the bottom of the page and so is this rack, and two of them would fight for
one edge — so the shell is the launcher: `apps` lists what can be opened,
`open files` and `term` open them.

The empty desktop does **not** swallow the mouse. Pointer events are off on the
desk's root and on again per window, so dragging where there is no window still
rotates what is behind them. The desk also gets a **z-index of its own**, which
makes it a stacking context: winbox numbers its windows from about 10 upward
and this panel sits at 10, so without one a raised window would climb over the
rack.

The selector beside the switch chooses one of **four 3-D desktops**, each a
reproduction of a real one, and each with the gesture that made it what it was:

| | | |
|---|---|---|
| **Flat** | — | ordinary windows |
| **Looking Glass** | Sun, 2003 | windows lean back in a legible stack; **double-click a title bar** to turn one over and read its back, which is what the original was actually for — the tilt everyone remembers was the side effect |
| **Cube** | Compiz, 2006 | four workspaces on the faces of a cube; the **arrow keys** spin it, and faces dim rather than vanish as they turn away |
| **Metisse** | 2004 | **shift-drag a title bar** to turn a window to any angle — and keep typing into it, because a CSS 3-D transform is a real projection and the browser hit-tests what it draws |
| **BumpTop** | 2009 | windows get weight and fall into a pile on top of the rack |

They are reproductions rather than pictures: the windows are real and the
shells in them are running.

The desk can also be drawn *into* the scene, as the **Desk** model (Solids →
Desk) — but only because desk was taught to draw its own chrome. A window is
more than its pane: the title, the buttons and the border are DOM, and DOM
cannot be sampled into a texture, so texturing the panes alone gives a desk of
frameless rectangles. desk's WebGL compositor can now rasterize each title bar
with Canvas2D — text, buttons and all — into the same canvas it draws the panes
into, and chaosrack samples that one canvas. You can work in it while it is a model, which took two goes at deciding it
was impractical before it turned out not to be.

The **keyboard** needs nothing clever: a key goes wherever the DOM focus is
and the windows are still in the DOM, hidden but present, so double-click the
canvas to type into the focused window and `Esc` to give the keyboard back.

The **mouse** is the half with a position, so the position is translated —
screen point to a ray, ray to the quad in model space, quad coordinates to
canvas pixels, and canvas pixels to a page point. That last step is what makes
the rest easy: the compositor's canvas is a 1:1 picture of the desk's own
rectangle, so a canvas pixel *is* a page point, and `elementFromPoint` hands
back the real element for winbox's own code to handle. Verified by clicking
where a window's close button is *drawn*, on a rotated quad, and having the
window close.

**Pass-thru** (Console → Window) chooses which gesture is the cheap one, and
**Ctrl inverts it** — so both are always available:

| | plain drag | Ctrl-drag |
|---|---|---|
| **Pass-thru off** | turn the model | reach the desk |
| **Pass-thru on** | reach the desk | turn the model |

The modifier is dropped from the synthesized event, or a Ctrl-drag would arrive
at a window as a Ctrl-click and open a context menu nobody asked for.

The cube is an honest approximation and not a closed box: windows cannot be
reparented into face elements without breaking dragging, so each is placed on
its face by transform alone about a shared origin, and faces holding windows of
different sizes do not tile.

**Contain** (Console → Window) turns the whole arrangement inside out. The two
above put the desk somewhere inside chaosrack — over the scene, or on a quad.
This puts *chaosrack inside the desk*: the desk's panel takes the bottom of the
screen, and the control panel becomes a **window on that desk**, with a task
button beside the desk's own windows. Minimize it and it goes to the panel;
click the button and it comes back. The background stays black, and the model
keeps running behind all of it.

It needed almost nothing new, which is worth saying because it sounds like a
rewrite. Three things already existed and only had to be pointed at each other:
the panel has been a real [winbox](https://github.com/0magnet/winbox-go) window
in float mode for a long time, with the minimize button every window gets;
desk's panel tracks a `Window` that is a *title, a focus function and a liveness
function* — not a winbox handle, not a desk-launched app — so it will adopt
anything that can say those three things; and the desk is already mounted,
already stacked between the canvas and the rack, and already knows how to keep
out of the mouse's way. So the rack is floated, described to the panel, and that
is the whole of it.

The **▤ button hides the environment**, not just the controls: the panel, the
desk's windows and the rack window all go, and the model is left on screen. The
rack window has to be hidden separately, because winbox mounts on `document.body`
— so the rack is not a *descendant* of the desk's element even while it is a
window *on* the desk. Hiding the desk alone left a bare title bar behind, since
▤ hides the panel's contents and the frame around them is not part of that.

Minimizing it also turned up a real bug one layer down, now fixed in winbox-go.
A window manager parks a minimized window in a slot along the bottom using the
same move-and-resize path a drag takes, so `OnMove` and `OnResize` fire for it —
and `Min` was set *after* the parking, leaving a callback no way to tell being
parked from being placed. Persisting geometry from those callbacks is what they
are for, so the panel saved the 251×35 slot as its remembered size, came back
from the minimize as a bare title bar, and came back that way on every later
load, because the wrong numbers were in `localStorage`. The flag goes up first
now; this end guards on it, and repairs a geometry already poisoned — all four
values, not the failing one, since the parked width landed eleven pixels above
the minimum and the parked *y* is a legal place to put a window and an
impossible place to put a 720-pixel-tall one.

**Rack bay** (Console → Window) draws the 19-inch frame the modules are
actually in: rails above and below the 3U opening, an ear each side with a
grab handle bolted to it, and blank panels filling the end of a row on the
same slot pitch as the modules. One frame **per row** — the modules wrap, and
a row is a bay, so a rack that has spilled onto a second row gets a second
frame. None of it is in the flow: a frame that was a flex item would wrap like
one and change which modules landed in which row.

Turning it on **resizes the interface to fit a whole 84 HP row**. A 19-inch
rack is 482.6 mm, which at the panel's own 4 px per millimeter is 1930 px —
wider than a 1920-pixel screen has left once the window's own furniture is
taken out. At scale 1 the bay could only ever hold eleven of its twelve slots
and the twelfth showed as a gap at the end of the row, so the bay shrinks the
scale until twelve fit. **Turning it off puts the size back** — the bay is a
view of the rack, not a change to it. It only fits once, when switched on, so
the Size knob still works while it is on: pick a bigger size and the bay keeps
it and simply draws as many whole slots as that size allows. Resize it
yourself and the old size is forgotten, because from then on the current one
is the one you asked for.

The tiling is [**0magnet/rack-go**](https://github.com/0magnet/rack-go), which
was lifted out of this app to be reusable: module widths snap to a whole
number of slots (`N·slot + (N−1)·gap`), the switches and the drag-to-reorder
are the rack's, and the host supplies only the CSS. The panel itself docks to
any edge or floats free — that half is
[**0magnet/winbox-go**](https://github.com/0magnet/winbox-go), where docking is
an opt-in mode layered on the draggable window.

