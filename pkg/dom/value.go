//go:build js && wasm

package dom

import (
	"math"
	"strconv"
	"syscall/js"
)

// Num is el's value as a number, and 0 where it is not one: an element
// that is missing, a text field somebody emptied, or NaN or an infinity,
// which ParseFloat reads as numbers and nothing that turns a knob can use.
//
// It is here because the same line, ParseFloat of Get("value").String()
// with its error discarded, had been written out some twenty times, each
// with the same comment explaining the discard, and the comment had been
// pasted onto lines it was not true of.
func Num(el js.Value) float64 {
	if !el.Truthy() {
		return 0
	}
	v, err := strconv.ParseFloat(el.Get("value").String(), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// Int is el's value as a whole number, and 0 where it is not one.
func Int(el js.Value) int {
	if !el.Truthy() {
		return 0
	}
	n, err := strconv.Atoi(el.Get("value").String())
	if err != nil {
		return 0
	}
	return n
}
