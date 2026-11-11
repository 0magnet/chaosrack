//go:build !js

// Package wasmfetch gives a binary built without the wasm builds the ones CI
// publishes.
//
// The builds were committed, by CI, on every push: fifty-seven megabytes a
// time, 1.2 GB of the repository's history in seven weeks. They are release
// assets now, and assets/gowasm, assets/tinywasm and assets/metersworker hold
// a build only where `make wasm` (or tinywasm, metersworker) has put one. A
// binary made from a bare checkout, or by go install, has none: Ensure
// downloads the bundle once, keeps it in the user's cache directory, and
// fills the three packages from it.
//
// A bundle is asked for by the commit the binary was built from, so the page
// is the one that commit's source makes; where CI has not published that one
// (not yet, or no longer), the newest is used and Ensure says so.
package wasmfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/assets/gowasm"
	"github.com/0magnet/chaosrack/assets/metersworker"
	"github.com/0magnet/chaosrack/assets/tinywasm"
)

// ReleaseURL is where CI publishes the bundles (see .github/workflows/ci.yml).
const ReleaseURL = "https://github.com/0magnet/chaosrack/releases/download/wasm/"

// Latest is the newest bundle's name; a commit's is BundleName(commit).
const Latest = "chaosrack-wasm.tar.gz"

// latestFor is how long a cached newest bundle is used before asking again.
const latestFor = 24 * time.Hour

// maxFile bounds one file out of a bundle: the Go build is 33 MB.
const maxFile = 256 << 20

// BundleName is the name of the bundle built from commit, by its first
// twelve characters, as a pseudo-version carries them.
func BundleName(commit string) string {
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return "chaosrack-wasm-" + commit + ".tar.gz"
}

// A Bundle is the files of one published build, by their path under assets/.
type Bundle map[string][]byte

// names are the files a bundle may hold. Anything else in the archive is
// not read: the names become nothing on disk, but a list is also what says
// a bundle is whole.
var names = []string{
	"gowasm/chaosrack.wasm", "gowasm/wasm_exec.js", "gowasm/built-from.txt",
	"tinywasm/chaosrack-tiny.wasm", "tinywasm/tinygo_wasm_exec.js", "tinywasm/built-from.txt",
	"metersworker/meters.wasm", "metersworker/worker.js",
}

// Read is the bundle in a gzipped tar.
func Read(r io.Reader) (Bundle, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	b := Bundle{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(h.Name, "./")
		if h.Typeflag != tar.TypeReg || !want[name] {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxFile+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxFile {
			return nil, fmt.Errorf("%s is larger than %d bytes", name, maxFile)
		}
		b[name] = data
	}
	if len(b["gowasm/chaosrack.wasm"]) == 0 || len(b["gowasm/wasm_exec.js"]) == 0 {
		return nil, errors.New("the bundle holds no Go build")
	}
	return b, nil
}

// Commit is the commit this binary was built from: the checkout's, for a
// build in one, or the one in the module's pseudo-version, for go install.
// Empty where neither is recorded.
func Commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return pseudoCommit(info.Main.Version)
}

// pseudoCommit is the commit a pseudo-version names, v0.0.0-20261111125654-d4e28b5a5fa8.
func pseudoCommit(version string) string {
	i := strings.LastIndex(version, "-")
	if i < 0 {
		return ""
	}
	c := strings.TrimSuffix(version[i+1:], "+dirty")
	if len(c) != 12 || strings.Trim(c, "0123456789abcdef") != "" {
		return ""
	}
	return c
}

// A Source is where bundles come from and where they are kept.
type Source struct {
	URL  string                                // ReleaseURL, ending in a slash
	Dir  string                                // the cache; no cache when empty
	Get  func(url string) ([]byte, int, error) // body and status
	Now  func() time.Time
	Logf func(format string, a ...any)
}

// DefaultSource is the published release, kept in the user's cache directory.
func DefaultSource(logf func(string, ...any)) Source {
	dir, err := os.UserCacheDir()
	if err == nil {
		dir = filepath.Join(dir, "chaosrack", "wasm")
	}
	return Source{URL: ReleaseURL, Dir: dir, Get: httpGet, Now: time.Now, Logf: logf}
}

func httpGet(url string) ([]byte, int, error) {
	c := http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url) //nolint:gosec,noctx // ReleaseURL and a bundle's name, neither from input
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*maxFile))
	return body, resp.StatusCode, err
}

// Bundle is the bundle for commit: its own if CI published one, the newest
// otherwise. From the cache where it is there, a newest one for a day.
func (s Source) Bundle(commit string) (Bundle, error) {
	if commit != "" {
		if b, err := s.named(BundleName(commit), 0); err == nil {
			return b, nil
		} else if !errors.Is(err, errNotPublished) {
			s.Logf("wasm: this build's own bundle: %v", err)
		}
	}
	b, err := s.named(Latest, latestFor)
	if err != nil {
		return nil, err
	}
	if commit != "" {
		s.Logf("wasm: no build is published for this commit (%.12s); using the newest, which may be of other source", commit)
	}
	return b, nil
}

var errNotPublished = errors.New("not published")

// named is one bundle by name, from the cache if it is there and, where
// fresh is not zero, no older than that.
func (s Source) named(name string, fresh time.Duration) (Bundle, error) {
	cached := ""
	if s.Dir != "" {
		cached = filepath.Join(s.Dir, name)
	}
	var old []byte
	if cached != "" {
		if st, err := os.Stat(cached); err == nil {
			if data, err := os.ReadFile(cached); err == nil { //nolint:gosec // the cache directory and a bundle's name
				if fresh == 0 || s.Now().Sub(st.ModTime()) < fresh {
					if b, err := Read(bytes.NewReader(data)); err == nil {
						return b, nil
					}
				}
				old = data
			}
		}
	}
	s.Logf("wasm: fetching %s", s.URL+name)
	data, status, err := s.Get(s.URL + name)
	if err == nil && status == http.StatusNotFound {
		return nil, errNotPublished
	}
	if err == nil && data == nil {
		err = fmt.Errorf("%s answered %d", s.URL+name, status)
	}
	var b Bundle
	if err == nil {
		b, err = Read(bytes.NewReader(data))
	}
	if err != nil {
		// Off the network, yesterday's build is better than none.
		if ob, oerr := Read(bytes.NewReader(old)); old != nil && oerr == nil {
			s.Logf("wasm: %v; using the copy from before", err)
			return ob, nil
		}
		return nil, err
	}
	if cached != "" {
		if err := os.MkdirAll(s.Dir, 0o750); err == nil {
			tmp := cached + ".part"
			if err := os.WriteFile(tmp, data, 0o600); err == nil {
				_ = os.Rename(tmp, cached) //nolint:errcheck // an unkept bundle is fetched again
			}
		}
	}
	return b, nil
}

// Ensure fills whichever of the three builds this binary was made without.
// With a Go build in the binary and nothing to be had, the others stay
// empty and the page runs without them, as it did before there was a TinyGo
// build; with no Go build either, that is the error.
func Ensure(logf func(format string, a ...any)) error {
	if gowasm.Has() && tinywasm.Has() && len(metersworker.Wasm) > 0 {
		return nil
	}
	b, err := DefaultSource(logf).Bundle(Commit())
	if err != nil {
		if gowasm.Has() {
			logf("wasm: %v; serving the builds this binary has", err)
			return nil
		}
		return fmt.Errorf("this binary was built without the page's wasm, and it could not be fetched (%w): "+
			"run `make wasm` in a checkout and build again, or run this once with a network", err)
	}
	Fill(b)
	return nil
}

// Fill gives each package that has no build the bundle's, whole: a wasm with
// the shim and the stamp that were made with it, never one beside another's.
func Fill(b Bundle) {
	if !gowasm.Has() {
		gowasm.Wasm, gowasm.WasmExec, gowasm.BuiltFrom = b["gowasm/chaosrack.wasm"], b["gowasm/wasm_exec.js"], b["gowasm/built-from.txt"]
	}
	if !tinywasm.Has() && len(b["tinywasm/chaosrack-tiny.wasm"]) > 0 && len(b["tinywasm/tinygo_wasm_exec.js"]) > 0 {
		tinywasm.Wasm, tinywasm.WasmExec, tinywasm.BuiltFrom = b["tinywasm/chaosrack-tiny.wasm"], b["tinywasm/tinygo_wasm_exec.js"], b["tinywasm/built-from.txt"]
	}
	if len(metersworker.Wasm) == 0 && len(b["metersworker/meters.wasm"]) > 0 && len(b["metersworker/worker.js"]) > 0 {
		metersworker.Wasm, metersworker.WorkerJS = b["metersworker/meters.wasm"], b["metersworker/worker.js"]
	}
}
