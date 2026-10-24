//go:build js && wasm

package attractor

import "testing"

// TestEveryCubeFaceReadsUpright checks the skin's picture on each face of the
// cube: whole (every corner at a corner of the picture), unmirrored from
// outside (right × up points out of the face), and on the side faces with its
// top at the top. Two of the side faces had it lying on its side.
func TestEveryCubeFaceReadsUpright(t *testing.T) {
	verts, idx := cubeSkinMesh(verticesCube, indicesCube)
	if len(idx) != len(verts)/5/4*6 {
		t.Fatalf("%d indices for %d faces", len(idx), len(verts)/5/4)
	}
	for f := range len(verts) / 20 {
		var at [2][2][3]float32 // corner position by (u, v)
		var center [3]float32
		for k := range 4 {
			v := verts[(f*4+k)*5 : (f*4+k)*5+5]
			u, w := v[3], v[4]
			if (u != 0 && u != 1) || (w != 0 && w != 1) {
				t.Fatalf("face %d corner %d: uv (%v, %v) is not a corner of the picture", f, k, u, w)
			}
			at[int(u)][int(w)] = [3]float32{v[0], v[1], v[2]}
			for a := range 3 {
				center[a] += v[a]
			}
		}
		var eu, ev [3]float32
		for a := range 3 {
			eu[a] = at[1][0][a] - at[0][0][a]
			ev[a] = at[0][1][a] - at[0][0][a]
		}
		out := (eu[1]*ev[2]-eu[2]*ev[1])*center[0] + (eu[2]*ev[0]-eu[0]*ev[2])*center[1] + (eu[0]*ev[1]-eu[1]*ev[0])*center[2]
		if out <= 0 {
			t.Errorf("face %d (center %v): the picture is mirrored seen from outside", f, center)
		}
		side := abs32(center[1]) < abs32(center[0]) || abs32(center[1]) < abs32(center[2])
		if side && ev[1] <= 0 {
			t.Errorf("face %d (center %v): the picture's top is not up (v runs along %v)", f, center, ev)
		}
	}
}
