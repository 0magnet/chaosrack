// Command wasmstamp prints a fingerprint of the sources the wasm builds are
// made from.
//
// It exists because the TinyGo artifact is COMMITTED and SERVED — the page
// offers a "tinygo" runtime and hands over the bytes in assets/tinywasm — while
// nothing rebuilds it as a side effect of ordinary work. It went stale by a week
// once, was refreshed with a commit that wrote the build command down, and went
// stale again by fifteen commits within two days, because writing the command
// down does not make anyone run it.
//
// So the fingerprint is recorded next to the artifact when it is built, and a
// test compares it against the sources as they are now. Forgetting to rebuild
// stops being invisible and becomes a failing test.
//
// The file list comes from `go list` for the js/wasm target rather than from a
// glob, so it is exactly what the compiler would read: build tags honored, and
// embedded files included, since a change to a shader or a stylesheet changes
// the binary as surely as a change to a .go file.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type pkg struct {
	Dir        string
	ImportPath string
	GoFiles    []string
	EmbedFiles []string
}

func main() {
	cmd := exec.Command("go", "list", "-deps", "-json", "./cmd/wasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wasmstamp: go list:", err)
		os.Exit(1)
	}

	var files []string
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			fmt.Fprintln(os.Stderr, "wasmstamp: decode:", err)
			os.Exit(1)
		}
		// Only this module. The toolchain and the dependencies are pinned by
		// go.mod, which is itself one of the files hashed below.
		if !strings.HasPrefix(p.ImportPath, "github.com/0magnet/chaosrack") {
			continue
		}
		// The wasm assets are the OUTPUT of this build; hashing them would make
		// the fingerprint depend on itself.
		if strings.Contains(p.ImportPath, "/assets/") {
			continue
		}
		for _, f := range append(append([]string{}, p.GoFiles...), p.EmbedFiles...) {
			files = append(files, filepath.Join(p.Dir, f))
		}
	}
	files = append(files, "go.mod", "go.sum")
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // paths come from `go list` for this module, not from input
		if err != nil {
			fmt.Fprintln(os.Stderr, "wasmstamp: read", f, err)
			os.Exit(1)
		}
		// The name goes in too, so moving code between files is a change.
		// A hash writer cannot fail, and treating it as if it could would put an
		// error check on every line of a loop that has nowhere to report one.
		_, _ = fmt.Fprintf(h, "%s\n", filepath.Base(f)) //nolint:errcheck // writing to a hash
		h.Write(b)
	}
	fmt.Printf("%x\n", h.Sum(nil)[:8])
}
