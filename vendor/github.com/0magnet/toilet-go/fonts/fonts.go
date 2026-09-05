// Package fonts carries the TOIlet font collection, so that toilet-go renders
// something without a font directory installed.
//
// These are the twenty-four fonts TOIlet itself installs, copied unmodified
// from its source tree. They are separate works from the program and are
// redistributed here under their own license: each one is by Sam Hocevar and is
// covered by the WTFPL, which the hand-written fonts state in their own comment
// block and which the generated ones inherit from TOIlet's COPYING and from its
// version banner, "TOIlet, along with the various TOIlet fonts and
// documentation, may be freely copied and distributed". See LICENSE in this
// directory.
//
// Nothing else is bundled. Fonts from the figlet collection have their own
// licenses, several of which are not redistribution-friendly, so they are left
// to be loaded from a font directory at run time with -d.
package fonts

import (
	"embed"
	"sort"
	"strings"
)

//go:embed *.tlf
var files embed.FS

// Get returns the contents of an embedded font, still in whatever container it
// ships in. The name is tried as given and then with a .tlf suffix, matching
// the way a font directory is searched.
func Get(name string) ([]byte, bool) {
	for _, n := range []string{name, name + ".tlf"} {
		if strings.ContainsAny(n, "/\\") {
			continue
		}
		if data, err := files.ReadFile(n); err == nil {
			return data, true
		}
	}
	return nil, false
}

// List returns the names of the embedded fonts, without their extension.
func List() []string {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".tlf"))
	}
	sort.Strings(names)

	return names
}
