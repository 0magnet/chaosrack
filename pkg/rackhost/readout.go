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
		return optionName(c.Value)
	}
	v, err := strconv.ParseFloat(c.Value, 64)
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

// findReadouts finds each control's readout: the text in its box that shows
// the catalog's value. Of several (a dial's scale has a 0 on it, and so does
// its LED at zero), the longest, which is the LED's.
func (r *Rack) findReadouts() {
	for _, c := range r.ctls {
		if c.IsSwitch {
			continue
		}
		box, ok := r.pic.Ctls[c.ID]
		if !ok {
			continue
		}
		best, bestLen := readout{item: -1}, 0
		for i, it := range r.pic.Items {
			if it == nil || it.Text == "" || !inside(it, box) {
				continue
			}
			if c.IsSelect {
				if c.Value != "" && strings.EqualFold(strings.TrimSpace(it.Text), c.Value) && len(it.Text) > bestLen {
					best, bestLen = readout{item: i}, len(it.Text)
				}
				continue
			}
			v, ro, ok := parseReadout(it.Text)
			if !ok {
				continue
			}
			want, err := strconv.ParseFloat(c.Value, 64)
			if err != nil || math.Abs(v-want) > 0.5*math.Pow(10, -float64(ro.dec)) || len(it.Text) <= bestLen {
				continue
			}
			ro.item = i
			best, bestLen = ro, len(it.Text)
		}
		if best.item >= 0 {
			r.reads[c.ID] = best
		}
	}
}
