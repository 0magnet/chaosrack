//go:build js && wasm

// Package hostfs is the real filesystem, as an afero.Fs.
//
// This is the lever the whole "desktop with access to the system" idea turns
// on, and it is short because of one fact: websh's shell and the desk's file
// manager both work against afero.Fs, an INTERFACE, and websh takes any
// implementation through web.Options.FS. So a single afero.Fs backed by the
// host makes the file manager list real directories AND gives the wasm shell
// real files — `ls`, `cat`, `grep`, redirection, globbing — with no change to
// either of them. The interpreter still runs in the tab; only what it reads
// and writes becomes real.
//
// # Why synchronous XMLHttpRequest, of all things
//
// afero.Fs is a synchronous interface: Open returns a File, not a promise.
// Everything in a browser that fetches is asynchronous, so something has to
// bridge them, and there are only two ways.
//
// The first is fetch plus a channel, blocking the calling goroutine until the
// response arrives. That is the idiomatic Go answer and it deadlocks here: in
// js/wasm the DOM event handlers ARE the main goroutine, so a file manager
// blocking in a click handler blocks the event loop that would deliver its own
// response, and the page hangs forever with no error and no way out.
//
// The second is a synchronous request, which blocks the JS thread and cannot
// deadlock because the browser services the request itself. It is deprecated on
// the main thread and it does stall rendering — but the far end is a loopback
// listener on the same machine, where a stat is measured in tens of
// microseconds. A hang that never resolves is a much worse failure than a frame
// that arrives late, and this way the same code works from a click handler and
// from the shell's goroutine without either caring.
//
// The awkwardness is confined to one function. request is the only thing here
// that knows any of this.
package hostfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"syscall/js"
	"time"

	"github.com/0magnet/afero"

	"github.com/0magnet/desk/panes/hostauth"

	"github.com/0magnet/desk/panes/hostproto"
)

// Fs is the host's filesystem.
type Fs struct {
	token string
}

// Compose returns the host filesystem as a desk should actually mount it, or
// false when the page was served without one.
//
// /BIN STAYS SYNTHETIC. websh's shell calls PopulateBin on startup, writing a
// stub file per applet so that `ls /bin` shows the command set — right for a
// filesystem that lives in a tab, and exactly wrong for the machine's: pointed
// at a home directory, the first thing the desk would do is scatter fifty empty
// files into it.
//
// This lives here rather than in each desk because it was previously two
// identical copies, one in cmd/desk and one in chaosrack, each carrying its own
// copy of the paragraph above. A rule about what a host filesystem must not
// write to somebody's disk should have one home.
func Compose() (afero.Fs, bool) {
	hf, ok := New()
	if !ok {
		return nil, false
	}
	return Mount(hf, map[string]afero.Fs{"/bin": afero.NewMemMapFs()}), true
}

// New returns the host filesystem, or false when the page was served without
// one. Absent is the ordinary case — a static host, or desk-serve without the
// flag — and callers are expected to fall back to the in-memory filesystem
// rather than treat it as an error.
func New() (*Fs, bool) {
	h := js.Global().Get("__deskHost")
	if !h.Truthy() {
		return nil, false
	}
	// The fs flag is checked separately from the token because the two grants
	// are separate: a page served with --shell but not --fs has a valid token
	// and no filesystem behind it, and answering true there would turn every
	// directory listing into a 404 instead of a clean fall back to memory.
	if !h.Get("fs").Truthy() {
		return nil, false
	}
	// May ask the person for it — see hostauth. An empty answer is a decision,
	// not a failure: the caller falls back to the in-memory filesystem.
	tok := hostauth.Token()
	if tok == "" {
		return nil, false
	}
	return &Fs{token: tok}, true
}

// Name identifies this filesystem. It shows up in afero's error strings, so it
// is worth being the thing a person needs to know: these files are not in the
// tab.
func (f *Fs) Name() string { return "hostfs" }

// request performs one synchronous call. See the package comment for why.
func (f *Fs) request(method, endpoint string, q url.Values, body []byte) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set(hostproto.TokenParam, f.token)
	u := endpoint + "?" + q.Encode()

	xhr := js.Global().Get("XMLHttpRequest").New()
	xhr.Call("open", method, u, false) // false: synchronous

	// A synchronous XHR is not allowed to set responseType, so the response
	// cannot be asked for as an ArrayBuffer. This is the standard way around
	// it: x-user-defined maps every byte to a distinct code point, so the
	// bytes survive in responseText and can be masked back out. Without it
	// the response would be decoded as UTF-8 and any file that is not valid
	// UTF-8 — every binary file — would come back corrupted.
	xhr.Call("overrideMimeType", "text/plain; charset=x-user-defined")

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("hostfs: %s %s: %v", method, endpoint, r)
			}
		}()
		if body == nil {
			xhr.Call("send")
			return
		}
		buf := js.Global().Get("Uint8Array").New(len(body))
		js.CopyBytesToJS(buf, body)
		xhr.Call("send", buf)
	}()
	if err != nil {
		return nil, err
	}

	raw := decodeUserDefined(xhr.Get("responseText").String())
	if status := xhr.Get("status").Int(); status < 200 || status > 299 {
		return nil, decodeError(raw, status)
	}
	return raw, nil
}

func pathValues(name string) url.Values {
	return url.Values{hostproto.PathParam: []string{name}}
}

func (f *Fs) statInfo(name string) (*info, error) {
	body, err := f.request("GET", hostproto.FSStat, pathValues(name), nil)
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	var fi hostproto.FileInfo
	if err := json.Unmarshal(body, &fi); err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	return &info{fi}, nil
}

// Stat implements afero.Fs.
func (f *Fs) Stat(name string) (os.FileInfo, error) {
	i, err := f.statInfo(name)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// Open opens a file or directory for reading.
func (f *Fs) Open(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

// Create makes or truncates a file.
func (f *Fs) Create(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

// OpenFile is where the whole-file model lives.
//
// A file is read in full on open and written in full on close, rather than the
// handle being kept open on the host and seeks turned into range requests.
// That is a real limitation and worth stating plainly: it costs memory
// proportional to the file, and two writers to one file will not interleave,
// the last close winning entirely.
//
// It is the right trade here because of what actually opens files through this
// — a file manager showing a listing, a viewer, and a shell running cat, grep
// or a redirection. Those read or write a whole file and then stop. The
// alternative is a session per open handle, with the lifetime problem that
// brings when the tab holding them is closed by the window manager.
func (f *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	h := &File{fs: f, name: name, perm: perm, flag: flag}
	h.write = flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0

	fi, statErr := f.statInfo(name)
	switch {
	case statErr == nil && fi.IsDir():
		h.dir = true
		return h, nil
	case statErr == nil && flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0:
		return nil, &os.PathError{Op: "open", Path: name, Err: fs.ErrExist}
	case statErr != nil:
		if flag&os.O_CREATE == 0 {
			return nil, statErr
		}
		h.dirty = true // nothing to read; the file begins empty and unsaved
		return h, nil
	}

	if flag&os.O_TRUNC != 0 {
		h.dirty = true
		return h, nil
	}
	body, err := f.request("GET", hostproto.FSRead, pathValues(name), nil)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	h.buf = body
	if flag&os.O_APPEND != 0 {
		h.off = int64(len(body))
	}
	return h, nil
}

// Mkdir creates one directory.
func (f *Fs) Mkdir(name string, perm os.FileMode) error {
	q := pathValues(name)
	q.Set(hostproto.PermParam, strconv.FormatUint(uint64(perm.Perm()), 8))
	_, err := f.request("POST", hostproto.FSMkdir, q, nil)
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return nil
}

// MkdirAll creates a directory and its parents.
func (f *Fs) MkdirAll(name string, perm os.FileMode) error {
	q := pathValues(name)
	q.Set(hostproto.PermParam, strconv.FormatUint(uint64(perm.Perm()), 8))
	q.Set(hostproto.AllParam, "1")
	_, err := f.request("POST", hostproto.FSMkdir, q, nil)
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return nil
}

// Remove deletes one file or empty directory.
func (f *Fs) Remove(name string) error { return f.remove(name, false) }

// RemoveAll deletes a tree.
func (f *Fs) RemoveAll(name string) error { return f.remove(name, true) }

func (f *Fs) remove(name string, all bool) error {
	q := pathValues(name)
	if all {
		q.Set(hostproto.AllParam, "1")
	}
	if _, err := f.request("POST", hostproto.FSRemove, q, nil); err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}
	return nil
}

// Rename moves a file.
func (f *Fs) Rename(oldname, newname string) error {
	q := pathValues(oldname)
	q.Set(hostproto.ToParam, newname)
	if _, err := f.request("POST", hostproto.FSRename, q, nil); err != nil {
		return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: err}
	}
	return nil
}

// Chmod changes a file's mode.
func (f *Fs) Chmod(name string, mode os.FileMode) error {
	q := pathValues(name)
	q.Set(hostproto.PermParam, strconv.FormatUint(uint64(mode.Perm()), 8))
	if _, err := f.request("POST", hostproto.FSChmod, q, nil); err != nil {
		return &os.PathError{Op: "chmod", Path: name, Err: err}
	}
	return nil
}

// Chown is not offered.
//
// Refused rather than silently ignored: a filesystem that accepts a chown and
// does nothing is worse than one that says it cannot, because the caller
// believes the ownership changed. Changing owners generally needs privileges
// the agent does not have and should not be asking for.
func (f *Fs) Chown(name string, _, _ int) error {
	return &os.PathError{Op: "chown", Path: name, Err: errors.New("hostfs does not change ownership")}
}

// Chtimes sets access and modification times.
func (f *Fs) Chtimes(name string, atime, mtime time.Time) error {
	q := pathValues(name)
	q.Set(hostproto.ATimeParam, strconv.FormatInt(atime.UnixNano(), 10))
	q.Set(hostproto.MTimeParam, strconv.FormatInt(mtime.UnixNano(), 10))
	if _, err := f.request("POST", hostproto.FSChtimes, q, nil); err != nil {
		return &os.PathError{Op: "chtimes", Path: name, Err: err}
	}
	return nil
}

// info adapts the wire form to os.FileInfo.
type info struct{ fi hostproto.FileInfo }

func (i *info) Name() string       { return i.fi.Name }
func (i *info) Size() int64        { return i.fi.Size }
func (i *info) Mode() os.FileMode  { return os.FileMode(i.fi.Mode) }
func (i *info) ModTime() time.Time { return time.Unix(0, i.fi.Mod) }
func (i *info) IsDir() bool        { return i.fi.Dir }
func (i *info) Sys() any           { return nil }

// File is an open file, held whole in memory. See OpenFile.
type File struct {
	fs    *Fs
	name  string
	perm  os.FileMode
	flag  int
	buf   []byte
	off   int64
	dirty bool
	write bool
	dir   bool
	ents  []os.FileInfo
	entAt int
	shut  bool
}

// Name returns the path this was opened with.
func (h *File) Name() string { return h.name }

func (h *File) Read(p []byte) (int, error) {
	if h.dir {
		return 0, &os.PathError{Op: "read", Path: h.name, Err: errors.New("is a directory")}
	}
	if h.off >= int64(len(h.buf)) {
		return 0, io.EOF
	}
	n := copy(p, h.buf[h.off:])
	h.off += int64(n)
	return n, nil
}

func (h *File) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off > int64(len(h.buf)) {
		return 0, io.EOF
	}
	n := copy(p, h.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *File) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = h.off + offset
	case io.SeekEnd:
		next = int64(len(h.buf)) + offset
	default:
		return 0, errors.New("hostfs: bad whence")
	}
	if next < 0 {
		return 0, errors.New("hostfs: negative position")
	}
	h.off = next
	return next, nil
}

func (h *File) Write(p []byte) (int, error) {
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.name, Err: fs.ErrPermission}
	}
	h.grow(h.off + int64(len(p)))
	n := copy(h.buf[h.off:], p)
	h.off += int64(n)
	h.dirty = true
	return n, nil
}

func (h *File) WriteAt(p []byte, off int64) (int, error) {
	if !h.write {
		return 0, &os.PathError{Op: "write", Path: h.name, Err: fs.ErrPermission}
	}
	h.grow(off + int64(len(p)))
	n := copy(h.buf[off:], p)
	h.dirty = true
	return n, nil
}

// WriteString is on the interface, and afero's callers use it heavily.
func (h *File) WriteString(s string) (int, error) { return h.Write([]byte(s)) }

func (h *File) grow(to int64) {
	for int64(len(h.buf)) < to {
		h.buf = append(h.buf, 0)
	}
}

// Truncate resizes the buffer; the host sees it at the next flush.
func (h *File) Truncate(size int64) error {
	if !h.write {
		return &os.PathError{Op: "truncate", Path: h.name, Err: fs.ErrPermission}
	}
	if size < int64(len(h.buf)) {
		h.buf = h.buf[:size]
	} else {
		h.grow(size)
	}
	h.dirty = true
	return nil
}

// Sync writes the buffer back.
//
// This is the one place the whole-file model shows through, and it is why Sync
// is implemented rather than being a no-op: a program that writes and syncs
// without closing — which is exactly what a careful editor does — would
// otherwise lose everything if the tab went away.
func (h *File) Sync() error {
	if !h.dirty {
		return nil
	}
	q := pathValues(h.name)
	q.Set(hostproto.PermParam, strconv.FormatUint(uint64(h.perm.Perm()), 8))
	if _, err := h.fs.request("POST", hostproto.FSWrite, q, h.buf); err != nil {
		return &os.PathError{Op: "write", Path: h.name, Err: err}
	}
	h.dirty = false
	return nil
}

// Close flushes and marks the handle spent.
func (h *File) Close() error {
	if h.shut {
		return &os.PathError{Op: "close", Path: h.name, Err: fs.ErrClosed}
	}
	h.shut = true
	return h.Sync()
}

// Stat describes the file. A dirty handle reports the size it has in memory,
// not the size on disk, because that is the size the caller just produced.
func (h *File) Stat() (os.FileInfo, error) {
	if h.dirty {
		return &pending{name: path.Base(h.name), size: int64(len(h.buf)), mode: h.perm}, nil
	}
	return h.fs.Stat(h.name)
}

func (h *File) loadEntries() error {
	if h.ents != nil {
		return nil
	}
	body, err := h.fs.request("GET", hostproto.FSList, pathValues(h.name), nil)
	if err != nil {
		return &os.PathError{Op: "readdir", Path: h.name, Err: err}
	}
	var raw []hostproto.FileInfo
	if err := json.Unmarshal(body, &raw); err != nil {
		return &os.PathError{Op: "readdir", Path: h.name, Err: err}
	}
	h.ents = make([]os.FileInfo, 0, len(raw))
	for i := range raw {
		h.ents = append(h.ents, &info{raw[i]})
	}
	sort.Slice(h.ents, func(i, j int) bool { return h.ents[i].Name() < h.ents[j].Name() })
	return nil
}

// Readdir returns directory entries, following os.File's contract: a count of
// zero or less means everything, and a positive count reads in batches and
// reports io.EOF once they are exhausted.
func (h *File) Readdir(count int) ([]os.FileInfo, error) {
	if !h.dir {
		return nil, &os.PathError{Op: "readdir", Path: h.name, Err: errors.New("not a directory")}
	}
	if err := h.loadEntries(); err != nil {
		return nil, err
	}
	if count <= 0 {
		out := h.ents[h.entAt:]
		h.entAt = len(h.ents)
		return out, nil
	}
	if h.entAt >= len(h.ents) {
		return nil, io.EOF
	}
	end := h.entAt + count
	if end > len(h.ents) {
		end = len(h.ents)
	}
	out := h.ents[h.entAt:end]
	h.entAt = end
	return out, nil
}

// Readdirnames is Readdir reduced to names.
func (h *File) Readdirnames(n int) ([]string, error) {
	ents, err := h.Readdir(n)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names, nil
}

// pending describes a file that exists only in this buffer so far.
type pending struct {
	name string
	size int64
	mode os.FileMode
}

func (p *pending) Name() string       { return p.name }
func (p *pending) Size() int64        { return p.size }
func (p *pending) Mode() os.FileMode  { return p.mode }
func (p *pending) ModTime() time.Time { return time.Time{} }
func (p *pending) IsDir() bool        { return false }
func (p *pending) Sys() any           { return nil }

// The compiler is the check that nothing on either interface was missed.
var (
	_ afero.Fs   = (*Fs)(nil)
	_ afero.File = (*File)(nil)
)
