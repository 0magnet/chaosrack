package rackhost

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackcatalog"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

func newRack(t *testing.T) *Rack {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Every dial the picture shows has its pointer, and at the catalog's value
// the pointer stands exactly where the page drew it.
func TestEveryDialHasItsPointerWhereThePageDrewIt(t *testing.T) {
	r := newRack(t)
	dials, found := 0, 0
	for _, c := range r.ctls {
		if c.IsSwitch || c.IsSelect || c.Max <= c.Min || isRotationKnob(c.ID) || isPin(c.ID) { // a pin is a lamp
			continue
		}
		if _, shown := r.pic.Ctls[c.ID]; !shown {
			continue
		}
		dials++
		p, ok := r.ptrs[c.ID]
		if !ok {
			// A dial the page draws with no pointer (a ring, a bar) has
			// nothing to turn.
			continue
		}
		found++
		v, _ := strconv.ParseFloat(c.Value, 64) //nolint:errcheck // a dial's value
		if got, want := p.angle(&c, v), *r.pic.Items[p.item].Angle; math.Abs(got-want) > 0.01 {
			t.Errorf("%s = %s: pointer at %.1f°, the page drew it at %.1f°", c.ID, c.Value, got, want)
		}
	}
	if dials == 0 || found*4 < dials*3 {
		t.Fatalf("pointers found for %d of %d dials in the catalog's picture", found, dials)
	}
}

// A value a control cannot take is refused and leaves it as it was.
func TestSetRefusesWhatAControlCannotTake(t *testing.T) {
	r := newRack(t)
	var sel, dial, sw string
	for _, c := range r.ctls {
		switch {
		case c.IsSelect && len(c.Options) > 1 && sel == "":
			sel = c.ID
		case c.IsSwitch && sw == "":
			sw = c.ID
		case !c.IsSelect && !c.IsSwitch && c.Max > c.Min && dial == "":
			dial = c.ID
		}
	}
	for _, bad := range []struct{ id, v string }{{sel, "banana"}, {dial, "abc"}, {sw, "maybe"}, {"no-such", "1"}} {
		before := r.value(bad.id)
		if err := r.Set(bad.id, bad.v); err == nil {
			t.Errorf("%s=%s was taken", bad.id, bad.v)
		}
		if r.value(bad.id) != before {
			t.Errorf("%s changed to %q on a refused value", bad.id, r.value(bad.id))
		}
	}
	c := r.ctls[r.byID[dial]]
	if err := r.Set(dial, strconv.FormatFloat(c.Max*10+100, 'f', -1, 64)); err != nil {
		t.Fatal(err)
	}
	if got := r.number(dial); got != c.Max {
		t.Errorf("a value past the travel left %s at %g, want its top %g", dial, got, c.Max)
	}
}

// Turning a dial turns its pointer, and the change is in the next patch.
func TestTurningADialTurnsItsPointer(t *testing.T) {
	r := newRack(t)
	var id string
	for k := range r.ptrs {
		c := r.ctls[r.byID[k]]
		if v, err := strconv.ParseFloat(c.Value, 64); err == nil && v < c.Max && !isRotationKnob(k) && !isFullTurn(k) {
			id = k
			break
		}
	}
	if id == "" {
		t.Fatal("no dial below the top of its travel")
	}
	c := r.ctls[r.byID[id]]
	p, _ := r.Picture() //nolint:errcheck // never fails
	if err := r.Set(id, strconv.FormatFloat(c.Max, 'f', -1, 64)); err != nil {
		t.Fatal(err)
	}
	d, _ := r.PictureChanges(p.Gen) //nolint:errcheck // never fails
	ptr := r.ptrs[id]
	it, ok := d.Items[ptr.item]
	if !ok {
		t.Fatalf("turning %s changed nothing in the picture", id)
	}
	if got := *it.Angle; got < ptr.at || got > 135 {
		t.Errorf("%s at its top: pointer at %.1f°, want from %.1f° up to 135°", id, got, ptr.at)
	}
}

// The scene draws the model it is set to, and nothing when the rack is off.
func TestTheSceneDrawsTheModel(t *testing.T) {
	r := newRack(t)
	lit := func() int {
		m, err := r.Scene(80, 40, 2)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for i := 0; i+2 < len(m.Px); i += 3 {
			if m.Px[i]|m.Px[i+1]|m.Px[i+2] != 0 {
				n++
			}
		}
		return n
	}
	if id := r.modelControl(); id != "" {
		if err := r.Set(id, "lorenz"); err != nil {
			t.Fatal(err)
		}
	}
	if lit() == 0 {
		t.Fatal("the scene is dark with a model set")
	}
	if err := r.Set("power-sw", "0"); err != nil {
		t.Fatal(err)
	}
	if n := lit(); n != 0 {
		t.Errorf("%d pixels lit with the rack off", n)
	}
}

// A readout writes the catalog's value exactly as the page wrote it, and a
// new value in the same hand.
func TestReadoutsWriteValuesAsThePageDid(t *testing.T) {
	r := newRack(t)
	if len(r.reads) < 20 {
		t.Fatalf("only %d readouts found", len(r.reads))
	}
	for id, ro := range r.reads {
		c := r.ctls[r.byID[id]]
		want := r.pic.Items[ro.item].Text
		if got := ro.show(&c); strings.TrimSpace(got) != strings.TrimSpace(want) && !c.IsSelect {
			t.Errorf("%s = %s: readout %q, the page wrote %q", id, c.Value, got, want)
		}
	}
	id := r.modelControl()
	if id == "" {
		t.Skip("no model selector in the catalog")
	}
	if err := r.Set(id, "lorenz"); err != nil {
		t.Fatal(err)
	}
	if got := r.pic.Items[r.reads[id].item].Text; got != "Lorenz" {
		t.Errorf("the model's window says %q after choosing lorenz", got)
	}
}

func TestParseReadout(t *testing.T) {
	for _, tc := range []struct {
		in   string
		v    float64
		back float64
		out  string
	}{
		{" 000.0", 0, -12.5, "-012.5"},
		{"-23", -23, 4, " 4"},
		{"  +0.000", 0, -1.5, "  -1.500"},
		{"130°", 130, 7, "007°"},
		{"0.60", 0.6, 0.25, "0.25"},
	} {
		v, ro, ok := parseReadout(tc.in)
		if !ok || v != tc.v {
			t.Errorf("%q: %v %v", tc.in, v, ok)
			continue
		}
		c := racktui.Control{Value: strconv.FormatFloat(tc.back, 'f', -1, 64)}
		if got := ro.show(&c); got != tc.out {
			t.Errorf("%q then %g: %q, want %q", tc.in, tc.back, got, tc.out)
		}
	}
}

// Every model the selector offers draws natively: none leaves the scene
// dark as one only a browser draws.
func TestNoModelIsBrowserOnly(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	r := newRack(t)
	id := r.modelControl()
	if id == "" {
		t.Skip("no model selector in the catalog")
	}
	for _, model := range r.ctls[r.byID[id]].Options {
		if err := r.Set(id, model); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Scene(20, 10, 2); errors.Is(err, ErrBrowserOnly) {
			t.Errorf("%s draws only in a browser", model)
		}
	}
}

// A flow's constant is a control of the native rack, and setting it redraws
// the model with it.
func TestAFlowConstantRedrawsTheModel(t *testing.T) {
	r := newRack(t)
	id := r.modelControl()
	if id == "" {
		t.Skip("no model selector in the catalog")
	}
	if err := r.Set(id, "lorenz"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.flow["lorenz-r"]; !ok {
		t.Fatal("lorenz-r is not a control of the native rack")
	}
	frame := func() []byte {
		m, err := r.Scene(60, 30, 2)
		if err != nil {
			t.Fatal(err)
		}
		return m.Px
	}
	if err := r.Set("auto-rotate", "0"); err != nil {
		t.Fatal(err)
	}
	before := frame()
	defer func(v string) { _ = r.Set("lorenz-r", v) }(r.value("lorenz-r")) //nolint:errcheck // restoring a package-wide constant
	if err := r.Set("lorenz-r", "60"); err != nil {
		t.Fatal(err)
	}
	if string(frame()) == string(before) {
		t.Error("the model looks the same with ρ = 60")
	}
}

// The globe is drawn with as many circles as its knobs count.
func TestTheGlobeFollowsItsKnobs(t *testing.T) {
	r := newRack(t)
	id := r.modelControl()
	if id == "" || r.value("globe-lat") == "" {
		t.Skip("no globe knobs in the catalog")
	}
	if err := r.Set(id, "globe"); err != nil {
		t.Fatal(err)
	}
	edges := func() int {
		if _, err := r.Scene(40, 20, 2); err != nil {
			t.Fatal(err)
		}
		return len(r.scene.fig.Edges)
	}
	at18 := edges()
	if err := r.Set("globe-lat", "4"); err != nil {
		t.Fatal(err)
	}
	if at4 := edges(); at4 >= at18 {
		t.Errorf("4 circles of latitude drew %d edges, 18 drew %d", at4, at18)
	}
}

// Choosing a model draws the bank as the page draws it for that model: its
// knobs in the picture, the page's own name for it in the selector's window,
// and the whole picture again for a panel holding the last one. A knob turned
// before keeps its value through the change and back.
func TestChoosingAModelRedrawsTheBank(t *testing.T) {
	r := newRack(t)
	cat, err := rackcatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Banks == nil || r.selector == "" {
		t.Skip("a catalog without banks")
	}
	if err := r.Set("globe-lat", "4"); err != nil {
		t.Fatal(err)
	}
	p, _ := r.Picture() //nolint:errcheck // never fails
	if err := r.Set(r.selector, "lorenz"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.pic.Ctls["lorenz-r"]; !ok {
		t.Error("lorenz's ρ is not in the picture with lorenz chosen")
	}
	if _, ok := r.pic.Ctls["globe-lat"]; ok {
		t.Error("the globe's knobs are still in the picture with lorenz chosen")
	}
	// The bank's knobs carry no pointer the picture records; their readouts
	// are what shows a value.
	if _, ok := r.reads["lorenz-r"]; !ok {
		t.Error("lorenz's ρ has no readout")
	}
	named := false
	for _, it := range r.pic.Items[r.outside:] {
		if it != nil && strings.Contains(strings.ToLower(it.Text), "lorenz") {
			named = true
		}
	}
	if !named {
		t.Error("nothing in the bank names lorenz")
	}
	if d, _ := r.PictureChanges(p.Gen); d.Full == nil { //nolint:errcheck // never fails
		t.Error("a panel with the globe's picture was not sent the whole of lorenz's")
	}
	if err := r.Set(r.selector, "globe"); err != nil {
		t.Fatal(err)
	}
	ro, ok := r.reads["globe-lat"]
	if !ok {
		t.Fatal("the globe's latitude has no readout")
	}
	if got := strings.TrimSpace(r.pic.Items[ro.item].Text); !strings.HasPrefix(got, "4") && !strings.HasPrefix(got, "04") && !strings.HasPrefix(got, "004") {
		t.Errorf("back on the globe, its latitude reads %q, want 4", got)
	}
}

// Layers · Colors and Display act on the drawing: a colormap colors it
// differently and Dots draws the points unjoined.
func TestColorsAndDotsChangeTheDrawing(t *testing.T) {
	r := newRack(t)
	if r.selector == "" {
		t.Skip("no model selector in the catalog")
	}
	for _, kv := range [][2]string{{r.selector, "lorenz"}, {"auto-rotate", "0"}} {
		if err := r.Set(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	frame := func() string {
		m, err := r.Scene(60, 30, 2)
		if err != nil {
			t.Fatal(err)
		}
		return string(m.Px)
	}
	base := frame()
	for _, kv := range [][2]string{{"gradient-colors", "9"}, {"use-points", "1"}} {
		before := frame()
		if err := r.Set(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
		if frame() == before {
			t.Errorf("%s = %s changed nothing in the drawing", kv[0], kv[1])
		}
	}
	if frame() == base {
		t.Error("the drawing is as it started")
	}
	// MAP names the colormap as the page does, not by its number.
	if ro, ok := r.reads["gradient-colors"]; !ok {
		t.Error("MAP has no readout")
	} else if got := strings.TrimSpace(r.pic.Items[ro.item].Text); got != "viridis" {
		t.Errorf("MAP reads %q, want viridis", got)
	}
}

// The picture is the panel once: the items outside the bank and the chosen
// model's bank, and no copy of the catalog's under them, which would leave
// one model's bank showing under the next and a lamp lit under a dark copy.
func TestThePictureHoldsEachItemOnce(t *testing.T) {
	r := newRack(t)
	cat, err := rackcatalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Banks == nil || r.selector == "" {
		t.Skip("a catalog without banks")
	}
	for _, model := range []string{r.value(r.selector), "lorenz", "globe"} {
		if err := r.Set(r.selector, model); err != nil {
			t.Fatal(err)
		}
		if got, want := len(r.pic.Items), r.outside+len(cat.Banks[model].Items); got != want {
			t.Errorf("with %s the picture has %d items, want %d", model, got, want)
		}
	}
}

// Every flow's and map's constants are the native rack's to turn: a knob
// moved redraws the model with it.
func TestEveryConstantTurnsItsModel(t *testing.T) {
	for _, k := range []struct{ model, id, to string }{
		{"lu", "lu-a", "30"},
		{"newtonleipnik", "nl-a", "0.3"},
		{"hyperrossler", "hyperrossler-a", "0.2"},
		{"henon", "henon-a", "1.2"},
		{"clifford", "clifford-a", "-1"},
		{"sprotta", "sprotta-dt", "0.02"},
	} {
		r, c := idleRack(t, k.model)
		p, ok := r.flow[k.id]
		_ = c
		if !ok {
			t.Errorf("%s is not a constant the native rack turns", k.id)
			continue
		}
		looks(t, r, c, 0.1)
		figure := func() [3]float64 {
			frame(t, r)
			r.scene.mu.Lock()
			defer r.scene.mu.Unlock()
			ps := r.scene.fig.Points
			return ps[len(ps)/2]
		}
		before := figure()
		orig := *p.Value
		set(t, r, k.id, k.to)
		after := figure()
		*p.Value = orig // the integrator's, shared by every rack in the process
		if before == after {
			t.Errorf("%s at %s drew the same %s", k.id, k.to, k.model)
		}
	}
}

// The Sprott family's lettered dial plays the member it is on, with that
// member's bank, from the MODEL knob's family position.
func TestTheSprottDialPlaysItsMember(t *testing.T) {
	r, c := idleRack(t, "sprotta")
	set(t, r, "fam-sprott", "sprottc")
	r.mu.Lock()
	m, bank := r.model(), r.inBank["sprottc-dt"]
	r.mu.Unlock()
	if m != "sprottc" || !bank {
		t.Fatalf("the dial on C plays %q, its dt in the bank %v", m, bank)
	}
	looks(t, r, c, 0.1)
	if _, err := r.Scene(40, 20, 1); err != nil {
		t.Fatalf("Sprott C: %v", err)
	}
	p := r.flow["sprottc-dt"]
	orig := *p.Value
	frame(t, r)
	r.scene.mu.Lock()
	before := r.scene.fig.Points[len(r.scene.fig.Points)/2]
	r.scene.mu.Unlock()
	set(t, r, "sprottc-dt", "0.03")
	frame(t, r)
	r.scene.mu.Lock()
	after := r.scene.fig.Points[len(r.scene.fig.Points)/2]
	r.scene.mu.Unlock()
	*p.Value = orig
	if before == after {
		t.Error("Sprott C's dt did not change it")
	}
}

// A palette knob's hue ring points at its hue, all the way round, and its
// level knob, on the same middle, keeps a pointer of its own.
func TestAHueRingPointsAtItsHue(t *testing.T) {
	r := newRack(t)
	hue, lvl := r.ptrs["ck-color-top-hue"], r.ptrs["ck-color-top-lvl"]
	if hue.item == lvl.item {
		t.Fatal("the hue ring and the level knob share one pointer")
	}
	c := r.ctls[r.byID["ck-color-top-hue"]]
	for v, want := range map[float64]float64{240: 240, 300: 300, 360: 0, 30: 30} {
		if got := hue.angle(&c, v); math.Abs(got-want) > 0.01 {
			t.Errorf("hue %.0f: pointer at %.1f°, want %.1f°", v, got, want)
		}
	}
}

// The switches hidden in the Display and Layers banks are where the buttons
// that set them are, so the panel can press them: LINE's S and G, POINTS' +,
// TRAIL's + and −, BEHIND's F and B, I and H.
func TestHiddenSwitchesSitOnTheirButtons(t *testing.T) {
	r := newRack(t)
	for _, id := range []string{"sect-sw", "scope-grat", "use-points", "persist-trail", "ring-sw", "spect-fill", "edit-back", "gradient-reverse", "color-lock-sw"} {
		if _, ok := r.pic.Ctls[id]; !ok {
			t.Errorf("%s has no place on the panel", id)
		}
	}
}
