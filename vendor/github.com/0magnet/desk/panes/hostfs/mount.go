// This file has no build tag, unlike the rest of the package, so the routing
// can be tested natively against two ordinary in-memory filesystems. The
// decision of which filesystem a path belongs to is the part worth being sure
// about, and none of it needs a browser.

package hostfs

import (
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/0magnet/afero"
)

// Mount routes some prefixes to other filesystems.
//
// # Why this exists
//
// websh's shell calls PopulateBin on startup: it creates /bin and writes a stub
// file for every applet, so that `ls /bin` shows the command set. That is
// exactly right for a filesystem that lives in a tab and exactly wrong for the
// host's — pointed at a home directory, the first thing the desk would do is
// scatter fifty empty files into it.
//
// The fix is not to stop the shell doing it. /bin is genuinely part of what the
// shell presents, and losing it would make `ls /bin` and tab completion worse
// for no gain. The fix is that a SYNTHETIC directory should stay synthetic:
// /bin is routed to memory, and everything else is the machine.
//
// Longest prefix wins, and paths are passed through unchanged — a mounted
// filesystem sees /bin/ls, not /ls — so a plain MemMapFs works as a mount with
// no adaptation.
func Mount(base afero.Fs, mounts map[string]afero.Fs) afero.Fs {
	m := &mountFs{base: base, mounts: map[string]afero.Fs{}}
	for prefix, fsys := range mounts {
		m.mounts[path.Clean("/"+prefix)] = fsys
	}
	for prefix := range m.mounts {
		m.prefixes = append(m.prefixes, prefix)
	}
	// Longest first, so /a/b is chosen over /a when both are mounted.
	sort.Slice(m.prefixes, func(i, j int) bool {
		return len(m.prefixes[i]) > len(m.prefixes[j])
	})
	return m
}

type mountFs struct {
	base     afero.Fs
	mounts   map[string]afero.Fs
	prefixes []string
}

// pick returns the filesystem a path belongs to.
func (m *mountFs) pick(name string) afero.Fs {
	clean := path.Clean("/" + strings.ReplaceAll(name, `\`, "/"))
	for _, p := range m.prefixes {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return m.mounts[p]
		}
	}
	return m.base
}

// mountsUnder returns the mount points that are direct children of dir.
func (m *mountFs) mountsUnder(dir string) []string {
	clean := path.Clean("/" + dir)
	var out []string
	for _, p := range m.prefixes {
		if path.Dir(p) == clean && p != clean {
			out = append(out, path.Base(p))
		}
	}
	sort.Strings(out)
	return out
}

func (m *mountFs) Name() string { return "hostfs+mounts" }

func (m *mountFs) Create(name string) (afero.File, error) { return m.wrap(name, afero.Fs.Create) }

func (m *mountFs) Open(name string) (afero.File, error) { return m.wrap(name, afero.Fs.Open) }

func (m *mountFs) wrap(name string, op func(afero.Fs, string) (afero.File, error)) (afero.File, error) {
	f, err := op(m.pick(name), name)
	if err != nil {
		return nil, err
	}
	// A directory that has mount points under it has to report them, or `ls /`
	// would not show /bin and the shell would look like it had lost a
	// directory it can still cd into.
	if extra := m.entriesUnder(name); len(extra) > 0 {
		return &mergedDir{File: f, extra: extra}, nil
	}
	return f, nil
}

// entriesUnder describes the mount points directly under dir.
//
// The mounted filesystem is asked to stat its own root, so a mount shows the
// modification time it actually has. The synthetic fallback is only for a mount
// whose root does not exist yet — otherwise a file manager displays every mount
// point as dated the year 1.
func (m *mountFs) entriesUnder(dir string) []os.FileInfo {
	names := m.mountsUnder(dir)
	if len(names) == 0 {
		return nil
	}
	clean := path.Clean("/" + dir)
	out := make([]os.FileInfo, 0, len(names))
	for _, name := range names {
		full := path.Join(clean, name)
		if fi, err := m.mounts[full].Stat(full); err == nil {
			out = append(out, renamed{FileInfo: fi, name: name})
			continue
		}
		out = append(out, &mountInfo{name: name})
	}
	return out
}

// renamed reports a different Name than the FileInfo it wraps, because a
// directory entry is named relative to its parent while the stat was by path.
type renamed struct {
	os.FileInfo
	name string
}

func (r renamed) Name() string { return r.name }

func (m *mountFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := m.pick(name).OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if extra := m.entriesUnder(name); len(extra) > 0 {
		return &mergedDir{File: f, extra: extra}, nil
	}
	return f, nil
}

func (m *mountFs) Mkdir(name string, perm os.FileMode) error {
	return m.pick(name).Mkdir(name, perm)
}

// MkdirAll is the one operation that can straddle a boundary: creating
// /bin/x/y must not create /bin on the host on its way past. Routing the whole
// call by its final path is what keeps that from happening.
func (m *mountFs) MkdirAll(name string, perm os.FileMode) error {
	return m.pick(name).MkdirAll(name, perm)
}

func (m *mountFs) Remove(name string) error    { return m.pick(name).Remove(name) }
func (m *mountFs) RemoveAll(name string) error { return m.pick(name).RemoveAll(name) }

// Rename across a boundary is refused rather than silently emulated by a copy.
// Moving a file out of the synthetic /bin onto the machine is not a rename, and
// pretending it is would hide which filesystem ended up holding it.
func (m *mountFs) Rename(oldname, newname string) error {
	from, to := m.pick(oldname), m.pick(newname)
	if from != to {
		return &os.LinkError{Op: "rename", Old: oldname, New: newname,
			Err: os.ErrInvalid}
	}
	return from.Rename(oldname, newname)
}

func (m *mountFs) Stat(name string) (os.FileInfo, error) { return m.pick(name).Stat(name) }

func (m *mountFs) Chmod(name string, mode os.FileMode) error {
	return m.pick(name).Chmod(name, mode)
}

func (m *mountFs) Chown(name string, uid, gid int) error {
	return m.pick(name).Chown(name, uid, gid)
}

func (m *mountFs) Chtimes(name string, atime, mtime time.Time) error {
	return m.pick(name).Chtimes(name, atime, mtime)
}

// mergedDir adds mount points to a directory's own entries.
type mergedDir struct {
	afero.File
	extra []os.FileInfo
	done  bool
}

func (d *mergedDir) Readdir(count int) ([]os.FileInfo, error) {
	ents, err := d.File.Readdir(count)
	if err != nil && len(ents) == 0 && !d.done {
		// The underlying directory is exhausted (or unreadable); the mount
		// points are still there to report.
		return d.flush(), nil
	}
	if err != nil {
		return ents, err
	}
	if count <= 0 {
		return append(ents, d.flush()...), nil
	}
	return ents, nil
}

func (d *mergedDir) flush() []os.FileInfo {
	if d.done {
		return nil
	}
	d.done = true
	return d.extra
}

func (d *mergedDir) Readdirnames(n int) ([]string, error) {
	ents, err := d.Readdir(n)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names, nil
}

// mountInfo describes a mount point as seen from its parent.
type mountInfo struct{ name string }

func (i *mountInfo) Name() string       { return i.name }
func (i *mountInfo) Size() int64        { return 0 }
func (i *mountInfo) Mode() os.FileMode  { return os.ModeDir | 0o755 }
func (i *mountInfo) ModTime() time.Time { return time.Time{} }
func (i *mountInfo) IsDir() bool        { return true }
func (i *mountInfo) Sys() any           { return nil }

var _ afero.Fs = (*mountFs)(nil)
