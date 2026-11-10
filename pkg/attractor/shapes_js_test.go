//go:build js && wasm

package attractor

import "testing"

// GlobeMesh converts every index to uint16 and says of each conversion that
// it is bounded by the knobs' caps. This is the check that it is: the globe
// at the most its own knobs can ask for, both ways the parallels are drawn.
//
// The caps are read from the table the knobs are built from, so a knob
// widened there is tested at its new reach. The check that stood in for
// this one pinned 60 and 90, the ranges of a prototype's knobs, while the
// rack's went to 90 and 180.
func TestTheGlobeAtItsKnobsCapsFitsAUint16Index(t *testing.T) {
	var lat, lon int
	for _, p := range attractorParams["globe"] {
		switch p.ID {
		case "globe-lat":
			lat = int(p.Max)
		case "globe-lon":
			lon = int(p.Max)
		}
	}
	if lat == 0 || lon == 0 {
		t.Fatalf("the globe's lat and lon knobs are not in the table (lat %d, lon %d)", lat, lon)
	}
	for _, spiral := range []bool{false, true} {
		verts, idx := GlobeMesh(lat, lon, spiral, false, 8, nil, nil)
		n := len(verts) / 3
		if n > 1<<16 {
			t.Fatalf("lat %d lon %d spiral %v: %d vertices, past what a uint16 index reaches", lat, lon, spiral, n)
		}
		// Each segment joins a vertex to the one after it. An index that
		// wrapped would join two that are far apart.
		for i := 0; i+1 < len(idx); i += 2 {
			if idx[i+1] != idx[i]+1 || int(idx[i+1]) >= n {
				t.Fatalf("lat %d lon %d spiral %v: segment %d joins %d to %d of %d vertices", lat, lon, spiral, i/2, idx[i], idx[i+1], n)
			}
		}
	}
}
