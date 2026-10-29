//go:build js && wasm

package attractor

// pclock is the page's panel clock, advanced by renderLoop.
var pclock panelClock
