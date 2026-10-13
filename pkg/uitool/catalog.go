//go:build !js

package uitool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0magnet/chaosrack/pkg/rackcatalog"
	"github.com/0magnet/chaosrack/pkg/racklink"
)

var (
	catalogServer = fs.String("catalog-server", "http://127.0.0.1:8080", "catalog: the chaosrack server whose page to capture")
	catalogOut    = fs.String("catalog-out", "pkg/rackcatalog/catalog.json.gz", "catalog: where to write the capture")
)

// runCatalog captures the rack from a page — its modules, its controls and
// the picture of its panel — into the catalog the native rack serves.
//
// Capture from a FRESH page: one in an origin that has never run chaosrack
// (http://127.0.0.2:8080/ beside a usual 127.0.0.1), with no #hash, since
// what the page opens with becomes every control's default, and a page
// carries its last session's settings in its origin's storage.
func runCatalog() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c, err := racklink.Dial(ctx, *catalogServer, racklink.ViaAuto)
	if err != nil {
		fail("catalog: reaching the rack at %s: %v (is its page open?)", *catalogServer, err)
	}
	defer c.Close() //nolint:errcheck // on the way out
	mods, perRow, err := c.Modules()
	if err != nil {
		fail("catalog: modules: %v", err)
	}
	ctls, err := c.Controls()
	if err != nil {
		fail("catalog: controls: %v", err)
	}
	pic, err := c.Picture()
	if err != nil {
		fail("catalog: picture: %v", err)
	}
	cat := &rackcatalog.Catalog{
		From:        *catalogServer + " at " + time.Now().UTC().Format(time.RFC3339),
		Modules:     mods,
		SlotsPerRow: perRow,
		Controls:    ctls,
		Picture:     pic,
	}
	if err := os.MkdirAll(filepath.Dir(*catalogOut), 0o750); err != nil {
		fail("catalog: %v", err)
	}
	f, err := os.Create(*catalogOut)
	if err != nil {
		fail("catalog: %v", err)
	}
	if err := cat.Encode(f); err != nil {
		f.Close() //nolint:errcheck,gosec // the encode error is the one to report
		fail("catalog: writing %s: %v", *catalogOut, err)
	}
	if err := f.Close(); err != nil {
		fail("catalog: %v", err)
	}
	fmt.Printf("catalog: %d modules, %d controls, %d picture items -> %s\n", len(mods), len(ctls), len(pic.Items), *catalogOut)
}

// fail reports and stops.
func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	exit(1)
}
