package manual

import (
	"regexp"
	"strings"
)

// Page is file name rendered whole for reading, its markers left out.
func Page(name string) string {
	load()
	src := markers.ReplaceAllString(docs[name], "")
	return mdLink.ReplaceAllString(renderPage(src), `href="#$1"`)
}

// mdLink is a link to another of the manual's files, which on the rack's
// manual page is the chapter of that name.
var mdLink = regexp.MustCompile(`href="([A-Za-z0-9_-]+)\.md(?:#[^"]*)?"`)

// Chapter is file name as text to read in a terminal: its markdown, which
// reads as a manual on its own, with the entries' key markers left out.
func Chapter(name string) (string, bool) {
	load()
	src, ok := docs[name]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(blanks.ReplaceAllString(markers.ReplaceAllString(src, ""), "\n\n")) + "\n", true
}

// ChapterTitle is file name's first heading, or its name.
func ChapterTitle(name string) string {
	load()
	for _, l := range strings.Split(docs[name], "\n") {
		if m := heading.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return name
}
