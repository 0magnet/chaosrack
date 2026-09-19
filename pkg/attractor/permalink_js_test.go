//go:build js && wasm

package attractor

import (
	"strings"
	"testing"
)

// Every control a permalink carries has to exist, and the table has to
// agree with the markup about what KIND it is: a select read as a checkbox
// restores as "off" and takes the setting out of every shared link
// silently. Two entries here changed kind in one commit — Views became a
// grid dial and A/B became a focus dial — which is exactly when this goes
// wrong unnoticed.
func TestEveryPermalinkControlExistsAndMatchesItsKind(t *testing.T) {
	for _, p := range permaCtls {
		i := strings.Index(controlsBody, `id="`+p.id+`"`)
		if i < 0 {
			t.Errorf("permalink %q names %s, which is not in the panel markup",
				p.key, p.id)
			continue
		}
		// Walk back to the tag this id sits on.
		start := strings.LastIndex(controlsBody[:i], "<")
		if start < 0 {
			continue
		}
		tag := controlsBody[start:i]
		isCheck := strings.Contains(tag, "<input") && strings.Contains(tag, `type="checkbox"`)
		isSelect := strings.HasPrefix(tag, "<select")
		isColor := strings.Contains(tag, "<input") && strings.Contains(tag, `type="color"`)
		switch {
		case p.check && !isCheck:
			t.Errorf("permalink %q reads %s as a checkbox, but the markup has %q",
				p.key, p.id, strings.TrimSpace(tag))
		case !p.check && isCheck:
			t.Errorf("permalink %q reads %s as a value, but the markup has a checkbox",
				p.key, p.id)
		case !p.check && !isSelect && !isColor:
			t.Errorf("permalink %q reads %s as a select or color, but the markup has %q",
				p.key, p.id, strings.TrimSpace(tag))
		}
	}
	// And no two entries share a key, which would make one of them
	// unreachable in a link.
	seen := map[string]string{}
	for _, p := range permaCtls {
		if prev, dup := seen[p.key]; dup {
			t.Errorf("key %q is used by both %s and %s", p.key, prev, p.id)
		}
		seen[p.key] = p.id
	}
}
