//go:build js && wasm

package attractor

// Rabinovich–Fabrikant — render loop only. The vector field, its timestep and
// its initial condition live untagged in flowdata.go; this file is the half
// that needs a browser.
//
// The system is stiff and its chaotic attractor has a small basin, so it uses
// the shared double-precision RK4 loop with state kept across frames — in
// single precision the trajectory escapes and renders blank.

func generateRabinovich() { integrate3D(float64(rabDT), rabDeriv) }
