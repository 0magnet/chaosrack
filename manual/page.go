package manual

import "regexp"

// Page is file name rendered whole for reading, its markers left out.
func Page(name string) string {
	load()
	src := markers.ReplaceAllString(docs[name], "")
	return mdLink.ReplaceAllString(renderPage(src), `href="#$1"`)
}

// mdLink is a link to another of the manual's files, which on the rack's
// manual page is the chapter of that name.
var mdLink = regexp.MustCompile(`href="([A-Za-z0-9_-]+)\.md(?:#[^"]*)?"`)
