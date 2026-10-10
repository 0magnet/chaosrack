//go:build js && wasm

package seat

import (
	"errors"
	"strconv"
	"syscall/js"
	"time"
)

// Screen is one of the seat's screens: a console, a desktop, a program that
// takes the whole page. Mount renders it into el, which fills the seat; it is
// called once, the first time the screen is shown. Close tears it down.
type Screen interface {
	Mount(el js.Value) error
	Close()
}

// Shower is a Screen that is told when it comes to the front (true) and when
// it leaves it (false): to pause drawing nobody sees, say.
type Shower interface {
	Shown(front bool)
}

// Options configure a Seat. The zero value is usable.
type Options struct {
	// Root is the element the screens fill. Zero is the whole window: an
	// element of the seat's own, fixed over the page.
	Root js.Value
	// NoKeys withholds Ctrl+Alt+1…9, for a page that switches by its own
	// means only.
	NoKeys bool
	// ZIndex is the screens' stacking order over the page; zero is very high,
	// above anything a page is likely to float.
	ZIndex int
}

// Seat holds the screens and shows one at a time.
type Seat struct {
	root    js.Value
	screens []*screen
	cur     int
	keyFn   js.Func
	osd     js.Value
	osdT    *time.Timer
	on      []func(from, to string)
}

type screen struct {
	name string
	// sc is a screen of its own; nil for a page screen (AddPage), which is
	// the page under the seat, shown by hiding every other.
	sc      Screen
	shown   func(bool)
	el      js.Value
	mounted bool
	// focus is what had the keyboard when the screen was left, given it back
	// when it returns: a console's terminal, a desktop's window.
	focus js.Value
}

// document is looked up when used, not once: a page builds its own body
// before it builds a seat, and a test installs one.
func document() js.Value { return js.Global().Get("document") }

// New makes a seat. It shows nothing until Show.
func New(opt Options) *Seat {
	s := &Seat{root: opt.Root, cur: -1}
	if !s.root.Truthy() {
		z := opt.ZIndex
		if z == 0 {
			z = 2147483000
		}
		s.root = document().Call("createElement", "div")
		s.root.Set("className", "seat")
		s.root.Get("style").Set("cssText", "position:fixed;inset:0;pointer-events:none;z-index:"+strconv.Itoa(z))
		document().Get("body").Call("appendChild", s.root)
	}
	if !opt.NoKeys {
		s.keyFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
			e := a[0]
			i := keyIndex(e.Get("ctrlKey").Bool(), e.Get("altKey").Bool(), e.Get("shiftKey").Bool(),
				e.Get("metaKey").Bool(), e.Get("code").String(), e.Get("key").String())
			if i < 0 || i >= len(s.screens) {
				return nil
			}
			// Before anything on the page sees it: a terminal with the
			// keyboard would otherwise take Ctrl+Alt+2 as a key of its own.
			e.Call("preventDefault")
			e.Call("stopPropagation")
			if err := s.showAt(i); err != nil {
				s.say(strconv.Itoa(i+1) + " · " + s.screens[i].name + ": " + err.Error())
			}
			return nil
		})
		js.Global().Call("addEventListener", "keydown", s.keyFn, map[string]any{"capture": true})
	}
	return s
}

// Add makes sc a screen called name, after the others.
func (s *Seat) Add(name string, sc Screen) {
	s.screens = append(s.screens, &screen{name: name, sc: sc})
}

// AddPage makes the page itself a screen called name: shown, every screen
// of the seat's own is hidden and the page shows through. shown, if not nil,
// is told when it comes to the front and when it leaves, so one page can be
// several screens — an instrument and a desktop over the same scene, say.
func (s *Seat) AddPage(name string, shown func(front bool)) {
	s.screens = append(s.screens, &screen{name: name, shown: shown})
}

// OnSwitch runs f after each switch, with the names of the screens.
func (s *Seat) OnSwitch(f func(from, to string)) { s.on = append(s.on, f) }

// Names is the screens' names, in order: Ctrl+Alt+1 is the first.
func (s *Seat) Names() []string {
	out := make([]string, len(s.screens))
	for i, sc := range s.screens {
		out[i] = sc.name
	}
	return out
}

// Current is the name of the screen in front, or "" before the first Show.
func (s *Seat) Current() string {
	if s.cur < 0 {
		return ""
	}
	return s.screens[s.cur].name
}

// ErrNoScreen is a Show of a name no screen has.
var ErrNoScreen = errors.New("seat: no such screen")

// Show brings the screen called name to the front.
func (s *Seat) Show(name string) error {
	for i, sc := range s.screens {
		if sc.name == name {
			return s.showAt(i)
		}
	}
	return ErrNoScreen
}

func (s *Seat) showAt(i int) error {
	if i == s.cur {
		return nil
	}
	next := s.screens[i]
	if next.sc != nil && !next.mounted {
		next.el = document().Call("createElement", "div")
		next.el.Set("className", "seat-screen")
		next.el.Get("style").Set("cssText", "position:absolute;inset:0;background:#000;pointer-events:auto;visibility:hidden")
		next.el.Call("setAttribute", "inert", "")
		s.root.Call("appendChild", next.el)
		if err := next.sc.Mount(next.el); err != nil {
			next.el.Call("remove")
			next.el = js.Value{}
			return err
		}
		next.mounted = true
	}
	from := ""
	if s.cur >= 0 {
		prev := s.screens[s.cur]
		from = prev.name
		prev.focus = document().Get("activeElement")
		if prev.el.Truthy() {
			// Hidden and inert, not taken out of the layout: a terminal or a
			// canvas laid out at nothing has nothing to measure itself by when
			// it comes back.
			prev.el.Get("style").Set("visibility", "hidden")
			prev.el.Call("setAttribute", "inert", "")
		}
		tell(prev, false)
	}
	s.cur = i
	if next.el.Truthy() {
		next.el.Get("style").Set("visibility", "visible")
		next.el.Call("removeAttribute", "inert")
	}
	tell(next, true)
	s.refocus(next)
	s.say(strconv.Itoa(i+1) + " · " + next.name)
	for _, f := range s.on {
		f(from, next.name)
	}
	return nil
}

func tell(sc *screen, front bool) {
	if sh, ok := sc.sc.(Shower); ok {
		sh.Shown(front)
	}
	if sc.shown != nil {
		sc.shown(front)
	}
}

// refocus gives the keyboard back to what had it on this screen, or, the
// first time, to the first thing on it that takes keys.
func (s *Seat) refocus(sc *screen) {
	f := sc.focus
	if (!f.Truthy() || !f.Get("isConnected").Bool()) && sc.el.Truthy() {
		f = sc.el.Call("querySelector", "textarea, input, [tabindex]")
	}
	if f.Truthy() && f.Get("focus").Type() == js.TypeFunction {
		f.Call("focus", map[string]any{"preventScroll": true})
	}
}

// say shows text for a moment over everything: where a switch went, as one
// made by a key should say, or why it went nowhere.
func (s *Seat) say(text string) {
	if !s.osd.Truthy() {
		s.osd = document().Call("createElement", "div")
		s.osd.Set("className", "seat-osd")
		s.osd.Get("style").Set("cssText", "position:fixed;top:12px;left:50%;transform:translateX(-50%);"+
			"padding:4px 12px;border-radius:6px;background:rgba(0,0,0,.75);color:#ddd;"+
			"font:13px/1.4 system-ui,sans-serif;pointer-events:none;transition:opacity .3s;opacity:0;z-index:2147483647")
		document().Get("body").Call("appendChild", s.osd)
	}
	s.osd.Set("textContent", text)
	s.osd.Get("style").Set("opacity", "1")
	if s.osdT != nil {
		s.osdT.Stop()
	}
	s.osdT = time.AfterFunc(1200*time.Millisecond, func() { s.osd.Get("style").Set("opacity", "0") })
}

// Close closes every screen and takes the seat off the page.
func (s *Seat) Close() {
	if s.keyFn.Truthy() {
		js.Global().Call("removeEventListener", "keydown", s.keyFn, map[string]any{"capture": true})
		s.keyFn.Release()
	}
	for _, sc := range s.screens {
		if sc.mounted {
			sc.sc.Close()
			sc.el.Call("remove")
		}
	}
	if s.osd.Truthy() {
		s.osd.Call("remove")
	}
	if s.root.Get("className").String() == "seat" {
		s.root.Call("remove")
	}
	s.screens, s.cur = nil, -1
}
