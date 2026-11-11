//go:build !js

package wasmfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0magnet/chaosrack/assets/gowasm"
	"github.com/0magnet/chaosrack/assets/metersworker"
	"github.com/0magnet/chaosrack/assets/tinywasm"
)

// pack is a bundle's archive holding files.
func pack(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func whole(tag string) map[string]string {
	m := map[string]string{}
	for _, n := range names {
		m[n] = tag + ":" + n
	}
	return m
}

// A bundle is the files it is meant to hold and no others, and is no bundle
// without the Go build: a page cannot be served from the rest.
func TestABundleIsItsNamedFilesAndNeedsTheGoBuild(t *testing.T) {
	files := whole("a")
	files["../../etc/passwd"] = "x"
	files["gowasm/extra.sh"] = "x"
	b, err := Read(bytes.NewReader(pack(t, files)))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != len(names) {
		t.Errorf("the bundle holds %d files, want the %d it names", len(b), len(names))
	}
	if got := string(b["tinywasm/chaosrack-tiny.wasm"]); got != "a:tinywasm/chaosrack-tiny.wasm" {
		t.Errorf("the TinyGo build read as %q", got)
	}
	files = whole("a")
	delete(files, "gowasm/chaosrack.wasm")
	if _, err := Read(bytes.NewReader(pack(t, files))); err == nil {
		t.Error("a bundle with no Go build was read as one")
	}
	if _, err := Read(strings.NewReader("<html>Not Found</html>")); err == nil {
		t.Error("an error page was read as a bundle")
	}
}

// The commit a go install build is asked for by is the one in its version.
func TestAPseudoVersionNamesItsCommit(t *testing.T) {
	for v, want := range map[string]string{
		"v0.0.0-20261111125654-d4e28b5a5fa8":       "d4e28b5a5fa8",
		"v0.0.3-0.20261111125654-d4e28b5a5fa8":     "d4e28b5a5fa8",
		"v0.0.0-20261111125654-d4e28b5a5fa8+dirty": "d4e28b5a5fa8",
		"(devel)":           "",
		"v1.2.3":            "",
		"v1.2.3-rc1":        "",
		"v1.2.3-notacommit": "",
		"":                  "",
	} {
		if got := pseudoCommit(v); got != want {
			t.Errorf("%q names commit %q, want %q", v, got, want)
		}
	}
	if got := BundleName("d4e28b5a5fa8aaaaaaaaaaaaaaaaaaaaaaaaaaaa"); got != "chaosrack-wasm-d4e28b5a5fa8.tar.gz" {
		t.Errorf("a full hash names %q", got)
	}
}

// served is a release holding the bundles given, counting what it is asked.
type served struct {
	bundles map[string][]byte
	asked   []string
	down    bool
}

func (s *served) get(url string) ([]byte, int, error) {
	name := url[strings.LastIndex(url, "/")+1:]
	s.asked = append(s.asked, name)
	if s.down {
		return nil, 0, errors.New("no network")
	}
	if b, ok := s.bundles[name]; ok {
		return b, http.StatusOK, nil
	}
	return nil, http.StatusNotFound, nil
}

func source(t *testing.T, s *served, now *time.Time) (Source, *[]string) {
	t.Helper()
	var said []string
	return Source{
		URL: "https://example.invalid/", Dir: t.TempDir(), Get: s.get,
		Now:  func() time.Time { return *now },
		Logf: func(f string, _ ...any) { said = append(said, f) },
	}, &said
}

// A binary gets the build of its own commit where there is one, and it is
// fetched once: the second asking reads the cache.
func TestACommitsOwnBundleIsUsedAndKept(t *testing.T) {
	now := time.Now()
	rel := &served{bundles: map[string][]byte{
		BundleName("aaaaaaaaaaaa"): pack(t, whole("mine")),
		Latest:                     pack(t, whole("newest")),
	}}
	src, _ := source(t, rel, &now)
	for range 2 {
		b, err := src.Bundle("aaaaaaaaaaaabbbb")
		if err != nil {
			t.Fatal(err)
		}
		if got := string(b["gowasm/chaosrack.wasm"]); !strings.HasPrefix(got, "mine:") {
			t.Fatalf("the build is %q, want this commit's", got)
		}
	}
	if len(rel.asked) != 1 {
		t.Errorf("the release was asked %d times %v, want once", len(rel.asked), rel.asked)
	}
	if _, err := os.Stat(filepath.Join(src.Dir, BundleName("aaaaaaaaaaaa"))); err != nil {
		t.Errorf("the bundle was not kept: %v", err)
	}
}

// Where CI has published none for the commit, the newest is used, and said
// to be: it may be of other source. It is asked for again after a day, and
// off the network the copy from before still serves.
func TestWithNoBundleOfItsOwnTheNewestIsUsedAndSaid(t *testing.T) {
	now := time.Now()
	rel := &served{bundles: map[string][]byte{Latest: pack(t, whole("newest"))}}
	src, said := source(t, rel, &now)
	b, err := src.Bundle("cccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b["gowasm/chaosrack.wasm"]); !strings.HasPrefix(got, "newest:") {
		t.Fatalf("the build is %q, want the newest", got)
	}
	if !strings.Contains(strings.Join(*said, "\n"), "no build is published for this commit") {
		t.Errorf("nothing said the build is not this commit's: %q", *said)
	}

	// Within the day the newest is not asked for again; after it, it is.
	rel.asked = nil
	if _, err := src.Bundle(""); err != nil {
		t.Fatal(err)
	}
	if len(rel.asked) != 0 {
		t.Errorf("a fresh newest bundle was asked for again: %v", rel.asked)
	}
	now = now.Add(latestFor + time.Hour)
	rel.bundles[Latest] = pack(t, whole("newer"))
	if b, err = src.Bundle(""); err != nil || !strings.HasPrefix(string(b["gowasm/chaosrack.wasm"]), "newer:") {
		t.Errorf("after a day the newest was not fetched again: %q, %v", b["gowasm/chaosrack.wasm"], err)
	}
	now = now.Add(latestFor + time.Hour)
	rel.down = true
	if b, err = src.Bundle(""); err != nil || !strings.HasPrefix(string(b["gowasm/chaosrack.wasm"]), "newer:") {
		t.Errorf("off the network the kept copy was not used: %q, %v", b["gowasm/chaosrack.wasm"], err)
	}

	// With nothing kept and no network there is nothing to serve.
	empty, _ := source(t, rel, &now)
	if _, err := empty.Bundle("cccccccccccc"); err == nil {
		t.Error("a bundle came from no cache and no network")
	}
}

// A package is filled whole or not at all: a build this binary has keeps its
// own shim, and one it lacks takes the bundle's with it.
func TestFillLeavesABuildTheBinaryHasAlone(t *testing.T) {
	gw, ge, gb := gowasm.Wasm, gowasm.WasmExec, gowasm.BuiltFrom
	tw, te, tb := tinywasm.Wasm, tinywasm.WasmExec, tinywasm.BuiltFrom
	mw, mj := metersworker.Wasm, metersworker.WorkerJS
	defer func() {
		gowasm.Wasm, gowasm.WasmExec, gowasm.BuiltFrom = gw, ge, gb
		tinywasm.Wasm, tinywasm.WasmExec, tinywasm.BuiltFrom = tw, te, tb
		metersworker.Wasm, metersworker.WorkerJS = mw, mj
	}()
	gowasm.Wasm, gowasm.WasmExec, gowasm.BuiltFrom = []byte("local"), []byte("local shim"), []byte("local stamp")
	tinywasm.Wasm, tinywasm.WasmExec, tinywasm.BuiltFrom = nil, []byte("checked-in shim"), nil
	metersworker.Wasm, metersworker.WorkerJS = nil, []byte("checked-in worker")

	b := Bundle{}
	for n, v := range whole("b") {
		b[n] = []byte(v)
	}
	Fill(b)
	if string(gowasm.Wasm) != "local" || string(gowasm.WasmExec) != "local shim" || string(gowasm.BuiltFrom) != "local stamp" {
		t.Error("the Go build this binary has was replaced")
	}
	if string(tinywasm.Wasm) != "b:tinywasm/chaosrack-tiny.wasm" || string(tinywasm.WasmExec) != "b:tinywasm/tinygo_wasm_exec.js" || string(tinywasm.BuiltFrom) != "b:tinywasm/built-from.txt" {
		t.Errorf("the TinyGo build was not taken whole: shim %q, stamp %q", tinywasm.WasmExec, tinywasm.BuiltFrom)
	}
	if string(metersworker.Wasm) != "b:metersworker/meters.wasm" || string(metersworker.WorkerJS) != "b:metersworker/worker.js" {
		t.Errorf("the worker was not taken whole: script %q", metersworker.WorkerJS)
	}

	// A bundle with no TinyGo build leaves that runtime absent, not half there.
	tinywasm.Wasm, tinywasm.WasmExec = nil, []byte("checked-in shim")
	delete(b, "tinywasm/chaosrack-tiny.wasm")
	Fill(b)
	if tinywasm.Has() {
		t.Error("a TinyGo runtime was made from a bundle with no TinyGo build")
	}
}
