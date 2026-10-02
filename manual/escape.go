package manual

import "strings"

// Escape is plain text as markdown that PlainText turns back into the same
// text: what moving a tooltip written in Go into the manual needs, so the
// tooltip does not change on the way.
//
// Paragraphs stay paragraphs and a line break inside one stays a line break.
// A paragraph of equations — every line of it with an = in it — is written
// as a code block, which is how the manual should show one anyway.
func Escape(s string) string {
	paras := strings.Split(strings.TrimSpace(s), "\n\n")
	for i, p := range paras {
		lines := strings.Split(strings.Trim(p, "\n"), "\n")
		if equations(lines) {
			paras[i] = "```text\n" + strings.Join(lines, "\n") + "\n```"
			continue
		}
		for j, l := range lines {
			lines[j] = escapeLine(strings.TrimSpace(l))
		}
		paras[i] = strings.Join(lines, "\\\n")
	}
	return strings.Join(paras, "\n\n")
}

// equations reports whether lines are two or more lines of equations.
func equations(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	for _, l := range lines {
		if !strings.Contains(l, "=") || strings.Contains(l, "```") {
			return false
		}
	}
	return true
}

// escapeLine is one line of text with everything markdown would read as
// markup escaped.
func escapeLine(l string) string {
	var b strings.Builder
	for _, r := range l {
		if strings.ContainsRune("\\`*_[]<>&", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	s := b.String()
	// What only means something at the start of a line: a heading, a quote,
	// a list item, a numbered one, a rule or a setext underline.
	switch {
	case strings.HasPrefix(s, "#"), strings.HasPrefix(s, "+"), strings.HasPrefix(s, "-"),
		strings.HasPrefix(s, "="), strings.HasPrefix(s, "~"), strings.HasPrefix(s, "|"):
		s = "\\" + s
	default:
		n := 0
		for n < len(s) && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n > 0 && n < len(s) && (s[n] == '.' || s[n] == ')') {
			s = s[:n] + "\\" + s[n:]
		}
	}
	return s
}
