package gowasm

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The Go build is committed and served: the page offers a "tinygo" runtime
// and hands over the bytes embedded here. Nothing rebuilds it as a side effect
// of ordinary work, so it goes stale silently — the TinyGo one
// beside it did exactly that, twice, and this one was found half a day stale in
// the same sweep — differing from a fresh build because two commits had changed
// sources it is made from. Writing the command down
// does not make anyone run it.
//
// So `make wasm` records what the sources hashed to at build time, and this
// compares that against what they hash to now. A stale artifact is now a failing
// test rather than a runtime somebody is quietly served old code from.
func TestEmbeddedBuildIsNotStale(t *testing.T) {
	// CI rebuilds both wasm artifacts and commits them, so on a push there is
	// nothing here worth reporting: this is stale for the few minutes between
	// the source commit and the refresh commit that follows, and failing for
	// that window would paint every push red. The variable is set only for a
	// push — on a pull request no refresh can happen, so the test runs and
	// says so. Locally it always runs, which is where the forgetting this
	// exists to catch actually happens.
	//
	// This one was left ungated when the TinyGo test was gated, and the whole
	// test job then failed on the first dependency bump: the fingerprint
	// covers go.mod, so a bump stales BOTH artifacts, and CI was refreshing
	// only one of them.
	if os.Getenv("WASM_REFRESHED_BY_CI") != "" {
		t.Skip("CI rebuilds and commits this artifact; staleness here is about to be fixed")
	}

	recorded, err := os.ReadFile("built-from.txt")
	if err != nil {
		t.Skipf("no build record yet: %v", err)
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
		t.Errorf("the embedded Go build is stale: sources now hash to %s, "+
			"the artifact was built from %s.\nRun `make wasm` (or `make wasms` for both) and commit the result.",
			now, was)
	}
}
