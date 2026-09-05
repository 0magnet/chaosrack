//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
)

// Permalink: serialize the live state into the URL hash and restore it on
// load, so any tuned view is shareable. The hash is
//
//	#<mode>[&<control>=<v>]...[&p.<param>=<v>]...[&rot=degX,degY,degZ][&m.<param>=<src>~<lvl>]...
//
// Only values that differ from their pristine defaults are written, so a
// clean load stays short. The hash is refreshed via history.replaceState
// (no reload, no history spam). The orientation quaternion is included
// only when the model is held still (auto-rotate off, spin rates 0) so it
// doesn't churn the URL while rotating.

// permaCtl maps a hash key to a DOM control. check = checkbox (else the
// element's .value is used; color inputs are stored without the leading #).
type permaCtl struct {
	key   string
	id    string
	check bool
}

// Ordered so "am" (audio-mod) is applied last — after params and mod
// routing are in place — so its panel rebuild reflects them.
// Numeric registry-owned controls (zoom, pans, spin rates, speed, trail,
// line, rainbow, sonify rate/level, …) are NOT listed here: they serialize
// and restore straight from builtControls via their ControlDesc.PermaKey —
// the row below would just duplicate the descriptor. This table now carries
// only the kinds the registry can't express yet: selects, colors, checkboxes.
var permaCtls = []permaCtl{
	// The backdrop was carried by nothing at all while it was four switches —
	// none of them was ever enrolled here, so a link with a spectrogram behind
	// the model arrived without one. It is a select now, which is a kind this
	// table can express, so it is in.
	{"bd", "bg-visual", false},
	{"sm", "sonify-map", false},
	{"sn", "sonify-mode", false},
	{"gs", "gradient-source", false},
	{"gc", "gradient-colors", false},
	{"sr", "step-ratio", false},
	{"fn", "fine-ratio", false},
	{"ks", "knob-size", false},
	{"cb", "color-base", false},
	{"cm", "color-mid", false},
	{"ct", "color-top", false},
	{"cg", "color-bg", false},
	{"ar", "auto-rotate", true},
	{"pt", "use-points", true},
	{"ps", "persist-trail", true},
	{"rb", "ring-sw", true},
	{"tw", "twin-sw", true},
	{"po", "sect-sw", true},
	{"pb", "patch-on", true},
	{"fc", "counter-on", true},
	{"an", "analysis-on", true},
	{"ky", "keys-on", true},
	{"tx", "tm-on", true},
	{"ry", "rhythm-on", true},
	{"rp", "rhythm-preset", false},
	{"pp", "preset-on", true},
	// The Template legend was the one module switch a link could not carry: it
	// had been persisted nowhere and shared nowhere, so a panel with it open
	// was a panel nobody else could be shown. Found by the test that says the
	// saved layout and the permalink have to describe the same set of modules.
	{"tl", "tpl-on", true},
	{"gr", "gradient-reverse", true},
	{"sk", "skin-visual", false},
	{"fl", "spect-fill", true},
	{"in", "show-info", true},
	{"am", "audio-mod", true},
}

var (
	permaDefaults = map[string]string{}
	lastPermaHash string
	// permaDirty is set by any input or change event and cleared when the hash
	// is next written. See startPermalinkSync.
	permaDirty bool
)

func isColorKey(k string) bool { return k == "cb" || k == "cm" || k == "ct" || k == "cg" }

// formatModRoute renders a paramMod as the permalink wire form
// channel~band0,band1,…~level — the ONE serializer for m./vm. routes.
func formatModRoute(m paramMod) string {
	bs := make([]string, len(m.bands))
	for i, bv := range m.bands {
		bs[i] = permaFmt(bv)
	}
	return m.channel + "~" + strings.Join(bs, ",") + "~" + permaFmt(m.level)
}

// parseModRoute is the inverse; ok is false on a malformed value.
func parseModRoute(val string) (paramMod, bool) {
	sv := strings.Split(val, "~")
	if len(sv) != 3 {
		return paramMod{}, false
	}
	bandStrs := strings.Split(sv[1], ",")
	bands := make([]float32, len(bandStrs))
	for i, bs := range bandStrs {
		f, _ := strconv.ParseFloat(bs, 32) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		bands[i] = float32(f)
	}
	lvl, err := strconv.ParseFloat(sv[2], 32)
	if err != nil {
		return paramMod{}, false
	}
	return paramMod{channel: sv[0], bands: bands, level: float32(lvl)}, true
}

func permaFmt(v float32) string { return strconv.FormatFloat(float64(v), 'g', 6, 32) }

func paramKey(id string) string { return strings.TrimPrefix(id, selectedMode+"-") }

// hashModeToken returns the mode portion of the URL hash (the token before
// the first '&'), or "" if there's no hash.
func hashModeToken() string {
	h := js.Global().Get("location").Get("hash").String()
	if len(h) < 2 {
		return ""
	}
	h = h[1:]
	if i := strings.IndexByte(h, '&'); i >= 0 {
		h = h[:i]
	}
	return h
}

func ctlValue(c permaCtl, el js.Value) string {
	if c.check {
		if el.Get("checked").Bool() {
			return "1"
		}
		return "0"
	}
	v := el.Get("value").String()
	if isColorKey(c.key) {
		v = strings.TrimPrefix(v, "#")
	}
	return v
}

// capturePermaDefaults records the pristine value of each tracked control
// so serialization can omit anything left at its default. Must run before
// applyStateFromHash.
func capturePermaDefaults() {
	permaDefaults = map[string]string{}
	for _, c := range permaCtls {
		el := doc.Call("getElementById", c.id)
		if el.Truthy() {
			permaDefaults[c.key] = ctlValue(c, el)
		}
	}
}

// serializeState builds the hash content (without the leading '#') from the
// current live state.
func serializeState() string {
	var b strings.Builder
	b.WriteString(selectedMode)

	// Registry-owned numeric controls that differ from their defaults.
	for _, ctl := range builtControls {
		if ctl.permaKey == "" || !ctl.slider.Truthy() {
			continue
		}
		v := ctl.slider.Get("value").String()
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			if d := f - float64(ctl.def); d > -1e-9 && d < 1e-9 {
				continue
			}
		}
		b.WriteString("&" + ctl.permaKey + "=" + v)
	}

	// Residual table-driven controls that differ from their captured defaults.
	for _, c := range permaCtls {
		el := doc.Call("getElementById", c.id)
		if !el.Truthy() {
			continue
		}
		v := ctlValue(c, el)
		if v != permaDefaults[c.key] {
			b.WriteString("&")
			b.WriteString(c.key)
			b.WriteString("=")
			b.WriteString(v)
		}
	}

	// Attractor parameters that differ from their default.
	for _, p := range attractorParams[selectedMode] {
		if *p.Value != p.Def {
			b.WriteString("&p.")
			b.WriteString(paramKey(p.ID))
			b.WriteString("=")
			b.WriteString(permaFmt(*p.Value))
		}
	}

	// Custom mode: the equations + their parameters.
	if selectedMode == "custom" {
		serializeCustom(&b)
	}

	// Orientation — the absolute X/Y/Z angles in degrees, only when the
	// model is held still (no spin, no auto-rotate) so the hash isn't
	// churning every frame. Restored into the same angles on load.
	if !autoRotate && cachedRotX == 0 && cachedRotY == 0 && cachedRotZ == 0 {
		if angleX != 0 || angleY != 0 || angleZ != 0 {
			b.WriteString("&rot=")
			b.WriteString(permaFmt(angleX*57.2957795) + "," + permaFmt(angleY*57.2957795) + "," + permaFmt(angleZ*57.2957795))
		}
	}

	// Trackball-drag orientation (independent of the euler pose above, and
	// stable unless the user drags, so it doesn't churn the hash).
	if d := dragQuatString(); d != "" {
		b.WriteString("&drag=")
		b.WriteString(d)
	}

	// Per-parameter audio-mod routing: channel~band0,band1,…~level.
	for _, p := range attractorParams[selectedMode] {
		if m := paramMods[p.ID]; m.channel != "" && m.level != 0 {
			b.WriteString("&m." + paramKey(p.ID) + "=" + formatModRoute(m))
		}
	}

	// View-knob (camera/motion) modulation routing, keyed vm.<suffix>.
	for _, vt := range viewModTargets {
		if m := paramMods[vt.id]; m.channel != "" && m.level != 0 {
			b.WriteString("&vm." + strings.TrimPrefix(vt.id, "view-") + "=" + formatModRoute(m))
		}
	}
	return b.String()
}

// startPermalinkSync refreshes the URL hash from the live state, writing only
// when it actually changed.
//
// It used to serialize on every tick and throw the result away. Serializing is
// not cheap: it reads a value out of the DOM for every registered control, and
// each read crosses the wasm boundary. Timed in the browser it averaged 15ms a
// call at 700ms intervals — roughly one whole frame's budget, spent to
// discover that nothing had moved.
//
// A capture-phase listener on the document sets a flag when anything changes,
// which catches every control including the ones the app drives itself: those
// dispatch real input and change events precisely so that everything watching
// stays in step. The tick then does nothing at all unless the flag is set.
//
// The flag is an optimization and not the source of truth, so a full check
// still runs every permaFullCheck ticks. If some future control ever changes
// state without an event, the hash goes stale for a few seconds rather than
// forever, which is the failure worth having.
const permaFullCheck = 8

func startPermalinkSync() {
	lastPermaHash = serializeState()

	doc.Call("addEventListener", "input", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		permaDirty = true
		return nil
	}), true)
	doc.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		permaDirty = true
		return nil
	}), true)

	ticks := 0
	js.Global().Call("setInterval", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		ticks++
		if permaDirty || ticks%permaFullCheck == 0 {
			permaDirty = false
			syncPermalinkNow()
		}
		return nil
	}), 700)
	// A link pasted into the address bar of a tab that is ALREADY RUNNING used
	// to do nothing at all. The state is read once, at start-up, so changing
	// only the fragment left the URL saying one thing and the screen showing
	// another — and the next sync overwrote the pasted link with the state it
	// had been ignoring. For a feature whose whole point is sharing a view,
	// silently discarding the view someone sent you is the wrong answer.
	//
	// It reloads rather than re-applying in place. applyStateFromHash is
	// written for start-up, where nothing has been set yet; running it over a
	// live session would update the controls it knows about and leave anything
	// else as it was, which is a hybrid of two views and honestly neither.
	// Booting again is what the person asking for that link wanted.
	//
	// No loop is possible: the app writes its own hash with replaceState, which
	// does not fire this event, and the comparison below ignores it anyway.
	js.Global().Call("addEventListener", "hashchange", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		h := strings.TrimPrefix(js.Global().Get("location").Get("hash").String(), "#")
		if h == "" || h == lastPermaHash {
			return nil
		}
		permaFrozen = true
		js.Global().Get("location").Call("reload")
		return nil
	}))
}

// permaFrozen stops the app writing the hash. Set while a reload is on its way,
// because reload() is a request rather than an instant: the page keeps running
// for a few frames, and the 700ms sync firing in that gap would replaceState the
// pasted link away and the reload would then boot with the app's own state --
// which is exactly the bug this was added to fix, one layer deeper.
var permaFrozen bool

// syncPermalinkNow updates the URL hash immediately if the state changed.
func syncPermalinkNow() {
	if permaFrozen {
		return
	}
	s := serializeState()
	if s == lastPermaHash {
		return
	}
	lastPermaHash = s
	writeHash("#" + s)
}

// writeHash rewrites the address bar without letting it take the app down.
//
// replaceState is rate-limited, and being over the limit is a THROW rather than
// a no-op: Safari refuses more than 100 calls in 30 seconds. A throw arriving
// back in Go is a panic, and a panic in wasm ends the program — the same way a
// full localStorage did (see storage_js.go, which is where this shim lives and
// why). Measured here, a brisk drag through the model catalog writes the hash
// about 109 times per 30 seconds, which is over that line while the user is
// only turning a knob.
//
// The Safari limit itself is documented rather than reproduced — there is no
// Safari on the machine this was written on. What IS verified locally is the
// part that matters: when the call throws, the app now keeps running and the
// URL simply stops following for a moment.
func writeHash(h string) {
	if hs := js.Global().Get("__crHash"); hs.Truthy() {
		hs.Call("set", h)
		return
	}
	// No shim (a host page serving this wasm itself): the recover is all there
	// is, and it is enough on the standard runtime.
	// errcheck counts a discarded recover as an unchecked return, in either
	// the blank-assignment or the bare-call form. Discarding it is the point.
	defer func() { _ = recover() }() //nolint:errcheck
	js.Global().Get("history").Call("replaceState", js.Null(), "", h)
}

func permaEvent(name string) js.Value { return js.Global().Get("Event").New(name) }

// eventFor picks the event a restored control reacts to, derived from the
// ELEMENT (selects and checkboxes fire "change"; range/color inputs fire
// "input") — it used to be a hardcoded key list that silently broke restore
// for any new select-backed permalink control that forgot to enroll.
func eventFor(c permaCtl, el js.Value) string {
	if c.check || el.Get("tagName").String() == "SELECT" {
		return "change"
	}
	return "input"
}

func applyControl(key, val string) {
	// Auto-rotate is adopted, not toggled. Its switch and the Y rate are
	// serialized as separate fields but are not independent: ry already
	// contains the auto contribution when the link was captured with the
	// switch on. Running the restore through the toggle would apply that
	// contribution a second time (the rate crept +0.1 per reload) or subtract
	// one that was never there (the rate went negative with the switch off, so
	// the model span while the control said it was stopped, and switching it on
	// canceled back to zero — a control that read backwards).
	if key == "ar" {
		setAutoRotate(val == "1")
		return
	}
	// Registry-owned numeric controls first: set the slider, dispatch input
	// (the descriptor's listener updates cache + LED).
	for _, ctl := range builtControls {
		if ctl.permaKey == key && ctl.slider.Truthy() {
			ctl.slider.Set("value", val)
			ctl.slider.Call("dispatchEvent", permaEvent("input"))
			return
		}
	}
	for _, c := range permaCtls {
		if c.key != key {
			continue
		}
		el := doc.Call("getElementById", c.id)
		if !el.Truthy() {
			return
		}
		if c.check {
			el.Set("checked", val == "1")
		} else if isColorKey(key) {
			el.Set("value", "#"+val)
		} else {
			el.Set("value", val)
		}
		el.Call("dispatchEvent", permaEvent(eventFor(c, el)))
		return
	}
}

func applyParam(suffix, val string) {
	el := doc.Call("getElementById", selectedMode+"-"+suffix)
	if !el.Truthy() {
		return
	}
	el.Set("value", val)
	el.Call("dispatchEvent", permaEvent("input"))
}

func applyRot(val string) {
	f := strings.Split(val, ",")
	if len(f) != 3 {
		return
	}
	var a [3]float32
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(f[i], 32)
		if err != nil {
			return
		}
		a[i] = float32(v) * 0.0174532925 // deg → rad
	}
	angleX, angleY, angleZ = a[0], a[1], a[2]
	rebuildModelMatrix()
	updateModelMatrix()
	updateRotKnobs()
}

// applyStateFromHash parses the URL hash and applies everything after the
// mode token. Params first, then the mod routing map, then controls (with
// audio-mod last so its panel rebuild reflects the params + routing), then
// the held pose.
func applyStateFromHash() {
	applyStateFrom(js.Global().Get("location").Get("hash").String())
}

// applyStateFrom applies a serialized state (leading '#' included) without
// reading the live URL — mid-session restores (the patch bank) must not race
// the permalink sync, which rewrites location.hash on mode changes.
func applyStateFrom(h string) {
	if len(h) < 2 {
		return
	}
	parts := strings.Split(h[1:], "&")
	if len(parts) < 2 {
		return
	}

	var poseVal, amVal, eqVal, dragVal string
	haveCustom, haveFlavor := false, false
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := kv[0], kv[1]
		// fr was the Front SWITCH, which the Fore knob replaced. A link written
		// before that carries fr=1 and no fo, and it meant "all of the model in
		// front of the rack" — which is the knob at +1. Translated rather than
		// ignored, because a permalink is a promise that what you shared is what
		// they see, and silently dropping it would put the model behind the rack
		// for everyone holding an old link.
		if key == "fr" {
			if val == "1" && !strings.Contains(h, "&fo=") {
				key, val = "fo", "1"
			} else {
				continue
			}
		}
		// sk was the Spectro-skin SWITCH, which the skin SELECTOR replaced when
		// the terminal and the desk became skins too. A link written before that
		// carries sk=1, meaning the one skin there was; the selector spells that
		// "spectrogram". Same key, because the meaning is the same — what is
		// painted on the model — and a link should not stop working over a
		// widening.
		if key == "sk" {
			switch val {
			case "1":
				val = "spectrogram"
			case "0":
				val = ""
			}
		}
		switch {
		case key == "rot":
			poseVal = val
		case key == "drag":
			dragVal = val
		case key == "q":
			// Legacy quaternion pose (pre-euler links); ignored.
		case key == "am":
			amVal = val
		case key == "eq":
			eqVal = val
			haveCustom = true
		case key == "cit":
			// The Custom mode's flavor: iterate (a discrete map) rather than
			// flow. It decides what the equations MEAN, so it has to be in
			// place before they are compiled below.
			customIterate = val == "1"
			haveCustom, haveFlavor = true, true
		case key == "cdt":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				customDT = float32(v)
			}
		case strings.HasPrefix(key, "cp."):
			applyCustomParam(strings.TrimPrefix(key, "cp."), val)
			haveCustom = true
		case strings.HasPrefix(key, "p."):
			applyParam(strings.TrimPrefix(key, "p."), val)
		case strings.HasPrefix(key, "vm."):
			if m, ok := parseModRoute(val); ok {
				paramMods["view-"+strings.TrimPrefix(key, "vm.")] = m
			}
		case strings.HasPrefix(key, "m."):
			if m, ok := parseModRoute(val); ok {
				paramMods[selectedMode+"-"+strings.TrimPrefix(key, "m.")] = m
			}
		default:
			if key == "rx" || key == "ry" || key == "rz" {
				// Remember hash-pinned spin rates: Run()'s randomizeOrientation
				// zeroes the rate sliders, and must re-apply these afterward.
				hashPinnedSpin[key[1:]] = val
			}
			applyControl(key, val)
		}
	}
	if amVal != "" {
		applyControl("am", amVal) // triggers setAudioMod → panel rebuild
	}
	if eqVal != "" {
		// A link that carries equations but no flavor is a FLOW link: cit is
		// omitted at its default like every other control, so its absence has
		// to CLEAR the flavor. Leaving whatever the editor was last set to
		// would reinterpret somebody else's derivatives as a map.
		if !haveFlavor {
			customIterate = false
		}
		applyCustomEq(eqVal)
	} else if haveFlavor {
		// A link can pin the flavor without pinning the equations (the editor's
		// default template is the Lorenz one). Recompiling is what moves the
		// system between the flow and map registries, so it still has to happen.
		parseCustom()
	}
	if haveCustom && selectedMode == "custom" {
		buildParamPanel("custom") // reflect restored equations + params
	}
	if poseVal != "" {
		applyRot(poseVal)
	}
	if dragVal != "" {
		if q := strings.Split(dragVal, ","); len(q) == 4 {
			f := [4]float32{}
			for i := 0; i < 4; i++ {
				v, _ := strconv.ParseFloat(q[i], 32) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
				f[i] = float32(v)
			}
			setDragQuat(f[0], f[1], f[2], f[3])
		}
	}
	// Record whether the link pinned an explicit pose, so Run() only applies the
	// "fresh random view each load" when the link DIDN'T — otherwise a shared
	// still-view link is faithfully restored (save→restore fidelity).
	hashPinnedPose = poseVal != "" || dragVal != ""
	syncKnobs() // move fixed-knob pointers to the restored values
}

// hashPinnedSpin holds spin rates (axis → slider value) the loaded permalink
// set via &rx/&ry/&rz, so the load-time orientation randomize can restore
// them after zeroing the rate sliders.
var hashPinnedSpin = map[string]string{}

// hashPinnedPose is true when the loaded permalink specified an explicit
// orientation (&rot= or &drag=), so the startup randomizer should stand down.
var hashPinnedPose bool
