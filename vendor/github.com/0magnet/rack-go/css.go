//go:build js && wasm

package rack

import (
	_ "embed"
)

//go:embed assets/rack.css
var styleSheet string

const styleID = "rack-style"

// CSS returns the embedded stylesheet. It is structure only — flex layout, the
// module box, the header strip — so a host styles its own panel without having
// to undo anything first.
func CSS() string { return styleSheet }

// InjectCSS appends the embedded stylesheet to document.head once. New calls
// it; call it yourself only to get the styles in before the first rack exists,
// or skip it entirely by putting your own stylesheet in an element with the id
// "rack-style".
func InjectCSS() {
	if document.Call("getElementById", styleID).Truthy() {
		return
	}
	style := document.Call("createElement", "style")
	style.Set("id", styleID)
	style.Set("textContent", styleSheet)
	document.Get("head").Call("appendChild", style)
}
