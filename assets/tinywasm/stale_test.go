package tinywasm

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The TinyGo build here is served: the page offers a "tinygo" runtime
// and hands over the bytes embedded here. Nothing rebuilds it as a side effect
// of ordinary work, so it goes stale silently — it did once by a week, was
// refreshed by a commit that wrote the build command into the Makefile, and
// went stale again by fifteen commits within two days. Writing the command down
// does not make anyone run it.
//
// So `make tinywasm` records what the sources hashed to at build time, and this
// compares that against what they hash to now. A stale artifact is now a failing
// test rather than a runtime somebody is quietly served old code from.
func TestEmbeddedBuildIsNotStale(t *testing.T) {
	// The build is not in the repository: CI publishes it (pkg/wasmfetch), and
	// a checkout holds one only where make has put one. With none here there
	// is nothing to be stale, and the binary serves the published build. With
	// one here it is what the binary serves, so it has to be current.
	if _, err := os.Stat("chaosrack-tiny.wasm"); err != nil {
		t.Skipf("no build here; the published one is served: %v", err)
	}
	recorded, err := os.ReadFile("built-from.txt")
	if err != nil {
		t.Fatalf("a build with no record of what it was built from: %v", err)
	}

	// The package path is relative to cmd.Dir, not to this test's directory.
	// Getting that wrong made the command fail, which took the skip below, and
	// a skipped test reads as "ok" — so this test passed while checking
	// nothing at all, which is precisely the failure it exists to catch.
	cmd := exec.Command("go", "run", "./cmd/wasmstamp")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		// Without a toolchain there is nothing to compare against, and failing
		// here would only punish an environment that cannot check. Loudly,
		// though: a silent skip is how this test lied the first time.
		t.Skipf("cannot fingerprint the sources here, so staleness is UNCHECKED: %v", err)
	}

	now, was := strings.TrimSpace(string(out)), strings.TrimSpace(string(recorded))
	if now != was {
		t.Errorf("the embedded TinyGo build is stale: sources now hash to %s, "+
			"the artifact was built from %s.\nRun `make tinywasm` (or `make wasms` for both), or delete the build here to serve the published one.",
			now, was)
	}
}
