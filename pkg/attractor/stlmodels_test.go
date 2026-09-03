package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/stlview"
)

// Every built-in must survive the round trip it exists for: built as a mesh,
// encoded as STL, and parsed back by the SAME loader the app's STL mode uses.
// A model that only the generator can read is not a model the app can show.
func TestEveryBuiltInLoadsInTheViewer(t *testing.T) {
	models := STLModels()
	if len(models) < 10 {
		t.Fatalf("only %d built-ins", len(models))
	}
	for _, m := range models {
		if m.Name == "" || m.Label == "" || m.Group == "" {
			t.Errorf("%q: incomplete entry", m.Name)
		}
		b, err := m.Bytes(0)
		if err != nil {
			t.Errorf("%s: encoding: %v", m.Name, err)
			continue
		}
		s, err := stlview.NewSTL(b)
		if err != nil {
			t.Errorf("%s: the app's own loader rejects it: %v", m.Name, err)
			continue
		}
		v, _, idx := s.GetModel()
		if len(v) == 0 || len(idx) == 0 {
			t.Errorf("%s: loaded empty", m.Name)
		}
	}
}

// Names are file stems and picker keys; duplicates would silently overwrite.
func TestBuiltInNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range STLModels() {
		if seen[m.Name] {
			t.Errorf("duplicate built-in name %q", m.Name)
		}
		seen[m.Name] = true
	}
}

// The viewer indexes with 16-bit indices, so a built-in that needs more than
// 65535 vertices gets decimated — which is fine, but it should be a decision
// rather than a surprise. This pins the size the attractors are generated at.
func TestAttractorModelsFitTheIndexPipeline(t *testing.T) {
	for _, m := range STLModels() {
		if m.Group != "Attractors" {
			continue
		}
		mesh := m.Build(0)
		if got := len(mesh.Tris); got > 70000 {
			t.Errorf("%s: %d triangles — too heavy for the viewer to show undecimated", m.Name, got)
		}
	}
}
