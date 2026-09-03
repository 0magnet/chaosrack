//go:build js && wasm

package rack

// Taking modules out and putting them back.
//
// The rule that shapes this: hiding only ever hides. Show is an explicit act,
// never a side effect of a rebuild. A host that rebuilds its panel when
// something else changes — a mode switch, a new document — has its own reasons
// for a module being absent, and a visibility pass that helpfully un-hid
// everything it did not recognize would fight those reasons every time.
//
// The same rule is why a pinned module exists. Every module can be put away
// because the way to bring one back is always somewhere that cannot itself be
// put away; without that, a rack can be emptied into a state with no way out.

import "syscall/js"

// Hidden reports whether the module with this key is currently put away.
func (r *Rack) Hidden(key string) bool { return r.hidden[key] }

// Hide puts a module away. A pinned module is left alone, and Hide reports
// whether it did anything.
func (r *Rack) Hide(key string) bool {
	if r.opts.Pinned != nil && r.opts.Pinned(key) {
		return false
	}
	if r.hidden[key] {
		return false
	}
	r.hidden[key] = true
	r.Apply()
	if r.opts.OnVisibility != nil {
		r.opts.OnVisibility(key, false)
	}
	return true
}

// Show brings a module back.
//
// This is the only place that makes a module visible. Apply cannot do it: it
// runs after every rebuild, over elements it did not create, and the one thing
// it must not do is overrule a host that had its own reason to hide something.
func (r *Rack) Show(key string) bool {
	if !r.hidden[key] {
		return false
	}
	delete(r.hidden, key)
	for _, m := range r.Modules() {
		if r.Key(m) == key {
			m.Get("style").Set("display", "")
		}
	}
	r.Quantize()
	if r.opts.OnVisibility != nil {
		r.opts.OnVisibility(key, true)
	}
	return true
}

// Toggle flips a module's visibility and returns whether it is now shown.
func (r *Rack) Toggle(key string) bool {
	if r.hidden[key] {
		r.Show(key)
		return true
	}
	r.Hide(key)
	return !r.hidden[key]
}

// Apply puts away what has been hidden and leaves everything else alone. Call
// it after rebuilding the modules: a rebuild replaces the elements the last
// pass acted on, so the hiding has to be re-applied to the new ones.
func (r *Rack) Apply() *Rack {
	for _, m := range r.Modules() {
		key := r.Key(m)
		if key == "" || !r.hidden[key] {
			continue
		}
		m.Get("style").Set("display", "none")
	}
	r.Quantize()
	return r
}

// HiddenKeys is every module currently put away, for persisting.
func (r *Rack) HiddenKeys() []string {
	out := make([]string, 0, len(r.hidden))
	for k, v := range r.hidden {
		if v {
			out = append(out, k)
		}
	}
	return out
}

// SetHidden replaces the hidden set wholesale, for restoring a saved rack.
// Keys naming modules that are not present are kept: a module may not have
// been built yet, and forgetting it would show it the moment it appears.
func (r *Rack) SetHidden(keys []string) *Rack {
	shown := map[string]bool{}
	for k := range r.hidden {
		shown[k] = true
	}
	r.hidden = map[string]bool{}
	for _, k := range keys {
		r.hidden[k] = true
		delete(shown, k)
	}
	// Anything that was hidden and no longer is has to be told so explicitly,
	// for the reason in Show's comment.
	for k := range shown {
		for _, m := range r.Modules() {
			if r.Key(m) == k {
				m.Get("style").Set("display", "")
			}
		}
	}
	r.Apply()
	return r
}

// Switches fills a host element with one checkbox per module, labeled with the
// module's title, that puts it away and brings it back. Pinned modules are
// skipped — a switch that could hide the switches would be a door that locks
// from the outside.
//
// Built from the rack as it stands, so call it after the modules exist. It
// returns the number of switches made.
func (r *Rack) Switches(host js.Value) int {
	if !host.Truthy() {
		return 0
	}
	seen := map[string]bool{}
	n := 0
	for _, m := range r.Modules() {
		key := r.Key(m)
		if key == "" || seen[key] {
			continue
		}
		if r.opts.Pinned != nil && r.opts.Pinned(key) {
			continue
		}
		seen[key] = true

		label := document.Call("createElement", "label")
		label.Set("className", r.opts.SwitchLabelClass)
		label.Set("title", "Show or hide the "+key+" module")

		cb := document.Call("createElement", "input")
		cb.Set("type", "checkbox")
		if r.opts.SwitchClass != "" {
			cb.Set("className", r.opts.SwitchClass)
		}
		cb.Set("checked", !r.hidden[key])
		cb.Call("setAttribute", "data-rack-module", key)

		k := key
		cb.Call("addEventListener", "change", r.track(func(this js.Value, _ []js.Value) interface{} {
			if this.Get("checked").Bool() {
				r.Show(k)
			} else {
				r.Hide(k)
			}
			return nil
		}))

		label.Call("appendChild", cb)
		label.Call("appendChild", document.Call("createTextNode", " "+r.Title(m)))
		host.Call("appendChild", label)
		n++
	}
	return n
}
