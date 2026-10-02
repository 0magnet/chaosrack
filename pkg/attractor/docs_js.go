//go:build js && wasm

package attractor

import (
	"html"
	"regexp"
	"strings"

	"github.com/0magnet/chaosrack/manual"
)

// docAttr is the attribute naming an element's entry.
var docAttr = regexp.MustCompile(`data-doc="([^"]+)"`)

// withDocs is markup with each data-doc's entry written in as the element's
// title, so the page is built with its tooltips in place, before anything
// that reads a title from the markup (a selector knob names itself from its
// select's) runs.
func withDocs(body string) string {
	return docAttr.ReplaceAllStringFunc(body, func(m string) string {
		t := manual.Text(docAttr.FindStringSubmatch(m)[1])
		if t == "" {
			return m
		}
		return m + ` title="` + html.EscapeString(t) + `"`
	})
}

// docf is doc with each {name} in the entry replaced by its value, given as
// name, value pairs: for an entry about whichever model or family it is shown
// beside.
func docf(key string, kv ...string) string {
	t := doc(key)
	for i := 0; i+1 < len(kv); i += 2 {
		t = strings.ReplaceAll(t, "{"+kv[i]+"}", kv[i+1])
	}
	return t
}
