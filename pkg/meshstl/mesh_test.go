package meshstl

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackspec"
)

// signedVolume is the divergence-theorem volume of a closed mesh. It is the
// single most useful check on generated geometry: it comes out at the true
// volume when the surface is closed and consistently wound outward, and it
// comes out wrong — often negative — when a face is inside out. Every solid
// below is checked against the volume its formula says it should have.
func signedVolume(m Mesh) float64 {
	var v float64
	for _, t := range m.Tris {
		v += t.A.Dot(t.B.Cross(t.C)) / 6
	}
	return v
}

// surfaceArea sums triangle areas.
func surfaceArea(m Mesh) float64 {
	var a float64
	for _, t := range m.Tris {
		a += t.Area()
	}
	return a
}

// isClosed reports whether every edge is shared by exactly two triangles —
// what a printable mesh needs, and what a sweep gets wrong at its seams.
func isClosed(m Mesh) (bool, int) {
	type edge struct{ a, b [3]float64 }
	count := map[edge]int{}
	key := func(p, q V3) edge {
		// Round, so that two vertices meant to be the same point but arrived
		// at by different arithmetic still match.
		r := func(v V3) [3]float64 {
			return [3]float64{
				math.Round(v[0]*1e6) / 1e6,
				math.Round(v[1]*1e6) / 1e6,
				math.Round(v[2]*1e6) / 1e6,
			}
		}
		a, b := r(p), r(q)
		// Undirected: order the endpoints canonically.
		for i := 0; i < 3; i++ {
			if a[i] != b[i] {
				if a[i] > b[i] {
					a, b = b, a
				}
				break
			}
		}
		return edge{a, b}
	}
	for _, t := range m.Tris {
		count[key(t.A, t.B)]++
		count[key(t.B, t.C)]++
		count[key(t.C, t.A)]++
	}
	bad := 0
	for _, n := range count {
		if n != 2 {
			bad++
		}
	}
	return bad == 0, bad
}

func TestBoxIsAClosedBoxOfTheRightVolume(t *testing.T) {
	m := Box(V3{0, 0, 0}, V3{2, 3, 5})
	if got := signedVolume(m); math.Abs(got-30) > 1e-9 {
		t.Errorf("volume = %g, want 30 (negative means the faces are inside out)", got)
	}
	if ok, bad := isClosed(m); !ok {
		t.Errorf("box is not closed: %d edges are not shared by two triangles", bad)
	}
	if got := surfaceArea(m); math.Abs(got-2*(2*3+2*5+3*5)) > 1e-9 {
		t.Errorf("area = %g, want %g", got, float64(2*(2*3+2*5+3*5)))
	}
}

func TestCylinderVolumeApproachesTheFormula(t *testing.T) {
	const r, h = 3, 7
	const seg = 256
	m := Cylinder(V3{0, 0, 0}, r, h, seg)
	// Checked against the INSCRIBED PRISM, not against πr²h: the mesh is a
	// polygon, and the exact polygon volume is a far tighter test of the code
	// than a circle it is only meant to approximate. (The gap between the two
	// goes as 1/seg² — at 256 segments it is one part in ten thousand, which
	// is what a tolerance against πr²h would really be measuring.)
	want := seg / 2.0 * r * r * math.Sin(2*math.Pi/seg) * h
	if got := signedVolume(m); math.Abs(got-want)/want > 1e-12 {
		t.Errorf("volume = %g, want %g", got, want)
	}
	if circle := math.Pi * r * r * h; math.Abs(want-circle)/circle > 1e-3 {
		t.Errorf("a %d-segment prism is %g, too far from the cylinder %g it stands in for", seg, want, circle)
	}
	if ok, bad := isClosed(m); !ok {
		t.Errorf("cylinder is not closed: %d bad edges", bad)
	}
}

func TestSphereAndTorusVolumes(t *testing.T) {
	const r = 2.0
	s := UVSphere(V3{0, 0, 0}, r, 128, 256)
	want := 4.0 / 3 * math.Pi * r * r * r
	if got := signedVolume(s); math.Abs(got-want)/want > 1e-3 {
		t.Errorf("sphere volume = %g, want ≈ %g", got, want)
	}

	const R, tr = 10.0, 3.0
	to := Torus(V3{0, 0, 0}, R, tr, 256, 128)
	wantT := 2 * math.Pi * math.Pi * R * tr * tr
	if got := signedVolume(to); math.Abs(got-wantT)/wantT > 1e-3 {
		t.Errorf("torus volume = %g, want ≈ %g", got, wantT)
	}
	if ok, bad := isClosed(to); !ok {
		t.Errorf("torus is not closed: %d bad edges", bad)
	}
}

// Each Platonic solid has a closed-form volume for a given circumradius. That
// makes them the strictest available check on the face winding: get one face
// backwards and the volume is visibly wrong.
func TestPlatonicSolids(t *testing.T) {
	const r = 1.0
	phi := (1 + math.Sqrt(5)) / 2
	cases := []struct {
		name string
		mesh Mesh
		vol  float64
	}{
		{"tetrahedron", Tetrahedron(r), 8 * math.Sqrt(3) / 27 * r * r * r},
		{"cube", Hexahedron(r), 8 / (3 * math.Sqrt(3)) * r * r * r},
		{"octahedron", Octahedron(r), 4.0 / 3 * r * r * r},
		{"dodecahedron", Dodecahedron(r), (4.0 / 3) * math.Pi * 0 /* set below */},
		{"icosahedron", Icosahedron(r), 0},
	}
	// Dodecahedron and icosahedron volumes in terms of the circumradius.
	cases[3].vol = 2.785163 * r * r * r // (15+7√5)/4 · a³ with a = 4r/(√3(1+√5))
	cases[4].vol = 2.536150 * r * r * r // (5/12)(3+√5)a³ with a = 4r/√(10+2√5)
	_ = phi

	for _, c := range cases {
		if ok, bad := isClosed(c.mesh); !ok {
			t.Errorf("%s is not closed: %d bad edges", c.name, bad)
		}
		got := signedVolume(c.mesh)
		if got <= 0 {
			t.Errorf("%s has volume %g — the faces are wound inward", c.name, got)
			continue
		}
		if math.Abs(got-c.vol)/c.vol > 1e-4 {
			t.Errorf("%s volume = %g, want ≈ %g", c.name, got, c.vol)
		}
		// Every vertex is on the circumsphere.
		for _, tr := range c.mesh.Tris {
			for _, v := range [3]V3{tr.A, tr.B, tr.C} {
				if math.Abs(v.Len()-r) > 1e-9 {
					t.Fatalf("%s has a vertex at radius %g, want %g", c.name, v.Len(), r)
				}
			}
		}
	}
}

// A tube swept along a straight line is a cylinder, and its volume says so.
// This is the check that the transported frame is not collapsing or twisting.
func TestTubeAlongALineIsACylinder(t *testing.T) {
	path := make([]V3, 0, 101)
	for i := 0; i <= 100; i++ {
		path = append(path, V3{0, 0, float64(i) / 10})
	}
	m := Tube(path, 0.5, 128, true)
	want := math.Pi * 0.25 * 10
	if got := signedVolume(m); math.Abs(got-want)/want > 1e-3 {
		t.Errorf("tube volume = %g, want ≈ %g", got, want)
	}
	if ok, bad := isClosed(m); !ok {
		t.Errorf("capped tube is not closed: %d bad edges", bad)
	}
}

// The failure this guards is specific: a tube framed from a fixed "up" flips
// over where the tangent passes vertical, and the surface pinches. A helix
// that climbs through vertical exercises it; a pinched tube loses volume.
func TestTubeSurvivesAVerticalTangent(t *testing.T) {
	var path []V3
	for i := 0; i <= 400; i++ {
		u := float64(i) / 400 * 4 * math.Pi
		// A curve whose tangent sweeps through +Z and back.
		path = append(path, V3{math.Cos(u), math.Sin(u), math.Sin(u/2) * 4})
	}
	m := Tube(path, 0.05, 24, true)
	if ok, bad := isClosed(m); !ok {
		t.Errorf("tube is not closed: %d bad edges", bad)
	}
	if v := signedVolume(m); v <= 0 {
		t.Errorf("tube volume %g — the surface turned inside out along the path", v)
	}
	for _, tr := range m.Tris {
		if tr.Area() <= 0 || math.IsNaN(tr.Area()) {
			t.Fatal("degenerate triangle in the sweep")
		}
	}
}

// The panel is the on-screen module, in millimeters: same width, same height,
// same tiling. If these drift apart the STL stops being a model of the app.
func TestModulePanelMatchesTheOnScreenModule(t *testing.T) {
	m := ModulePanel(PanelOptions{Body: true})
	s := m.Size()
	if math.Abs(s[0]-rackspec.SlotWidth) > 1e-9 {
		t.Errorf("panel is %.3f mm wide, the module is %.3f mm", s[0], rackspec.SlotWidth)
	}
	if math.Abs(s[1]-rackspec.PanelHeight3U) > 1e-9 {
		t.Errorf("panel is %.3f mm tall, a 3U panel is %.3f mm", s[1], rackspec.PanelHeight3U)
	}
	// The defining property of a card-cage module, and the one the first
	// version got wrong: it is DEEPER THAN IT IS TALL. A 40 mm body is the
	// depth of the potentiometer behind the panel, not of the card it is
	// soldered to.
	wantDepth := PanelThickness + rackspec.PCBDepth
	if math.Abs(s[2]-wantDepth) > 1e-9 {
		t.Errorf("module is %.3f mm deep, want %.3f (panel + Eurocard)", s[2], wantDepth)
	}
	if s[2] <= s[1] {
		t.Errorf("module is %.1f mm deep and %.1f mm tall — a card cage module is deeper than it is tall", s[2], s[1])
	}

	// The board has to protrude past the case, or it cannot reach the
	// backplane connector.
	if rackspec.PCBDepth-rackspec.CaseDepth < 5 {
		t.Errorf("the board stands only %.1f mm proud of the case — nothing to plug in",
			rackspec.PCBDepth-rackspec.CaseDepth)
	}

	// And the board has to fit the opening it rides in.
	if rackspec.PCBHeight3U >= rackspec.PanelHeight3U {
		t.Errorf("a %.1f mm board does not fit behind a %.1f mm panel",
			rackspec.PCBHeight3U, rackspec.PanelHeight3U)
	}

	// Two panels side by side must meet on the whole-HP grid.
	wide := ModulePanel(PanelOptions{HP: 2 * rackspec.ModuleHP})
	if got, want := wide.Size()[0], 2*rackspec.SlotPitch-rackspec.Seam; math.Abs(got-want) > 1e-9 {
		t.Errorf("a two-slot panel is %.3f mm, want %.3f", got, want)
	}
}

// The matrix is on the pitch the spec says, so a printed panel and the screen
// agree about where the pins are.
func TestPinMatrixIsOnTheSpecPitch(t *testing.T) {
	pins := PinMatrix(0, 0, 6, 3)
	if len(pins) != 18 {
		t.Fatalf("%d pins, want 18", len(pins))
	}
	dx := pins[1].X - pins[0].X
	if math.Abs(dx-rackspec.PinPitch) > 1e-9 {
		t.Errorf("pin pitch %.3f mm, spec says %.3f", dx, rackspec.PinPitch)
	}
	dy := pins[0].Y - pins[6].Y
	if math.Abs(dy-rackspec.PinPitch) > 1e-9 {
		t.Errorf("row pitch %.3f mm, spec says %.3f", dy, rackspec.PinPitch)
	}
}

// A binary STL has a fixed layout, and the header must not start with "solid"
// or readers sniff it as ASCII and fail.
func TestBinarySTLLayout(t *testing.T) {
	m := Box(V3{0, 0, 0}, V3{1, 1, 1})
	var buf bytes.Buffer
	if err := WriteBinarySTL(&buf, m, "test"); err != nil {
		t.Fatal(err)
	}
	want := 84 + 50*len(m.Tris)
	if buf.Len() != want {
		t.Errorf("wrote %d bytes, want %d (80 header + 4 count + 50 per triangle)", buf.Len(), want)
	}
	if bytes.HasPrefix(buf.Bytes(), []byte("solid")) {
		t.Error("header begins with \"solid\"; readers will parse the file as ASCII")
	}
	var n uint32
	if err := binary.Read(bytes.NewReader(buf.Bytes()[80:84]), binary.LittleEndian, &n); err != nil {
		t.Fatal(err)
	}
	if int(n) != len(m.Tris) {
		t.Errorf("count says %d triangles, mesh has %d", n, len(m.Tris))
	}
}

// A measured layout can put a control slightly over an edge; the panel it is
// built into must still be a panel.
func TestExactPanelKeepsControlsOnThePanel(t *testing.T) {
	l := PanelLayout{
		ID: "test", HP: rackspec.ModuleHP,
		Controls: []Control{
			{X: -5, Y: 200, Kind: LEDControl, Diam: 3},     // off two edges
			{X: 1e6, Y: -1e6, Kind: KnobControl, Diam: 19}, // far off both
			{X: 17, Y: 64, Kind: KnobControl, Diam: 1000},  // bigger than the panel
		},
	}
	m := ExactPanel(l, false, 12)
	s := m.Size()
	if s[0] > rackspec.SlotWidth+1e-9 {
		t.Errorf("panel is %.2f mm wide, wider than the %.2f mm module", s[0], rackspec.SlotWidth)
	}
	if s[1] > rackspec.PanelHeight3U+1e-9 {
		t.Errorf("panel is %.2f mm tall, taller than a %.1f mm 3U panel", s[1], rackspec.PanelHeight3U)
	}
}

// A label's up-vector decides whether the number can be read. Following the
// offset direction — which points away from the part — draws it upside down.
func TestTextUpIsReadable(t *testing.T) {
	cases := []struct{ right, want V3 }{
		{V3{1, 0, 0}, V3{0, 1, 0}},  // along +X: reads left to right, up is up
		{V3{0, 1, 0}, V3{-1, 0, 0}}, // along +Y: reads bottom to top
		{V3{0, 0, -1}, V3{0, 1, 0}}, // along Z: no rotation to take, stays flat
	}
	for _, c := range cases {
		got := textUp(c.right)
		if math.Abs(got[0]-c.want[0]) > 1e-9 || math.Abs(got[1]-c.want[1]) > 1e-9 || math.Abs(got[2]-c.want[2]) > 1e-9 {
			t.Errorf("textUp(%v) = %v, want %v", c.right, got, c.want)
		}
		if d := got.Dot(c.right); math.Abs(d) > 1e-9 {
			t.Errorf("textUp(%v) is not perpendicular to it (dot %g)", c.right, d)
		}
	}
}

// The annotation has to end up somewhere sane: a dimension of a 100 mm span
// should not be a thousand millimeters of geometry.
func TestDimensionStaysNearWhatItMeasures(t *testing.T) {
	a, b := V3{0, 0, 0}, V3{100, 0, 0}
	m := Dimension(a, b, V3{0, -1, 0}, 10, nil, 6, RodRadius)
	if len(m.Tris) == 0 {
		t.Fatal("no geometry")
	}
	min, max := m.Bounds()
	if min[0] < -10 || max[0] > 110 {
		t.Errorf("x span %.1f..%.1f, well outside the 0..100 it measures", min[0], max[0])
	}
	if max[1] > 1 || min[1] < -30 {
		t.Errorf("y span %.1f..%.1f — the dimension should sit just below the part", min[1], max[1])
	}
}
