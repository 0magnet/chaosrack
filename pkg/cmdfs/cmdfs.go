// Package cmdfs is the filesystem a chaosrack command reads and writes: this
// machine's on a host, and in the page the shell's, where a file named at its
// prompt is expected to land. The page's os package has no filesystem at all.
package cmdfs

import (
	"context"
	"os"
	"path/filepath"

	"github.com/0magnet/afero"
)

type key struct{}

type place struct {
	fs  afero.Fs
	dir string
}

// With is ctx for commands that use fsys, with relative names taken from dir.
func With(ctx context.Context, fsys afero.Fs, dir string) context.Context {
	return context.WithValue(ctx, key{}, place{fsys, dir})
}

// at is the filesystem under ctx and name as it is found there.
func at(ctx context.Context, name string) (afero.Fs, string) {
	if ctx != nil {
		if p, ok := ctx.Value(key{}).(place); ok {
			if p.dir != "" && !filepath.IsAbs(name) {
				name = filepath.Join(p.dir, name)
			}
			return p.fs, name
		}
	}
	return afero.NewOsFs(), name
}

// Create creates or truncates name for writing.
func Create(ctx context.Context, name string) (afero.File, error) {
	fsys, name := at(ctx, name)
	return fsys.Create(name)
}

// MkdirAll makes name and any parents it lacks.
func MkdirAll(ctx context.Context, name string, perm os.FileMode) error {
	fsys, name := at(ctx, name)
	return fsys.MkdirAll(name, perm)
}

// ReadFile reads all of name.
func ReadFile(ctx context.Context, name string) ([]byte, error) {
	fsys, name := at(ctx, name)
	return afero.ReadFile(fsys, name)
}

// WriteFile writes data to name, creating it if need be.
func WriteFile(ctx context.Context, name string, data []byte, perm os.FileMode) error {
	fsys, name := at(ctx, name)
	return afero.WriteFile(fsys, name, data, perm)
}
