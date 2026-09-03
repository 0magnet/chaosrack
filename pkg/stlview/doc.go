// Package stlview is a WebGL viewer for STL (stereolithograph) models,
// compiled to WebAssembly. It renders a rotating, gradient-colored mesh
// on a canvas element with id "gocanvas" and appends X/Y/Z/Zoom slider
// controls (plus a Stop button) to the page footer.
//
// With no model loaded it renders a rotating wireframe sphere instead —
// usable as a lightweight page ornament.
//
// Typical use from a wasm main, after the DOM is ready:
//
//	go func() {
//		data := fetchSomehow()              // e.g. via js fetch
//		raw, _ := stlview.ParseBase64(data) // if base64-wrapped
//		stlview.LoadSTL(raw)
//	}()
//	stlview.Run("model.stl") // blocks; pass "" for the bare sphere
//
// Run must be called from the main goroutine so the render loop keeps
// the wasm instance alive; LoadSTL may be called later from an async
// callback. Set OnStop to hook the Stop button (e.g. to restore
// host-page UI).
//
// Ported from the magnetosphere.net store's product-page STL viewer.
package stlview
