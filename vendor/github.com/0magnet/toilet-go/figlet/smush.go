package figlet

// Horizontal smushing rules, as numbered by the FIGfont specification. A layout
// value is the OR of the rules it enables; rule 0 (universal smushing, where
// the later character simply wins) is the absence of all six.
const (
	RuleEqual        = 0x01 // rule 1: two equal characters smush to themselves
	RuleUnderscore   = 0x02 // rule 2: an underscore yields to a border glyph
	RuleHierarchy    = 0x04 // rule 3: the later class in | /\ [] {} () <> wins
	RuleOppositePair = 0x08 // rule 4: [] {} () facing each other become |
	RuleBigX         = 0x10 // rule 5: /\ \/ >< become | Y X
	RuleHardblank    = 0x20 // rule 6: two hardblanks smush to one
)

// hierarchy is the class list rules 3 and 2 share. Rule 3 pairs it up two by
// two, so | is class 0, / and \ are class 1, and so on.
const hierarchy = "|/\\[]{}()<>"

// Smush returns the character that replaces ch1 and ch2 when they are pushed
// into the same cell, or zero if the enabled rules do not allow it.
//
// Two details are libcaca's rather than the specification's, and are kept
// because the rendered output depends on them. Rules 2 to 6 are only consulted
// when both characters are below U+0080, which makes rule 6 unreachable: it
// tests for U+00A0, the hardblank's internal representation. And rule 1
// explicitly refuses to smush a pair of hardblanks. The effect is that
// hardblanks never smush at all, which is what keeps a hardblank column open.
func Smush(ch1, ch2 rune, rule int) rune {
	// Rule 1: equal characters, hardblanks excepted.
	if rule&RuleEqual != 0 && ch1 == ch2 && ch1 != 0xa0 {
		return ch2
	}

	if ch1 >= 0x80 || ch2 >= 0x80 {
		return 0
	}

	// Rule 2: an underscore is replaced by any of the border characters.
	if rule&RuleUnderscore != 0 {
		if ch1 == '_' && classOf(ch2) >= 0 {
			return ch2
		}
		if ch2 == '_' && classOf(ch1) >= 0 {
			return ch1
		}
	}

	// Rule 3: the character from the later class in the hierarchy wins.
	if rule&RuleHierarchy != 0 {
		i1, i2 := classOf(ch1), classOf(ch2)
		if i1 >= 0 && i2 >= 0 {
			cl1, cl2 := (i1+1)/2, (i2+1)/2
			if cl1 < cl2 {
				return ch2
			}
			if cl1 > cl2 {
				return ch1
			}
		}
	}

	// Rule 4: opposite brackets become a vertical bar.
	if rule&RuleOppositePair != 0 {
		s, p := int(ch1)+int(ch2), int(ch1)*int(ch2)
		if p == '{'*'}' || p == '['*']' || (p == '('*')' && s == '('+')') {
			return '|'
		}
	}

	// Rule 5: the big X.
	if rule&RuleBigX != 0 {
		switch int(ch1)<<8 | int(ch2) {
		case 0x2f5c: // /\
			return '|'
		case 0x5c2f: // \/
			return 'Y'
		case 0x3e3c: // ><
			return 'X'
		}
	}

	// Rule 6: two hardblanks. Unreachable, see the doc comment.
	if rule&RuleHardblank != 0 && ch1 == ch2 && ch1 == 0xa0 {
		return 0xa0
	}

	return 0
}

// classOf returns the byte offset of ch in the hierarchy list, or -1. The list
// is pure ASCII, so a byte scan is the same thing strchr() does.
func classOf(ch rune) int {
	for i := 0; i < len(hierarchy); i++ {
		if rune(hierarchy[i]) == ch {
			return i
		}
	}
	return -1
}
