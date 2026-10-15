//go:build !js

package uitool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/rackcatalog"
	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racktui"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
		Picture:     pic,
	}
	// The manual as the panel opens, each bay's page: a bank keeps only the
	// pages its model changes (captureBanks).
	cat.Manual = captureManual(c)
	// Every model's bank, and then the controls: the page builds a model's
	// knobs the first time it is chosen, so only after all of them have been
	// is every knob in the registry.
	cat.Banks, cat.BankRegion = captureBanks(c, ctls, cat.Manual)
	if ctls, err = c.Controls(); err != nil {
		fail("catalog: controls: %v", err)
	}
	cat.Controls = ctls
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
	fmt.Printf("catalog: %d modules, %d controls, %d picture items, %d banks -> %s\n", len(mods), len(ctls), len(pic.Items), len(cat.Banks), *catalogOut)
}

// fail reports and stops.
func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	exit(1)
}

// bankModule is the model bank's module, which a model reprograms.
const bankModule = "visual · model"

// bankSettle is how long a page takes to reprogram its bank for a model: it
// builds the model's knobs the first time, and lays the bank out again.
const bankSettle = 800 * time.Millisecond

// settledPicture is the picture once the page has finished building it: a
// model's bank is built the first time it is chosen, and some take longer
// than bankSettle, which caught them half built — a bank with no knobs in
// it. It reads again until two reads in a row place the same controls, for
// at most bankSettleMax.
func settledPicture(c *racklink.Client) (*rackpic.Picture, error) {
	time.Sleep(bankSettle)
	p, err := c.Picture()
	for waited := bankSettle; err == nil && waited < bankSettleMax; waited += bankRecheck {
		time.Sleep(bankRecheck)
		q, qerr := c.Picture()
		if qerr != nil {
			return p, qerr
		}
		if sameCtls(p, q) {
			return q, nil
		}
		p = q
	}
	return p, err
}

// How long settledPicture reads again after, and how long at most it waits.
const (
	bankRecheck   = 300 * time.Millisecond
	bankSettleMax = 6 * time.Second
)

// sameCtls reports whether two pictures place the same controls.
func sameCtls(p, q *rackpic.Picture) bool {
	if len(p.Ctls) != len(q.Ctls) {
		return false
	}
	for id := range p.Ctls {
		if _, ok := q.Ctls[id]; !ok {
			return false
		}
	}
	return true
}

// captureBanks chooses every model on the page in turn and keeps the bank as
// the page draws it for each, and the region they all share: the union of
// every bank control's box. It leaves the page on the model it found.
func captureBanks(c *racklink.Client, ctls []racktui.Control, man [][]manual.Module) (map[string]*rackcatalog.Bank, [4]float64) {
	var sel racktui.Control
	for _, ct := range ctls {
		if ct.IsSelect && strings.HasPrefix(ct.ID, "bay-") && strings.HasSuffix(ct.ID, "-sel-all") {
			sel = ct
		}
	}
	if sel.ID == "" {
		fail("catalog: the page has no model selector in its registry")
	}
	pics := map[string]*rackpic.Picture{}
	mans := map[string]map[int][]manual.Module{}
	for i, model := range sel.Options {
		if err := c.Set(sel.ID, model); err != nil {
			fail("catalog: choosing %q: %v", model, err)
		}
		p, err := settledPicture(c)
		if err != nil {
			fail("catalog: the picture with %q: %v", model, err)
		}
		pics[model] = p
		mans[model] = manualDiff(man, captureManual(c))
		fmt.Printf("\rcatalog: bank %d of %d (%s)          ", i+1, len(sel.Options), model)
	}
	fmt.Println()
	if err := c.Set(sel.ID, sel.Value); err != nil {
		fail("catalog: back to %q: %v", sel.Value, err)
	}

	after, err := c.Controls()
	if err != nil {
		fail("catalog: controls: %v", err)
	}
	// The families' dials are built with their banks, so they are in this
	// registry, after every bank has been up, and may not be in the first.
	captureFamilies(c, after, sel, pics, mans, man)
	if err := c.Set(sel.ID, sel.Value); err != nil {
		fail("catalog: back to %q: %v", sel.Value, err)
	}
	inBank := map[string]bool{}
	for _, ct := range after {
		if ct.Module == bankModule {
			inBank[ct.ID] = true
		}
	}
	var region [4]float64
	first := true
	for _, p := range pics {
		for id, b := range p.Ctls {
			if !inBank[id] {
				continue
			}
			if first {
				region, first = b, false
				continue
			}
			x0, y0 := min(region[0], b[0]), min(region[1], b[1])
			x1, y1 := max(region[0]+region[2], b[0]+b[2]), max(region[1]+region[3], b[1]+b[3])
			region = [4]float64{x0, y0, x1 - x0, y1 - y0}
		}
	}
	if first {
		fail("catalog: no bank control has a box in any picture")
	}
	banks := map[string]*rackcatalog.Bank{}
	for model, p := range pics {
		b := &rackcatalog.Bank{Ctls: map[string][4]float64{}, Manual: mans[model]}
		for _, it := range p.Items {
			if rackcatalog.InBank(it, region) {
				b.Items = append(b.Items, it)
			}
		}
		// Every control drawn in the region, the bank's and the bay's: the
		// model selector's window, which names the model, is there too.
		for id, box := range p.Ctls {
			if inBox(box, region) {
				b.Ctls[id] = box
			}
		}
		banks[model] = b
	}
	return banks, region
}

// inBox reports whether a box's middle is in the region r.
func inBox(b, r [4]float64) bool {
	x, y := b[0]+b[2]/2, b[1]+b[3]/2
	return x >= r[0] && x <= r[0]+r[2] && y >= r[1] && y <= r[1]+r[3]
}

// captureFamilies keeps the bank of every member of a model family (a
// fam-* dial: Sprott's A–S), which the MODEL knob offers as one position: on
// that position, each member the dial lands on is played and its bank put up.
func captureFamilies(c *racklink.Client, ctls []racktui.Control, sel racktui.Control, pics map[string]*rackpic.Picture, mans map[string]map[int][]manual.Module, man [][]manual.Module) {
	for _, fam := range ctls {
		if !fam.IsSelect || !strings.HasPrefix(fam.ID, "fam-") || len(fam.Options) == 0 {
			continue
		}
		// The MODEL knob onto the family's position: it plays the first member.
		if err := c.Set(sel.ID, fam.Options[0]); err != nil {
			fail("catalog: choosing %q: %v", fam.Options[0], err)
		}
		for i, m := range fam.Options {
			if _, have := pics[m]; have {
				continue
			}
			if err := c.Set(fam.ID, m); err != nil {
				fail("catalog: %s to %q: %v", fam.ID, m, err)
			}
			p, err := settledPicture(c)
			if err != nil {
				fail("catalog: the picture with %q: %v", m, err)
			}
			pics[m] = p
			mans[m] = manualDiff(man, captureManual(c))
			fmt.Printf("\rcatalog: %s %d of %d (%s)          ", fam.ID, i+1, len(fam.Options), m)
		}
		if err := c.Set(fam.ID, fam.Options[0]); err != nil {
			fail("catalog: %s back to %q: %v", fam.ID, fam.Options[0], err)
		}
	}
	fmt.Println()
}

// captureManual is every bay's page of the manual as the page reads it off
// the panel now, bay 1 first, to the last bay with anything on it.
func captureManual(c *racklink.Client) [][]manual.Module {
	var pages [][]manual.Module
	for n := 1; n <= 64; n++ {
		pg, err := c.Manual(n)
		if err != nil {
			fail("catalog: bay %d's manual: %v", n, err)
		}
		if len(pg.Mods) == 0 {
			break
		}
		pages = append(pages, pg.Mods)
	}
	return pages
}

// manualDiff is the bays of pages that read otherwise than in base, by bay
// number: what a bank keeps of its manual.
func manualDiff(base, pages [][]manual.Module) map[int][]manual.Module {
	out := map[int][]manual.Module{}
	for i, pg := range pages {
		if i < len(base) && reflect.DeepEqual(base[i], pg) {
			continue
		}
		out[i+1] = pg
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
