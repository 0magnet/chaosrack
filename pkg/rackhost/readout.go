package rackhost

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// readout is a control's display in the picture — the LED a dial's value is
// written on, the window a selector's choice is named in — and how the page
// wrote the catalog's value there, so a new value is written the same way.
type readout struct {
	item int
	// For a number, how the page wrote it: the spaces it is indented by, a
	// sign column (a space, a sign, or none), whether a positive value is
	// written with its +, the integer digits zeros pad it to (0 for none),
	// the decimals, and what follows the digits (a degree sign).
	lead   int
	column bool
	plus   bool
	width  int
	dec    int
	suffix string
}

// show is what the readout says for c's value now.
func (ro readout) show(c *racktui.Control) string {
	if c.IsSelect {
		return labelOf(c, c.Value)
	}
	v, err := strconv.ParseFloat(shown(c.ID, c.Value), 64)
	if err != nil {
		return c.Value
	}
	s := strconv.FormatFloat(math.Abs(v), 'f', ro.dec, 64)
	intLen := len(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intLen = i
	}
	if pad := ro.width - intLen; pad > 0 {
		s = strings.Repeat("0", pad) + s
	}
	neg := v < 0 && math.Abs(v) >= 0.5*math.Pow(10, -float64(ro.dec))
	switch {
	case neg:
		s = "-" + s
	case ro.plus:
		s = "+" + s
	case ro.column:
		s = " " + s
	}
	return strings.Repeat(" ", ro.lead) + s + ro.suffix
}

// optionName is how a selector's window names an option: the page shows a
// model by its name, which for most is the key with a capital; "" is off.
func optionName(v string) string {
	if v == "" {
		return "off"
	}
	r := []rune(v)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// parseReadout reads a number the way a panel LED writes it — " 000.0",
// "-23", "130°", "0.60" — and reports the format with its value.
func parseReadout(t string) (float64, readout, bool) {
	var ro readout
	body := strings.TrimRight(t, " ")
	trimmed := strings.TrimLeft(body, " ")
	spaces := len(body) - len(trimmed)
	switch {
	case strings.HasPrefix(trimmed, "+"):
		ro.lead, ro.column, ro.plus = spaces, true, true
		trimmed = trimmed[1:]
	case strings.HasPrefix(trimmed, "-"):
		ro.lead, ro.column = spaces, true
	case spaces > 0:
		// A space before the digits is where a minus would go.
		ro.lead, ro.column = spaces-1, true
	}
	body = trimmed
	end := len(body)
	for end > 0 && !unicode.IsDigit(rune(body[end-1])) {
		end--
	}
	ro.suffix = body[end:]
	num := body[:end]
	if num == "" || strings.ContainsAny(ro.suffix, "0123456789") {
		return 0, ro, false
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, ro, false
	}
	digits := strings.TrimPrefix(num, "-")
	intDigits := digits
	if i := strings.IndexByte(digits, '.'); i >= 0 {
		intDigits, ro.dec = digits[:i], len(digits)-i-1
	}
	// Padded to a fixed number of digits only where the page pads: a
	// leading zero says so ("000.0", "036.0"), and so do degrees ("130°"
	// beside "002°"); "-23" is just a number.
	if (len(intDigits) > 1 && intDigits[0] == '0') || ro.suffix == "°" {
		ro.width = len(intDigits)
	}
	return v, ro, true
}

// findReadout is a control's readout: of the text among items lo to hi in
// its box, what shows value. Of several (a dial's scale has a 0 on it, and
// so does its LED at zero), the longest, which is the LED's.
func (r *Rack) findReadout(c *racktui.Control, value string, box [4]float64, lo, hi int) (readout, bool) {
	if c.IsSwitch {
		return readout{}, false
	}
	want, numErr := strconv.ParseFloat(shown(c.ID, value), 64)
	best, bestLen := readout{item: -1}, 0
	for i := lo; i < hi; i++ {
		it := r.pic.Items[i]
		if it == nil || it.Text == "" || !inside(it, box) || len(it.Text) <= bestLen {
			continue
		}
		if c.IsSelect {
			if t := strings.TrimSpace(it.Text); value != "" && (strings.EqualFold(t, value) || strings.EqualFold(t, labelOf(c, value))) {
				best, bestLen = readout{item: i}, len(it.Text)
			}
			continue
		}
		v, ro, ok := parseReadout(it.Text)
		if !ok || numErr != nil || math.Abs(v-want) > 0.5*math.Pow(10, -float64(ro.dec)) {
			continue
		}
		ro.item = i
		best, bestLen = ro, len(it.Text)
	}
	return best, best.item >= 0
}

// shown is the number a control's display writes for value, where that is
// not the value itself: a generator's frequency knob counts semitones and
// its LED says hertz.
func shown(id, value string) string {
	if _, part := genOf(id); part == "freq" {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return strconv.FormatFloat(freqFromKnob(v), 'f', -1, 64)
		}
	}
	return value
}

// labelOf is how a selector's window names option v: the page's own name for
// it where the catalog has one ("2-color" for "2"), else optionName.
func labelOf(c *racktui.Control, v string) string {
	for i, o := range c.Options {
		if o == v && i < len(c.Labels) && c.Labels[i] != "" {
			return c.Labels[i]
		}
	}
	return optionName(v)
}
