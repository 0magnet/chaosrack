//go:build js && wasm

// Package rack lays out panel modules the way a physical equipment rack does:
// side by side, each one a whole number of slots wide, wrapping into rows and
// reflowing when one is taken out.
//
// It is not a window manager. Windows overlap, are positioned absolutely, and
// are arranged by the person using them; a rack is a constraint — nothing
// overlaps, everything sits on the same pitch, and taking a module out closes
// the gap. The two compose rather than compete: put a rack inside a window
// (github.com/0magnet/winbox-go docks one to a screen edge) and the rack does
// not care that it is in a window, while the window does not care what it
// holds.
//
// What the rack provides:
//
//   - slot quantization — a module is measured, then snapped to the smallest
//     whole number of slots that holds it, so module edges line up down the
//     panel and across wrapped rows
//   - show and hide, with the remaining modules reflowing
//   - reordering by dragging a module's header into the gap you want it in
//   - one scale factor over the whole rack, so "make the panel bigger" is a
//     single number rather than a per-module size
//
// It provides no appearance beyond structure. The embedded stylesheet sets
// layout — flex wrap, the module box, the header strip — and no colors, fonts
// or control styling, because a rack that dictated those would be a theme
// wearing a layout engine's name.
package rack

import (
	"math"
	"strconv"
	"syscall/js"
)

var (
	document = js.Global().Get("document")
	window   = js.Global().Get("window")
)

// Options configures a Rack. The zero value of every field has a working
// default, so New(Options{}) builds a usable rack in a fresh container.
type Options struct {
	// Container is the element the modules live in. When it is not set, New
	// creates a div and leaves it to the caller to place via Root.
	Container js.Value

	// SlotWidth is the pitch a module's width is snapped to, before Scale.
	// Defaults to 132px, which is about the width of one column of controls.
	SlotWidth float64

	// Gap is the space between modules, and must match the stylesheet's gap.
	// Defaults to 4px.
	Gap float64

	// Chrome is the module's own horizontal padding and borders — everything
	// between the module's edge and the content span that gets measured.
	// Defaults to 18px, matching the stylesheet's 8px padding either side plus
	// a 1px border either side.
	//
	// It is a separate number from Gap and SlotWidth because it does not scale
	// with them: a border stays one pixel wide however big the rack is. A host
	// that styles its modules with different padding has to say so here, or
	// every module will be measured against the wrong margin and the ones that
	// nearly fit will take an extra slot.
	Chrome float64

	// Scale multiplies the slot pitch, so the whole rack grows or shrinks
	// together. Defaults to 1. It is also published as the CSS custom property
	// --rack-scale on the container, for a host's own sizing to follow.
	Scale float64

	// ModuleClass and HeaderClass name the elements the rack manages. They are
	// configurable because a host with an existing panel should be able to
	// adopt the rack without renaming everything it already styles.
	// Default to "rack-mod" and "rack-mod-hdr".
	ModuleClass string
	HeaderClass string

	// SwitchClass and SwitchLabelClass name the elements Switches builds, for
	// the same reason: a host that already styles its checkboxes should get its
	// own checkboxes back, not unstyled ones that happen to work. SwitchClass
	// defaults to empty (a bare input) and SwitchLabelClass to "rack-switch".
	SwitchClass      string
	SwitchLabelClass string

	// ContentSelector finds the element inside a module whose natural span
	// decides how many slots the module needs. It is a selector list, tried in
	// order. Defaults to ".rack-grid".
	//
	// This is a parameter rather than "measure the module" because the module
	// is the thing being sized: measuring it would ask how wide it is in order
	// to decide how wide it should be.
	ContentSelector string

	// Pinned reports whether a module may not be hidden. A rack whose every
	// module can be put away can be emptied to the point where there is no way
	// to bring anything back; the host names the module that holds the
	// controls for that.
	Pinned func(key string) bool

	// Reorderable turns header dragging on. Defaults to on; set Fixed to
	// disable it.
	Fixed bool

	// OnReorder and OnVisibility report changes the user made, by module key,
	// so a host can persist them.
	OnReorder    func(order []string)
	OnVisibility func(key string, shown bool)
}

// Rack is one rack of modules.
type Rack struct {
	opts   Options
	root   js.Value
	hidden map[string]bool
	funcs  []js.Func

	dragHeld    js.Value
	dragMoving  bool
	dragPointer js.Value
}

// New builds a rack over the given container, injecting the stylesheet on
// first use.
func New(opts Options) *Rack {
	if opts.SlotWidth <= 0 {
		opts.SlotWidth = 132
	}
	if opts.Gap < 0 {
		opts.Gap = 0
	} else if opts.Gap == 0 {
		opts.Gap = 4
	}
	if opts.Chrome == 0 {
		opts.Chrome = 18
	} else if opts.Chrome < 0 {
		opts.Chrome = 0
	}
	if opts.Scale <= 0 {
		opts.Scale = 1
	}
	if opts.ModuleClass == "" {
		opts.ModuleClass = "rack-mod"
	}
	if opts.HeaderClass == "" {
		opts.HeaderClass = "rack-mod-hdr"
	}
	if opts.ContentSelector == "" {
		opts.ContentSelector = ".rack-grid"
	}
	if opts.SwitchLabelClass == "" {
		opts.SwitchLabelClass = "rack-switch"
	}

	InjectCSS()

	r := &Rack{opts: opts, hidden: map[string]bool{}}
	r.root = opts.Container
	if !r.root.Truthy() {
		r.root = document.Call("createElement", "div")
	}
	r.root.Get("classList").Call("add", "rack")
	r.setScaleProperty()

	if !opts.Fixed {
		r.wireDrag()
	}
	return r
}

// Root is the rack's container element — what to append to a page, or hand to
// a window manager to mount.
func (r *Rack) Root() js.Value { return r.root }

// Release drops the rack's event listeners. Only needed if the rack is being
// discarded while the page lives on; a rack that lasts as long as the page
// does not need it.
func (r *Rack) Release() {
	for _, f := range r.funcs {
		f.Release()
	}
	r.funcs = nil
}

// track keeps a js.Func alive for the rack's lifetime and returns it.
func (r *Rack) track(fn func(this js.Value, args []js.Value) interface{}) js.Func {
	f := js.FuncOf(fn)
	r.funcs = append(r.funcs, f)
	return f
}

// ── Modules ──────────────────────────────────────────────────────────────────

// Modules is every module element in the rack, in the order they appear.
func (r *Rack) Modules() []js.Value {
	kids := r.root.Get("children")
	n := kids.Get("length").Int()
	out := make([]js.Value, 0, n)
	for i := 0; i < n; i++ {
		m := kids.Index(i)
		if m.Get("classList").Call("contains", r.opts.ModuleClass).Bool() {
			out = append(out, m)
		}
	}
	return out
}

// Key names a module by its header text, lowercased. The header is what a
// switch or a menu would be labeled with anyway, so deriving the key from it
// means the two cannot drift apart.
func (r *Rack) Key(module js.Value) string {
	h := module.Call("querySelector", "."+r.opts.HeaderClass)
	if !h.Truthy() {
		return ""
	}
	return normalizeKey(h.Get("textContent").String())
}

// Title is a module's header text as written, for labeling a control with.
func (r *Rack) Title(module js.Value) string {
	h := module.Call("querySelector", "."+r.opts.HeaderClass)
	if !h.Truthy() {
		return ""
	}
	return trimSpace(h.Get("textContent").String())
}

// Module finds a module by key, or a zero Value.
func (r *Rack) Module(key string) js.Value {
	for _, m := range r.Modules() {
		if r.Key(m) == key {
			return m
		}
	}
	return js.Value{}
}

// Add appends a module element to the rack and re-quantizes.
func (r *Rack) Add(module js.Value) *Rack {
	module.Get("classList").Call("add", r.opts.ModuleClass)
	r.root.Call("appendChild", module)
	r.Quantize()
	return r
}

// ── Layout ───────────────────────────────────────────────────────────────────

// Quantize snaps every module's width to a whole number of slots.
//
// A module is first widened out of the way so its content lays out at its
// natural size, then measured from the positions of the content's own children
// rather than from any width property — a wrapped flex column misreports both
// max-content and scrollWidth, and believing either produces modules that are
// one slot too narrow and clip.
//
// An N-slot module also spans the N-1 gaps it covers, so its right edge lands
// where N separate one-slot modules would end and corners line up across
// wrapped rows.
func (r *Rack) Quantize() *Rack {
	// A rack that is not being displayed measures every module at zero and
	// would lock them all to one slot. Leave the widths alone; whoever shows
	// it re-quantizes.
	if !r.visible() {
		return r
	}

	slot := r.opts.SlotWidth * r.opts.Scale
	for _, m := range r.Modules() {
		if isHidden(m) {
			continue
		}
		style := m.Get("style")
		style.Set("width", "3000px")

		w := r.contentSpan(m)

		// The span is measured edge to edge of the content's own children, which
		// sit inside the module's padding and borders — that is Chrome. Columns
		// sit on the slot pitch, which is the slot plus one gap.
		slots := int(math.Ceil((w + r.opts.Chrome) / (slot + r.opts.Gap)))
		if slots < 1 {
			slots = 1
		}
		width := float64(slots)*slot + float64(slots-1)*r.opts.Gap
		style.Set("width", strconv.FormatFloat(width, 'f', 0, 64)+"px")
	}
	return r
}

// contentSpan is how wide a module's content actually is, taken from the
// left-most and right-most edges of the content element's children.
func (r *Rack) contentSpan(module js.Value) float64 {
	content := module.Call("querySelector", r.opts.ContentSelector)
	if !content.Truthy() {
		return 0
	}
	items := content.Get("children")
	minL, maxR := math.Inf(1), math.Inf(-1)
	found := false
	for i := 0; i < items.Get("length").Int(); i++ {
		it := items.Index(i)
		// Out-of-flow children are not part of the span — a fixed or absolute
		// item can sit anywhere on the page and would inflate the module to
		// the width of wherever it landed.
		if pos := computedStyle(it).Get("position").String(); pos == "fixed" || pos == "absolute" {
			continue
		}
		rect := it.Call("getBoundingClientRect")
		l, right := rect.Get("left").Float(), rect.Get("right").Float()
		if right-l <= 0 {
			continue // hidden or zero-size
		}
		found = true
		if l < minL {
			minL = l
		}
		if right > maxR {
			maxR = right
		}
	}
	if !found {
		return 0
	}
	return maxR - minL
}

// Scale is the current rack scale.
func (r *Rack) Scale() float64 { return r.opts.Scale }

// SetScale grows or shrinks the whole rack together, clamped to a range that
// keeps modules legible at one end and on the screen at the other.
func (r *Rack) SetScale(v float64) *Rack {
	if v < 0.6 {
		v = 0.6
	} else if v > 2.2 {
		v = 2.2
	}
	r.opts.Scale = v
	r.setScaleProperty()
	r.Quantize()
	return r
}

func (r *Rack) setScaleProperty() {
	r.root.Get("style").Call("setProperty", "--rack-scale",
		strconv.FormatFloat(r.opts.Scale, 'f', 3, 64))
}

// visible reports whether the rack is currently laid out. A display:none
// ancestor makes every measurement zero, which is not the same as a module
// having no content.
func (r *Rack) visible() bool {
	return r.root.Get("offsetParent").Truthy() ||
		computedStyle(r.root).Get("display").String() != "none"
}

// ── helpers ──────────────────────────────────────────────────────────────────

func computedStyle(el js.Value) js.Value {
	return window.Call("getComputedStyle", el)
}

func isHidden(el js.Value) bool {
	return el.Get("style").Get("display").String() == "none"
}

// trimSpace and normalizeKey avoid pulling in strings for two operations, which
// matters under TinyGo where every package costs binary size.
func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && isSpace(s[i]) {
		i++
	}
	for j > i && isSpace(s[j-1]) {
		j--
	}
	return s[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func normalizeKey(s string) string {
	s = trimSpace(s)
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
