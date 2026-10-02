package manual

import "strings"

// plainFast is md as PlainText gives it, without the markdown parser, when md
// is plain: paragraphs, line breaks, backslash escapes and code blocks, and
// none of what makes markup — emphasis, code spans, links, raw HTML, entities,
// lists, quotes or headings. Most entries are exactly that: the text they were
// written as, escaped (Escape). Rendering every tooltip through the parser
// was most of what the manual cost the rack's start. ok is false for anything
// else, which the parser then has.
func plainFast(md string) (s string, ok bool) {
	var out []string // paragraphs
	var para strings.Builder
	inCode := false
	var code strings.Builder
	flush := func() {
		if para.Len() > 0 {
			out = append(out, para.String())
			para.Reset()
		}
	}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if inCode {
				out = append(out, strings.TrimSuffix(code.String(), "\n"))
				code.Reset()
			} else {
				flush()
			}
			inCode = !inCode
			continue
		}
		if inCode {
			code.WriteString(line + "\n")
			continue
		}
		if t == "" {
			flush()
			continue
		}
		if line != t && strings.HasPrefix(line, "    ") || !plainLineStart(t) {
			return "", false
		}
		text, hard, ok := unescapeLine(t)
		if !ok {
			return "", false
		}
		// A line break, soft or hard, is a line break in the text, as the
		// parser leaves it.
		if para.Len() > 0 && !strings.HasSuffix(para.String(), "\n") {
			para.WriteString("\n")
		}
		para.WriteString(text)
		if hard {
			para.WriteString("\n")
		}
	}
	if inCode {
		return "", false
	}
	flush()
	for i, p := range out {
		out[i] = strings.TrimRight(p, "\n")
	}
	return strings.TrimSpace(strings.Join(out, "\n\n")), true
}

// plainLineStart reports whether a line starts the way a line of a plain
// paragraph does, not as a heading, quote, list item, rule or table.
func plainLineStart(t string) bool {
	switch t[0] {
	case '#', '>', '-', '+', '*', '=', '|', '~', '`':
		return false
	}
	n := 0
	for n < len(t) && t[n] >= '0' && t[n] <= '9' {
		n++
	}
	return !(n > 0 && n < len(t) && (t[n] == '.' || t[n] == ')'))
}

// unescapeLine is one line of a plain paragraph as text: each backslash
// escape its character, and whether it ends in a hard break (a trailing
// backslash). ok is false for anything that would be markup.
func unescapeLine(t string) (text string, hard, ok bool) {
	var b strings.Builder
	rs := []rune(t)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i == len(rs)-1:
			return b.String(), true, true
		case r == '\\' && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", rs[i+1]):
			b.WriteRune(rs[i+1])
			i++
		case strings.ContainsRune("*_`[]<&", r):
			return "", false, false
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), false, true
}
