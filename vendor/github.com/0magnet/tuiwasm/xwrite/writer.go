// Package xwrite presents an xterm-go terminal as an io.Writer.
//
// Most of the libraries here never need a tcell.Screen. lipgloss, glamour,
// chroma, go-pretty, pterm, asciigraph and the progress bars all just write
// styled text to an io.Writer, so they need no adapter beyond somewhere for
// that text to go. This is that somewhere, and it is a much shorter road than
// xtcell: no screen, no cells, no key events.
//
// The one thing it has to do is fix line endings. A program writing to a pipe
// ends a line with "\n", but a terminal reads that as "move down one row" and
// nothing else, so the next line starts under the end of the previous one and
// the output walks off to the right. Terminals want "\r\n".
package xwrite

import "bytes"

// crlf rewrites bare newlines as CRLF, leaving existing CRLF alone.
//
// Doubling the carriage return would be harmless on most emulators, but this
// also passes through a "\r" used on its own for progress bars redrawing a
// line in place, which is what most of these libraries do.
func crlf(p []byte) []byte {
	if !bytes.ContainsRune(p, '\n') {
		return p
	}
	out := make([]byte, 0, len(p)+bytes.Count(p, []byte{'\n'}))
	for i := 0; i < len(p); i++ {
		if p[i] == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, p[i])
	}
	return out
}
