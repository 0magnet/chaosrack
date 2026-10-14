//go:build !unix || js

package racktui

// measuredCellAspect is 0 where the terminal's pixel size cannot be asked
// for: the default guess stands.
func measuredCellAspect() float64 { return 0 }
