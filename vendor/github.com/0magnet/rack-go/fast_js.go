//go:build js && wasm

package rack

import (
	"encoding/json"
	"syscall/js"
)

// The measuring passes, done in JavaScript.
//
// Quantize and Apply walk every module, and Quantize walks every child of
// every module's content row. In the browser that is the same handful of
// reads per element — a computed style, a bounding rect, a class test — and
// from Go each one is a crossing into JavaScript that comes back wrapped in
// a js.Value. A js.Value holding a JS *object* or *string* carries a runtime
// finalizer so the reference table entry can be released, and attaching one
// (runtime.addspecial) is the most expensive thing this package does. A
// js.Value holding a number carries none.
//
// Measured in chaosrack, a rack of about eighty modules: one Quantize cost
// 208ms, 149ms of it inside contentSpan, against 0.7ms for the same reads
// written in JavaScript. A forced layout of the whole ten-thousand-element
// page is 0.3ms, so the expense was never the browser recomputing anything
// — it was the trip.
//
// So the loops live here, in one string, and Go crosses once per pass and
// makes no decisions it has not already made. Nothing below chooses
// anything: the slot arithmetic is the same expression Quantize used, and
// the key normalizing is the same ASCII-only rule as normalizeKey, spelled
// out rather than handed to toLowerCase and trim, which also fold characters
// Go's version leaves alone.
//
// The Go loops are kept as the fallback — see fast(). A page that forbids
// eval still gets a correct rack, only a slower one.
const fastSource = `(function () {
  function trimASCII(s) {
    return String(s).replace(/^[ \t\n\r\v\f]+/, "").replace(/[ \t\n\r\v\f]+$/, "");
  }
  function key(s) {
    return trimASCII(s).replace(/[A-Z]/g, function (c) { return c.toLowerCase(); });
  }
  function modules(root, moduleClass) {
    var kids = root.children, out = [];
    for (var i = 0; i < kids.length; i++) {
      var m = kids[i];
      if (m.classList && m.classList.contains(moduleClass)) out.push(m);
    }
    return out;
  }
  function headerKey(m, headerClass) {
    var h = m.querySelector("." + headerClass);
    return h ? key(h.textContent) : "";
  }
  return {
    // keys is every module's key, in rack order.
    keys: function (root, moduleClass, headerClass) {
      var ms = modules(root, moduleClass), out = [];
      for (var i = 0; i < ms.length; i++) out.push(headerKey(ms[i], headerClass));
      return JSON.stringify(out);
    },
    // hide puts away every module whose key is in the given set, and leaves
    // the rest alone — Apply's rule, not a general show/hide.
    hide: function (root, moduleClass, headerClass, keysJSON) {
      var want = {}, list = JSON.parse(keysJSON);
      for (var i = 0; i < list.length; i++) want[list[i]] = true;
      var ms = modules(root, moduleClass);
      for (var j = 0; j < ms.length; j++) {
        var k = headerKey(ms[j], headerClass);
        if (k && want[k]) ms[j].style.display = "none";
      }
    },
    // quantize snaps every visible module to a whole number of slots,
    // measured from the span of its content's own in-flow children.
    //
    // In three phases, and that is the point of it. Widening a module is a
    // write and measuring one is a read, so doing both per module made the
    // browser re-lay the page out once per module — eighty forced layouts
    // where two will do. Every module is widened first, then every one is
    // measured, then every width is set.
    quantize: function (root, moduleClass, contentSel, slot, gap, chrome) {
      return this.quantizeAll([root], moduleClass, contentSel, slot, gap, chrome);
    },
    // quantizeAll does the same for several racks at once, and that is the
    // whole reason it exists. A rack of sixteen bays quantized one bay at a
    // time widened, measured and set sixteen times over, and every measure
    // after a set made the browser recompute style and layout for the whole
    // page: measured, 106 style recalculations and layouts in one model
    // change, 278ms of the 602ms it took. Widening every module in every bay
    // before measuring any of them makes that two.
    quantizeAll: function (roots, moduleClass, contentSel, slot, gap, chrome) {
      var win = globalThis.window || globalThis;
      var live = [], i, j, r, ms;
      for (r = 0; r < roots.length; r++) {
        ms = modules(roots[r], moduleClass);
        for (i = 0; i < ms.length; i++) {
          if (ms[i].style && ms[i].style.display === "none") continue;
          ms[i].style.width = "3000px";
          live.push(ms[i]);
        }
      }
      var widths = [];
      for (i = 0; i < live.length; i++) {
        var content = live[i].querySelector(contentSel);
        if (!content) { widths.push(-1); continue; }
        var items = content.children, minL = Infinity, maxR = -Infinity, found = false;
        for (j = 0; j < items.length; j++) {
          var it = items[j];
          var pos = win.getComputedStyle(it).position;
          if (pos === "fixed" || pos === "absolute") continue;
          var rect = it.getBoundingClientRect();
          var l = rect.left, right = rect.right;
          if (right - l <= 0) continue;
          found = true;
          if (l < minL) minL = l;
          if (right > maxR) maxR = right;
        }
        widths.push(found ? maxR - minL : 0);
      }
      for (i = 0; i < live.length; i++) {
        if (widths[i] < 0) continue; // no content row; leave it at 3000px, as before
        var slots = Math.ceil((widths[i] + chrome) / (slot + gap));
        if (slots < 1) slots = 1;
        live[i].style.width = (slots * slot + (slots - 1) * gap).toFixed(0) + "px";
      }
    }
  };
})()`

var (
	fastHelper js.Value
	fastTried  bool
)

// fast is the JS helper, or a zero Value if this page will not evaluate it.
//
// Tried once. A CSP without 'unsafe-eval' makes the call throw, which reaches
// Go as a panic, so the recover is the whole point: the rack still lays out,
// by the Go loops, and nothing above has to know which ran.
func fast() (v js.Value) {
	if fastTried {
		return fastHelper
	}
	fastTried = true
	defer func() {
		if recover() != nil {
			fastHelper = js.Value{}
			v = js.Value{}
		}
	}()
	fastHelper = js.Global().Call("eval", fastSource)
	return fastHelper
}

// hideKeys puts away every module in the hidden set, in one crossing.
//
// Reports whether it did. An empty set is still a success — Apply's contract
// is that hiding only ever hides, so there is nothing to do and nothing to
// fall back to.
func (r *Rack) hideKeys(h js.Value, keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return false
	}
	h.Call("hide", r.root, r.opts.ModuleClass, r.opts.HeaderClass, string(b))
	return true
}

// QuantizeAll snaps every module in several racks in one pass.
//
// A frame of sixteen bays is sixteen racks, and quantizing them one at a
// time widened, measured and set sixteen times over. Each measure after a
// set makes the browser recompute style and layout for the whole page, so
// the cost is not sixteen small layouts but sixteen full ones: measured in
// chaosrack, 106 style recalculations and layouts in a single model change,
// 278ms of the 602ms it took.
//
// Widening every module in every bay before measuring any of them makes
// that two. The racks must agree on the slot geometry, which racks sharing
// a frame do; any that do not are quantized on their own.
func QuantizeAll(rs []*Rack) {
	h := fast()
	if !h.Truthy() || len(rs) == 0 {
		for _, r := range rs {
			r.Quantize()
		}
		return
	}
	var same []*Rack
	var odd []*Rack
	first := rs[0]
	for _, r := range rs {
		if r == nil || !r.visible() {
			continue
		}
		if r.sameGeometry(first) {
			same = append(same, r)
		} else {
			odd = append(odd, r)
		}
	}
	for _, r := range odd {
		r.Quantize()
	}
	if len(same) == 0 {
		return
	}
	roots := js.Global().Get("Array").New(len(same))
	for i, r := range same {
		roots.SetIndex(i, r.root)
	}
	h.Call("quantizeAll", roots, first.opts.ModuleClass, first.opts.ContentSelector,
		first.opts.SlotWidth*first.opts.Scale, first.opts.Gap, first.opts.Chrome)
}

// sameGeometry reports whether two racks snap to the same grid, and so can
// be measured together.
func (r *Rack) sameGeometry(o *Rack) bool {
	return r.opts.ModuleClass == o.opts.ModuleClass &&
		r.opts.ContentSelector == o.opts.ContentSelector &&
		r.opts.SlotWidth == o.opts.SlotWidth &&
		r.opts.Scale == o.opts.Scale &&
		r.opts.Gap == o.opts.Gap &&
		r.opts.Chrome == o.opts.Chrome
}

// ApplyAll puts away what has been hidden in every rack, then quantizes them
// together. Apply on each would quantize once per rack; see QuantizeAll.
func ApplyAll(rs []*Rack) {
	h := fast()
	if !h.Truthy() {
		for _, r := range rs {
			r.Apply()
		}
		return
	}
	for _, r := range rs {
		if r == nil {
			continue
		}
		if !r.hideKeys(h, r.HiddenKeys()) {
			for _, m := range r.Modules() {
				key := r.Key(m)
				if key == "" || !r.hidden[key] {
					continue
				}
				m.Get("style").Set("display", "none")
			}
		}
	}
	QuantizeAll(rs)
}
