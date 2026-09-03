// The package doc lives in desk.go, which is js/wasm-only. This file carries no
// build tag so that the embedded assets — and therefore the serve command — are
// available to a native binary, which is the whole point of having them.
package desk

import (
	"embed"
	"io/fs"
)

// Assets is the built demo — the page, the wasm and its loader shim.
//
// Embedding it means the serve command is a single binary with nothing to host
// and nothing to fetch: `desk serve` works on a machine with no network and no
// checkout. The cost is that the binary carries the wasm, and that the build
// has to have been run before this package compiles — which is why docs/ holds
// a committed build rather than being a build artifact.
//
//go:embed docs
var assets embed.FS

// Assets returns the demo's web root, ready to hand to http.FileServerFS.
func Assets() fs.FS {
	sub, err := fs.Sub(assets, "docs")
	if err != nil {
		// docs is embedded above, so this cannot fail at runtime; if it does
		// the package is broken rather than the caller.
		panic("desk: embedded assets missing: " + err.Error())
	}
	return sub
}
