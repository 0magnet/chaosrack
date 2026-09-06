//go:build !js && (windows || plan9)

package server

import "github.com/0magnet/desk/panes/hostagent"

// !js like every other file in this package: pkg/server is the SERVER and is
// not reachable from a js/wasm build. Leaving it off made these the only
// js-buildable files here, which pulled the package — and its vendored
// hostagent, which has no js files at all — into the js/wasm lint pass.
//
// printSessionsOnSignal does nothing where SIGUSR1 does not exist. The listing
// is an operator convenience, not part of the feature: --reconnect works the
// same on these platforms, the shells are simply not enumerable from here.
func printSessionsOnSignal(*hostagent.Registry) {}
