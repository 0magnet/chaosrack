//go:build !js

package hostagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/desk/panes/hostproto"
)

// The filesystem half of the agent.
//
// Separately enabled from the shell, and worth being: a file manager over the
// real filesystem is useful on its own, and giving one out is a smaller thing
// than giving out a shell. The reverse is not true — --shell already implies
// every file this could reach — so the two are independent switches rather than
// one being a subset of the other.
//
// ROOT. Unlike the pty, this can be confined to a subtree, and the confinement
// is real rather than advisory: every path is resolved with filepath.EvalSymlinks
// where it exists and rejected if it leaves the root, so a symlink planted
// inside the root cannot be followed out of it.

// FSConfig is what the filesystem endpoints are allowed to reach.
type FSConfig struct {
	// Root confines every path to this subtree. Empty means the whole
	// filesystem, which is the right default only when --shell is also on;
	// desk-serve is what decides that.
	Root string
}

// FSHandler serves the filesystem endpoints under a mux of its own.
func (c Config) FSHandler(fsc FSConfig) http.Handler {
	mux := http.NewServeMux()
	h := &fsAgent{root: fsc.Root}
	mux.HandleFunc(hostproto.FSStat, h.wrap(h.stat))
	mux.HandleFunc(hostproto.FSList, h.wrap(h.list))
	mux.HandleFunc(hostproto.FSRead, h.wrap(h.read))
	mux.HandleFunc(hostproto.FSWrite, h.wrap(h.write))
	mux.HandleFunc(hostproto.FSMkdir, h.wrap(h.mkdir))
	mux.HandleFunc(hostproto.FSRemove, h.wrap(h.remove))
	mux.HandleFunc(hostproto.FSRename, h.wrap(h.rename))
	mux.HandleFunc(hostproto.FSChmod, h.wrap(h.chmod))
	mux.HandleFunc(hostproto.FSChtimes, h.wrap(h.chtimes))
	mux.HandleFunc(hostproto.FSTruncate, h.wrap(h.truncate))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.checkToken(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := c.checkBrowserOrigin(r); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

type fsAgent struct{ root string }

// resolve turns a request path into a real one, or refuses.
func (h *fsAgent) resolve(p string) (string, error) {
	if p == "" {
		return "", errors.New("no path given")
	}
	if h.root == "" {
		// Unrooted: the client is addressing the machine's own paths, and its
		// "/" is the machine's "/".
		return filepath.Abs(p)
	}
	root, err := filepath.Abs(h.root)
	if err != nil {
		return "", err
	}
	// ROOTED: THE CLIENT'S "/" IS THE ROOT, and this is what makes the whole
	// thing usable rather than merely safe. A shell in the tab has a working
	// directory and it starts at "/"; if that meant the machine's "/" while
	// the root was somewhere else, every single path would be refused and the
	// filesystem would appear broken rather than confined.
	//
	// Cleaning "/"+p before joining is also the traversal guard: Clean
	// resolves ".." against that leading slash, so "/../../etc/passwd"
	// becomes "/etc/passwd" and lands inside the root. Escaping textually is
	// not possible; the symlink walk below is what stops escaping for real.
	abs := filepath.Join(root, filepath.Clean("/"+p))
	// Symlinks are resolved on the LONGEST EXISTING PREFIX, not on the whole
	// path, because a create names something that does not exist yet. Checking
	// only the literal string would let a symlink inside the root point out of
	// it and be followed on the next open.
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		real = probe
	}
	rest := strings.TrimPrefix(abs, probe)
	final := filepath.Join(real, rest)
	if final != root && !strings.HasPrefix(final, root+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the served root", p)
	}
	return final, nil
}

// wrap turns a handler that can fail into one that reports the failure in the
// shape the client can turn back into an os error.
func (h *fsAgent) wrap(f func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			writeFSError(w, err)
		}
	}
}

func writeFSError(w http.ResponseWriter, err error) {
	kind, code := hostproto.ErrOther, http.StatusBadRequest
	switch {
	case errors.Is(err, fs.ErrNotExist):
		kind, code = hostproto.ErrNotExist, http.StatusNotFound
	case errors.Is(err, fs.ErrExist):
		kind, code = hostproto.ErrExist, http.StatusConflict
	case errors.Is(err, fs.ErrPermission):
		kind, code = hostproto.ErrPermission, http.StatusForbidden
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(hostproto.ErrorReply{Kind: kind, Msg: err.Error()}) //nolint:errcheck
}

func (h *fsAgent) path(r *http.Request) (string, error) {
	return h.resolve(r.URL.Query().Get(hostproto.PathParam))
}

func toInfo(fi os.FileInfo) hostproto.FileInfo {
	return hostproto.FileInfo{
		Name: fi.Name(),
		Size: fi.Size(),
		Mode: uint32(fi.Mode()),
		Mod:  fi.ModTime().UnixNano(),
		Dir:  fi.IsDir(),
	}
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

func (h *fsAgent) stat(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	fi, err := os.Stat(p) //nolint:gosec // guarded by resolve
	if err != nil {
		return err
	}
	return writeJSON(w, toInfo(fi))
}

func (h *fsAgent) list(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	// resolve confined p to the served root; see the fsAgent doc.
	ents, err := os.ReadDir(p) //nolint:gosec // guarded by resolve
	if err != nil {
		return err
	}
	out := make([]hostproto.FileInfo, 0, len(ents))
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil {
			// A file that vanished between the listing and the stat is
			// normal in a live directory; leaving it out beats failing the
			// whole listing.
			continue
		}
		out = append(out, toInfo(fi))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return writeJSON(w, out)
}

func (h *fsAgent) read(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	f, err := os.Open(p) //nolint:gosec // guarded by resolve; serving files is the point
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory", p)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	_, err = io.Copy(w, f)
	return err
}

func (h *fsAgent) write(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	perm := parsePerm(r, 0o644)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, body, perm); err != nil { //nolint:gosec // guarded by resolve
		return err
	}
	fi, err := os.Stat(p) //nolint:gosec // guarded by resolve
	if err != nil {
		return err
	}
	return writeJSON(w, toInfo(fi))
}

func (h *fsAgent) mkdir(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	perm := parsePerm(r, 0o755)
	if r.URL.Query().Get(hostproto.AllParam) == "1" {
		err = os.MkdirAll(p, perm) //nolint:gosec // guarded by resolve
	} else {
		err = os.Mkdir(p, perm) //nolint:gosec // guarded by resolve
	}
	if err != nil {
		return err
	}
	return writeJSON(w, struct{}{})
}

func (h *fsAgent) remove(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	if r.URL.Query().Get(hostproto.AllParam) == "1" {
		err = os.RemoveAll(p) //nolint:gosec // guarded by resolve
	} else {
		err = os.Remove(p) //nolint:gosec // guarded by resolve
	}
	if err != nil {
		return err
	}
	return writeJSON(w, struct{}{})
}

func (h *fsAgent) rename(w http.ResponseWriter, r *http.Request) error {
	from, err := h.path(r)
	if err != nil {
		return err
	}
	to, err := h.resolve(r.URL.Query().Get(hostproto.ToParam))
	if err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil { //nolint:gosec // both sides guarded by resolve
		return err
	}
	return writeJSON(w, struct{}{})
}

func (h *fsAgent) chmod(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	if err := os.Chmod(p, parsePerm(r, 0o644)); err != nil { //nolint:gosec // guarded by resolve
		return err
	}
	return writeJSON(w, struct{}{})
}

func (h *fsAgent) chtimes(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	at, err := strconv.ParseInt(r.URL.Query().Get(hostproto.ATimeParam), 10, 64)
	if err != nil {
		return fmt.Errorf("bad access time: %w", err)
	}
	mt, err := strconv.ParseInt(r.URL.Query().Get(hostproto.MTimeParam), 10, 64)
	if err != nil {
		return fmt.Errorf("bad modification time: %w", err)
	}
	if err := os.Chtimes(p, time.Unix(0, at), time.Unix(0, mt)); err != nil {
		return err
	}
	return writeJSON(w, struct{}{})
}

func (h *fsAgent) truncate(w http.ResponseWriter, r *http.Request) error {
	p, err := h.path(r)
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(r.URL.Query().Get(hostproto.SizeParam), 10, 64)
	if err != nil {
		return err
	}
	if err := os.Truncate(p, n); err != nil {
		return err
	}
	return writeJSON(w, struct{}{})
}

func parsePerm(r *http.Request, def os.FileMode) os.FileMode {
	v := r.URL.Query().Get(hostproto.PermParam)
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 8, 32)
	if err != nil {
		return def
	}
	return os.FileMode(n)
}
