package attractor

// A stored snapshot, and when one can be honored.
//
// A snapshot is a serializeState() string — the same text the address bar
// carries. The patch bank's eight slots hold them, the Presets module holds
// named ones, and both recall through the single path in patchbay_js.go.
//
// Untagged, because the question that matters here is not a DOM question. It is
// whether a string stored some time ago still describes something this build
// can produce, and getting that wrong costs the user the view they were looking
// at — see recallRefusedMode.

import "strings"

// hashModeOf extracts the mode token from a serialized snapshot: everything
// before the first '&', which is where serializeState puts the model.
func hashModeOf(s string) string {
	if i := strings.IndexByte(s, '&'); i >= 0 {
		return s[:i]
	}
	return s
}

// recallRefusedMode reports the model a snapshot names when this build has no
// such model — the case where recalling it must change nothing at all.
//
// Recall resets every control to its default before re-applying the snapshot
// over the top. That is right when the snapshot can be applied and destructive
// when it cannot: the reset lands, the restore does not, and the user is left
// with neither the patch they asked for nor the view they had. Measured before
// this existed, the URL went from "#thomas&pb=1" to "#thomas" — the Patchbay
// module switching itself off, in the middle of a click on the Patchbay.
//
// A model this build does not have is how a snapshot goes bad: one stored by an
// older build, or by a build where the model was called something else. Nothing
// else in the snapshot can be trusted to still mean the same thing either, so
// the answer is to refuse the whole thing rather than apply half of it.
//
// An EMPTY mode token is not a refusal. It means the snapshot names no model,
// so there is nothing to fail to find, and the controls it carries still apply
// to whatever is on screen.
func recallRefusedMode(snapshot string, known func(string) bool) (string, bool) {
	if snapshot == "" {
		return "", false
	}
	mode := hashModeOf(snapshot)
	if mode == "" || known(mode) {
		return "", false
	}
	return mode, true
}
