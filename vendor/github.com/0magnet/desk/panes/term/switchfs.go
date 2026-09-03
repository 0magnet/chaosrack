// This file has no build tag, unlike the rest of the package, so the switch
// can be tested natively against ordinary in-memory filesystems. Whether a
// swap reaches a holder that took the filesystem before it happened is the
// part worth being sure about, and none of it needs a browser.

package term

import (
	"os"
	"sync"
	"time"

	"github.com/0magnet/afero"
)

// switchFs is an afero.Fs whose backing can be replaced while it is in use.
//
// Every call is one pointer read away from the real filesystem, which is what
// lets a pane hold this once and still follow a change made later — see FS and
// SwapFS for why anything needs to.
//
// A nil backing answers ErrInvalid rather than panicking. It should not be
// reachable, since FS seeds one and SetFS installs one, but a filesystem that
// crashes the tab is worse than one that returns an error: a wasm panic takes
// the whole page down, not the pane that caused it.
type switchFs struct {
	mu  sync.RWMutex
	cur afero.Fs
}

func (s *switchFs) swap(f afero.Fs) {
	s.mu.Lock()
	s.cur = f
	s.mu.Unlock()
}

func (s *switchFs) get() afero.Fs {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// errNoFS is what every operation gets when there is no backing at all.
var errNoFS = os.ErrInvalid

func (s *switchFs) Name() string {
	if f := s.get(); f != nil {
		return f.Name()
	}
	return "switchfs(empty)"
}

func (s *switchFs) Create(name string) (afero.File, error) {
	f := s.get()
	if f == nil {
		return nil, errNoFS
	}
	return f.Create(name)
}

func (s *switchFs) Open(name string) (afero.File, error) {
	f := s.get()
	if f == nil {
		return nil, errNoFS
	}
	return f.Open(name)
}

func (s *switchFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f := s.get()
	if f == nil {
		return nil, errNoFS
	}
	return f.OpenFile(name, flag, perm)
}

func (s *switchFs) Mkdir(name string, perm os.FileMode) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Mkdir(name, perm)
}

func (s *switchFs) MkdirAll(name string, perm os.FileMode) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.MkdirAll(name, perm)
}

func (s *switchFs) Remove(name string) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Remove(name)
}

func (s *switchFs) RemoveAll(name string) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.RemoveAll(name)
}

func (s *switchFs) Rename(oldname, newname string) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Rename(oldname, newname)
}

func (s *switchFs) Stat(name string) (os.FileInfo, error) {
	f := s.get()
	if f == nil {
		return nil, errNoFS
	}
	return f.Stat(name)
}

func (s *switchFs) Chmod(name string, mode os.FileMode) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Chmod(name, mode)
}

func (s *switchFs) Chown(name string, uid, gid int) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Chown(name, uid, gid)
}

func (s *switchFs) Chtimes(name string, atime, mtime time.Time) error {
	f := s.get()
	if f == nil {
		return errNoFS
	}
	return f.Chtimes(name, atime, mtime)
}

var _ afero.Fs = (*switchFs)(nil)
