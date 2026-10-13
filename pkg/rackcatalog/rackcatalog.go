// Package rackcatalog is the rack as a page last laid it out: its modules
// and their widths, its controls with their defaults, and the picture the
// panel's page look draws.
//
// A front end without a browser needs all three and can make none of them.
// The panel is built by the page, in DOM, and a module's width is measured
// from it, so what the page shows is recorded here instead: captured from a
// fresh page by `chaosrack uitool catalog` and committed, as the embedded
// wasm builds are. The native rack (pkg/rackhost) serves it with values of
// its own.
package rackcatalog

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

//go:embed catalog.json.gz
var data []byte

// Catalog is one capture of the rack.
type Catalog struct {
	// From says which page it was captured from: the server and its build.
	From string `json:"from"`
	// Modules is every module with its width in slots and its bay, and
	// SlotsPerRow how many slots a row of the rack has.
	Modules     []racksurface.Item `json:"modules"`
	SlotsPerRow int                `json:"slotsPerRow"`
	// Controls is the whole surface with the values it opens with.
	Controls []racktui.Control `json:"controls"`
	// Picture is the panel as the page draws it, with those values.
	Picture *rackpic.Picture `json:"picture"`
}

var embedded = sync.OnceValues(func() (*Catalog, error) {
	return Decode(bytes.NewReader(data))
})

// Load is the catalog built into this binary. It is shared: a caller that
// changes it changes it for every other.
func Load() (*Catalog, error) { return embedded() }

// Decode reads a catalog written by Encode.
func Decode(r io.Reader) (*Catalog, error) {
	z, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer z.Close() //nolint:errcheck // a reader; the decode error is the one that matters
	var c Catalog
	if err := json.NewDecoder(z).Decode(&c); err != nil {
		return nil, err
	}
	if c.Picture == nil || len(c.Controls) == 0 || len(c.Modules) == 0 {
		return nil, errors.New("the rack catalog is empty: capture one with `chaosrack uitool catalog`")
	}
	return &c, nil
}

// Encode writes the catalog compressed.
func (c *Catalog) Encode(w io.Writer) error {
	z, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(z).Encode(c); err != nil {
		return err
	}
	return z.Close()
}
