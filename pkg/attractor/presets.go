package attractor

// Named presets: the whole live view, saved under a name.
//
// The app has always been able to write its entire state down — that is what
// the address bar carries, and permalink_js.go's serializeState is the one
// thing that produces it. The Patchbay's eight-slot bank already stores those
// strings; what it cannot do is tell you what is in slot 5. A numbered slot is
// a patch memory on a 1978 synthesizer, and it is the right thing for a rack,
// but it is not how anyone finds the view they built last week.
//
// So a preset is exactly what a patch slot is — a serializeState() string —
// with a name on it. No second serializer, no second restore path: recalling a
// preset runs the same recallSerializedState the bank does, which is
// applyStateFrom, which is the entry point that deliberately does not read
// location.hash so a mid-session restore cannot race the permalink sync.
//
// Untagged so the store can be tested on the host. The interesting behavior —
// saving over a name, a name with a separator in it, the cap — is all
// list arithmetic, and none of it needs a browser.

import "strings"

// presetStoreKey is where the presets live, beside the patch bank's
// wasmstuff-patchbank.
const presetStoreKey = "wasmstuff-presets"

// The record separators are the ASCII ones meant for exactly this: 0x1e
// between records, 0x1f between fields. The patch bank already stores its
// slots \x1f-joined, so this is the same convention one level deeper. Neither
// can appear in a name (cleanPresetName strips them) nor in a serialized
// state, which is URL-hash text.
const (
	presetRecSep   = "\x1e"
	presetFieldSep = "\x1f"
)

// presetNameMax matches the name field's maxlength. Enforced here as well
// because the field is not the only way in: a record hand-edited in devtools,
// or written by a future build, must not produce a name that overruns the
// module.
const presetNameMax = 24

// presetsMax caps the store. localStorage is a few megabytes for the whole
// origin and a serialized state runs to a few hundred bytes, so the cap is
// about the module's <select> staying usable, not about space.
const presetsMax = 32

// preset is one saved view.
type preset struct {
	Name  string
	State string // a serializeState() string, without the leading '#'
}

// cleanPresetName is what a name is allowed to be: no control characters (the
// record separators among them), trimmed, and short enough to read on a
// module front panel.
func cleanPresetName(s string) string {
	var b strings.Builder
	for _, r := range s {
		// Control characters, not just the two separators: a name carrying a
		// newline or a tab reads as two names in any list that prints it, and
		// there is no reason to want one.
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	name := strings.TrimSpace(b.String())
	if r := []rune(name); len(r) > presetNameMax {
		name = strings.TrimSpace(string(r[:presetNameMax]))
	}
	return name
}

// encodePresets renders the store for localStorage.
func encodePresets(ps []preset) string {
	recs := make([]string, 0, len(ps))
	for _, p := range ps {
		name := cleanPresetName(p.Name)
		if name == "" || p.State == "" {
			continue
		}
		recs = append(recs, name+presetFieldSep+p.State)
	}
	return strings.Join(recs, presetRecSep)
}

// decodePresets parses the store. A malformed record is skipped rather than
// failing the lot: one bad entry must not cost somebody every preset they
// have.
func decodePresets(s string) []preset {
	if s == "" {
		return nil
	}
	var out []preset
	for _, rec := range strings.Split(s, presetRecSep) {
		kv := strings.SplitN(rec, presetFieldSep, 2)
		if len(kv) != 2 {
			continue
		}
		name := cleanPresetName(kv[0])
		if name == "" || kv[1] == "" {
			continue
		}
		out = append(out, preset{Name: name, State: kv[1]})
	}
	return out
}

// findPreset looks a preset up by name, case-insensitively.
func findPreset(ps []preset, name string) (preset, bool) {
	if i := presetIndex(ps, name); i >= 0 {
		return ps[i], true
	}
	return preset{}, false
}

// presetIndex is where a name sits, or -1.
//
// Case-insensitive, because the name is typed twice — once to save and once to
// save over — and "Lorenz Blue" and "lorenz blue" being two different presets
// that look identical in the list is a way to lose work rather than a feature.
func presetIndex(ps []preset, name string) int {
	name = cleanPresetName(name)
	if name == "" {
		return -1
	}
	for i, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return i
		}
	}
	return -1
}

// putPreset saves a state under a name, replacing any preset already called
// that IN PLACE — re-saving must not move a preset to the end of the list, or
// the list reshuffles itself every time somebody tunes a view they already had.
//
// An empty name or an empty state is refused: there is nothing to file it
// under, and a nameless preset is one nobody can ever recall.
func putPreset(ps []preset, name, state string) []preset {
	name = cleanPresetName(name)
	if name == "" || state == "" {
		return ps
	}
	if i := presetIndex(ps, name); i >= 0 {
		out := make([]preset, len(ps))
		copy(out, ps)
		out[i] = preset{Name: name, State: state}
		return out
	}
	out := make([]preset, 0, len(ps)+1)
	out = append(out, ps...)
	out = append(out, preset{Name: name, State: state})
	// At the cap the OLDEST goes. Refusing the save instead would lose the
	// thing the user just made in favor of something they made thirty presets
	// ago, and a Save button that silently does nothing reads as broken.
	if len(out) > presetsMax {
		out = out[len(out)-presetsMax:]
	}
	return out
}

// deletePreset removes a preset by name, and is a no-op if there is none.
func deletePreset(ps []preset, name string) []preset {
	i := presetIndex(ps, name)
	if i < 0 {
		return ps
	}
	out := make([]preset, 0, len(ps)-1)
	out = append(out, ps[:i]...)
	out = append(out, ps[i+1:]...)
	return out
}
