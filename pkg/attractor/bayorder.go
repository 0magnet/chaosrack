package attractor

import (
	"slices"
	"strconv"
)

// The order of the rack's bays, as somebody has rearranged it: the screws on
// a bay's ears move it, up or down a place (rackbayorder_js.go). Here is the
// arithmetic, so it can be tested without a page.
//
// A bay is named by its section and which of that section's bays it is,
// "gen#1" for the generators' second, because that is what stays the same
// from one packing to the next: the modules in a bay can change, and its
// number is the very thing being changed.

// bayKeys names the bays whose sections are sections, in order.
func bayKeys(sections []string) []string {
	seen := map[string]int{}
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s + "#" + strconv.Itoa(seen[s])
		seen[s]++
	}
	return out
}

// arrangeBays is the order to show the bays named factory in: the order saved,
// for the bays it names, and any bay it does not name — a bay that is new, or
// was never moved — at its place in the factory order.
func arrangeBays(factory, saved []string) []string {
	var out []string
	for _, k := range saved {
		if slices.Contains(factory, k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	for i, k := range factory {
		if !slices.Contains(out, k) {
			out = slices.Insert(out, min(i, len(out)), k)
		}
	}
	return out
}

// moveBay moves the bay at i one place, up for dir < 0 and down for dir > 0.
// The top bay pushed up goes to the bottom, and the bottom one pushed down
// goes to the top, so every bay can reach every place by one screw.
func moveBay(order []string, i, dir int) []string {
	out := slices.Clone(order)
	if i < 0 || i >= len(out) || dir == 0 {
		return out
	}
	k := out[i]
	out = slices.Delete(out, i, i+1)
	switch {
	case dir < 0 && i == 0:
		return append(out, k)
	case dir > 0 && i == len(order)-1:
		return slices.Insert(out, 0, k)
	case dir < 0:
		return slices.Insert(out, i-1, k)
	default:
		return slices.Insert(out, i+1, k)
	}
}
