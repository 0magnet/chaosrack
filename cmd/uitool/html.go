// Subcommand html: is the markup the panel actually builds well formed?
//
// The panel's HTML is a Go string constant plus several thousand lines of DOM
// built at runtime, and nothing ever parses either. A browser renders almost
// anything — an unclosed span, a duplicate id, a label pointing at a control
// that is not there — so a mistake in the markup fails no build and no test.
// It shows up as a control that is slightly the wrong shape, which is
// indistinguishable from a CSS fault and gets debugged as one. Both of those
// happened in a single session: an edit re-encoded the source and turned every
// reset glyph into mojibake, and a readout ended up named for its neighbor.
//
// So this checks the RENDERED DOM — after every runtime builder has run, which
// is what actually reaches a person, and which no amount of reading the Go
// string will tell you:
//
//   - it parses
//
//   - no id appears twice, because getElementById then picks one and every
//     control sharing an id is one the permalink, the MIDI map and Reset All
//     can address only half of
//
//   - every label's "for" names something that exists
//
//   - no text is mojibake: a replacement character, or the sequences a
//     double-encoded UTF-8 file always produces
//
//     uitool html                  # check the rendered DOM
//     uitool html -save out.html   # and keep a copy, e.g. to feed to vnu
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/0magnet/chaosrack/internal/cdp"
	"golang.org/x/net/html"
)

var htmlSave = flag.String("save", "", "also write the rendered DOM to this file")

// mojibakeMarks are the sequences that mean a UTF-8 string has been decoded
// as Latin-1 and re-encoded. They are ordinary characters in isolation, so
// this is a heuristic — but none of them belongs in this panel's text.
var mojibakeMarks = []string{"�", "Â", "Ã¢", "Ã°"}

func runHTML() {
	c, err := cdp.Dial(*cdpPort, *target)
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	src, ok := c.Eval("document.documentElement.outerHTML").(string)
	if !ok || src == "" {
		fmt.Println("could not read the rendered DOM")
		os.Exit(1)
	}
	if *htmlSave != "" {
		if err := os.WriteFile(*htmlSave, []byte(src), 0o600); err != nil {
			fmt.Println("save:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", *htmlSave, len(src), "bytes")
	}

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		fmt.Println("PARSE FAILED:", err)
		os.Exit(1)
	}

	ids := map[string]int{}
	labelFor := map[string]bool{}
	mojibake := map[string]int{}
	elements := 0

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			elements++
			for _, a := range n.Attr {
				switch a.Key {
				case "id":
					ids[a.Val]++
				case "for":
					labelFor[a.Val] = true
				}
			}
		case html.TextNode:
			for _, m := range mojibakeMarks {
				if strings.Contains(n.Data, m) {
					mojibake[m]++
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)

	var faults []string

	var dups []string
	for id, n := range ids {
		if n > 1 {
			dups = append(dups, fmt.Sprintf("%s (x%d)", id, n))
		}
	}
	sort.Strings(dups)
	if len(dups) > 0 {
		faults = append(faults, fmt.Sprintf("%d duplicate ids:\n    %s",
			len(dups), strings.Join(dups, "\n    ")))
	}

	var dangling []string
	for t := range labelFor {
		if ids[t] == 0 {
			dangling = append(dangling, t)
		}
	}
	sort.Strings(dangling)
	if len(dangling) > 0 {
		faults = append(faults, fmt.Sprintf("%d label[for] naming nothing:\n    %s",
			len(dangling), strings.Join(dangling, "\n    ")))
	}

	if len(mojibake) > 0 {
		var m []string
		for k, n := range mojibake {
			m = append(m, fmt.Sprintf("%q x%d", k, n))
		}
		sort.Strings(m)
		faults = append(faults, "mojibake in text: "+strings.Join(m, ", ")+
			"\n    (a double-encoded UTF-8 source file reads exactly like this)")
	}

	fmt.Printf("rendered DOM: %d elements, %d ids\n", elements, len(ids))
	if len(faults) == 0 {
		fmt.Println("ok - parses, no duplicate ids, no dangling labels, no mojibake")
		return
	}
	for _, f := range faults {
		fmt.Println("  FAIL", f)
	}
	os.Exit(1)
}
