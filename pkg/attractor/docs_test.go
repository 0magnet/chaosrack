package attractor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/manual"
)

// Every key the panel asks the manual for, written into the source as a
// literal — doc("…"), docf("…", …) or data-doc="…" in the markup — has an
// entry: one missing is a control with no tooltip and no page in the manual,
// which nothing else notices.
func TestEveryDocKeyIsInTheManual(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	key := regexp.MustCompile(`\bdocf?\("([^"]+)"[,)]|data-doc="([^"]+)"`)
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "docs.go" || f == "docs_js.go" {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // this package's own sources, globbed above
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range key.FindAllStringSubmatch(string(b), -1) {
			k := m[1] + m[2]
			seen++
			if !manual.Has(k) {
				t.Errorf("%s: %q is not in the manual", f, k)
			}
		}
	}
	if seen < 500 {
		t.Errorf("found only %d keys: the pattern has stopped matching", seen)
	}
}
