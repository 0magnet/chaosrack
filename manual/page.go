package manual

import (
	"bytes"
	"html"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// pages renders a whole file for reading: tables and heading anchors, which
// an entry never needs and the front of the manual does. Entries keep the
// plain renderer, so a tooltip cannot change because a page did.
var pages = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// Page is file name rendered whole for reading, its markers left out.
func Page(name string) string {
	load()
	var b bytes.Buffer
	src := markers.ReplaceAllString(docs[name], "")
	if err := pages.Convert([]byte(src), &b); err != nil {
		return "<pre>" + html.EscapeString(src) + "</pre>"
	}
	return mdLink.ReplaceAllString(b.String(), `href="#$1"`)
}

// mdLink is a link to another of the manual's files, which on the rack's
// manual page is the chapter of that name.
var mdLink = regexp.MustCompile(`href="([A-Za-z0-9_-]+)\.md(?:#[^"]*)?"`)
