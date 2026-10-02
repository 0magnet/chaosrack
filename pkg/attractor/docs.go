package attractor

// The panel's help text is the manual's (the manual directory at the
// repository root, one markdown file to a part of the rack). The markup names
// the entry each element's tooltip is with data-doc="key", and Go asks for an
// entry by its key with doc; the Info window renders the same entries.

import "github.com/0magnet/chaosrack/manual"

// doc is the manual's entry for key as a tooltip: plain text.
func doc(key string) string { return manual.Text(key) }
