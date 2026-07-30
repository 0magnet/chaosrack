//go:build js && wasm

package attractor

import (
	"math"
	"sort"
	"syscall/js"
)

// Spectrogram "skin": paint the live spectrogram texture onto a surface
// model instead of its gradient wireframe. Enabled by a checkbox; applies
// to the parametric surfaces (sphere, globe, torus) whose UV mapping is
// natural — u = time (wraps around, scrolling), v = frequency. Flat-faced
// models (cube, polyhedra) would need per-face UV unwrapping and are left
// for later.
//
// The skinned model is a filled, UV-mapped triangle mesh drawn through
// texProgram, so it rotates/zooms/auto-rotates via the normal render path.

var (
	spectroSkin  bool // skin checkbox state
	skinDirty    = true
	skinVBuf     js.Value
	skinIBuf     js.Value
	skinIdxCount int
)

// isSkinnable reports whether the spectrogram skin can be applied to a mode.
func isSkinnable(mode string) bool {
	switch mode {
	case "sphere", "globe", "torus",
		"tetrahedron", "cube", "octahedron", "dodecahedron", "icosahedron", "nestedcube":
		return true
	}
	return false
}

// renderSkinnedMode keeps the spectrogram texture current and draws the
// current surface model as a filled, textured mesh. Called from
// generateForMode when the skin is on and the mode is skinnable.
func renderSkinnedMode(mode string, nowMs float64) {
	if !spectReady {
		initSpectrogram()
	}
	ensureAudioSource()
	updateSpectrogramTexture(nowMs)

	if skinDirty || skinVBuf.IsUndefined() {
		buildSkinMesh(mode)
		skinDirty = false
	}
	offset := float32(spectTexCol) / float32(spectTexW)
	drawTexturedMesh(skinVBuf, skinIBuf, skinIdxCount, spectTexture, offset)
	maybeShowAudioStatus()
}

// buildSkinMesh (re)generates and uploads the interleaved pos+uv vertex
// buffer and triangle index buffer for the current mode's surface.
func buildSkinMesh(mode string) {
	var verts []float32
	var idx []uint16
	switch mode {
	case "torus":
		verts, idx = torusSkinMesh(torusR, torusr, int(torusStacksF), int(torusSlicesF))
	case "globe":
		verts, idx = sphereSkinMesh(1.0, int(globeLatF)*2, int(globeLonF))
	case "cube":
		verts, idx = cubeSkinMesh(verticesCube[:72], indicesCube[:36])
	case "nestedcube":
		verts, idx = cubeSkinMesh(verticesCube, indicesCube)
	case "tetrahedron":
		verts, idx = polySkinMesh(tetrahedronVertices())
	case "octahedron":
		verts, idx = polySkinMesh(octahedronVertices())
	case "dodecahedron":
		verts, idx = polySkinMesh(dodecahedronVertices())
	case "icosahedron":
		verts, idx = polySkinMesh(icosahedronVertices())
	default: // sphere
		verts, idx = sphereSkinMesh(sphereRadius, int(sphereStacksF), int(sphereSlicesF))
	}
	if skinVBuf.IsUndefined() {
		skinVBuf = gl.Call("createBuffer")
	}
	if skinIBuf.IsUndefined() {
		skinIBuf = gl.Call("createBuffer")
	}
	gl.Call("bindBuffer", glTypes.ArrayBuffer, skinVBuf)
	gl.Call("bufferData", glTypes.ArrayBuffer, SliceToTypedArray(verts), glTypes.StaticDraw)
	gl.Call("bindBuffer", glTypes.ElementArrayBuffer, skinIBuf)
	gl.Call("bufferData", glTypes.ElementArrayBuffer, SliceToTypedArray(idx), glTypes.StaticDraw)
	skinIdxCount = len(idx)
}

// gridTriangles emits two triangles per (stacks x slices) grid quad for a
// vertex layout of (slices+1) columns per row.
func gridTriangles(stacks, slices int) []uint16 {
	idx := make([]uint16, 0, stacks*slices*6)
	row := slices + 1
	for i := 0; i < stacks; i++ {
		for j := 0; j < slices; j++ {
			a := uint16(i*row + j)
			b := a + 1
			c := uint16((i+1)*row + j)
			d := c + 1
			idx = append(idx, a, b, c, b, d, c)
		}
	}
	return idx
}

// sphereSkinMesh returns interleaved pos(xyz)+uv verts and triangle indices
// for a UV sphere. u = longitude (wraps, time axis), v = latitude
// (frequency axis, 0 Hz at the south pole so it matches the plane).
func sphereSkinMesh(radius float32, stacks, slices int) ([]float32, []uint16) {
	verts := make([]float32, 0, (stacks+1)*(slices+1)*5)
	for i := 0; i <= stacks; i++ {
		phi := float64(i) * math.Pi / float64(stacks)
		v := 1.0 - float32(i)/float32(stacks) // i=0 (north pole) → v=1 (high freq)
		for j := 0; j <= slices; j++ {
			theta := float64(j) * 2.0 * math.Pi / float64(slices)
			x := radius * float32(math.Sin(phi)*math.Cos(theta))
			y := radius * float32(math.Sin(phi)*math.Sin(theta))
			z := radius * float32(math.Cos(phi))
			u := float32(j) / float32(slices)
			verts = append(verts, x, y, z, u, v)
		}
	}
	return verts, gridTriangles(stacks, slices)
}

// torusSkinMesh returns interleaved pos+uv verts and triangle indices for a
// torus. u = around the main ring (time, wraps), v = around the tube.
func torusSkinMesh(R, r float32, stacks, slices int) ([]float32, []uint16) {
	verts := make([]float32, 0, (stacks+1)*(slices+1)*5)
	for i := 0; i <= stacks; i++ {
		theta := float64(i) * 2.0 * math.Pi / float64(stacks)
		u := float32(i) / float32(stacks)
		for j := 0; j <= slices; j++ {
			phi := float64(j) * 2.0 * math.Pi / float64(slices)
			x := (float64(R) + float64(r)*math.Cos(phi)) * math.Cos(theta)
			y := (float64(R) + float64(r)*math.Cos(phi)) * math.Sin(theta)
			z := float64(r) * math.Sin(phi)
			v := float32(j) / float32(slices)
			verts = append(verts, float32(x), float32(y), float32(z), u, v)
		}
	}
	return verts, gridTriangles(stacks, slices)
}

// cubeSkinMesh maps the full spectrogram onto each face of a cube whose
// vertices already come in per-face quads of 4 (BL,BR,TR,TL) with matching
// triangle indices — so we just tag each vertex with its quad-corner UV and
// reuse the existing indices.
func cubeSkinMesh(cubeVerts []float32, _ []uint16) ([]float32, []uint16) {
	quadUV := [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	n := len(cubeVerts) / 3
	out := make([]float32, 0, n*5)
	for i := 0; i < n; i++ {
		uv := quadUV[i%4]
		out = append(out, cubeVerts[i*3], cubeVerts[i*3+1], cubeVerts[i*3+2], uv[0], uv[1])
	}
	// Generate FILL triangulation from the 4-verts-per-face layout: (0,1,2)
	// + (0,2,3) tiles each quad. The cube's own index table is the WIREFRAME
	// pattern (0,1,2, 1,2,3) — fine for lines, where edges overdraw, but as
	// filled triangles abc+bcd don't tile the quad: the corner triangle at
	// vertex d is left uncovered (an unskinned triangular hole on every face).
	idx := make([]uint16, 0, (n/4)*6)
	for f := 0; f < n/4; f++ {
		b := uint16(f * 4)
		idx = append(idx, b, b+1, b+2, b, b+2, b+3)
	}
	return out, idx
}

// polySkinMesh wraps the spectrogram onto a convex polyhedron given only its
// vertices. Faces are recovered as convex-hull supporting planes, then each
// face is fan-triangulated. UVs use a spherical projection (u = longitude /
// time, v = latitude / frequency) matching the sphere skin, with a per-
// triangle seam fix so faces spanning the u-wrap don't smear. Vertices are
// duplicated per triangle (non-indexed soup) to keep UVs independent.
func polySkinMesh(verts []float32) ([]float32, []uint16) {
	faces := convexFaces(verts)
	out := make([]float32, 0, 256)
	var idx []uint16
	emit := func(vi int, u, v float32) {
		out = append(out, verts[vi*3], verts[vi*3+1], verts[vi*3+2], u, v)
		idx = append(idx, uint16(len(idx)))
	}
	for _, face := range faces {
		for t := 1; t < len(face)-1; t++ {
			tri := [3]int{face[0], face[t], face[t+1]}
			var us, vs [3]float32
			for a, vi := range tri {
				us[a], vs[a] = sphericalUV(verts[vi*3], verts[vi*3+1], verts[vi*3+2])
			}
			// Seam fix: if the triangle straddles the u=0/1 wrap, lift the
			// low-u corners by 1 so interpolation stays local.
			mn, mx := us[0], us[0]
			for _, u := range us {
				if u < mn {
					mn = u
				}
				if u > mx {
					mx = u
				}
			}
			if mx-mn > 0.5 {
				for a := range us {
					if us[a] < 0.5 {
						us[a] += 1
					}
				}
			}
			for a, vi := range tri {
				emit(vi, us[a], vs[a])
			}
		}
	}
	return out, idx
}

// sphericalUV projects a point onto the unit sphere and returns texture
// coordinates: u from longitude (atan2), v from latitude (z axis pole),
// matching sphereSkinMesh so all skins share one mapping convention.
func sphericalUV(x, y, z float32) (float32, float32) {
	r := math.Sqrt(float64(x*x + y*y + z*z))
	if r == 0 {
		return 0, 0
	}
	phi := math.Acos(math.Max(-1, math.Min(1, float64(z)/r))) // 0 at +z pole
	v := 1 - float32(phi/math.Pi)
	u := float32(math.Atan2(float64(y), float64(x))/(2*math.Pi)) + 0.5
	return u, v
}

// convexFaces recovers the faces of a convex polyhedron (centered near the
// origin) from its vertices alone. For every vertex triple it forms the
// candidate plane, keeps it only if all vertices lie on one side (a
// supporting hull plane), dedupes coplanar triples, then collects and
// angularly orders every vertex on that plane. Returns each face as an
// ordered vertex-index ring. O(n^4) but n is tiny (<=20).
func convexFaces(verts []float32) [][]int {
	n := len(verts) / 3
	px := func(i int) (float32, float32, float32) { return verts[i*3], verts[i*3+1], verts[i*3+2] }
	const eps = 1e-4
	var faces [][]int
	seen := map[[4]int]bool{}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			for k := j + 1; k < n; k++ {
				ax, ay, az := px(i)
				bx, by, bz := px(j)
				cx, cy, cz := px(k)
				ux, uy, uz := bx-ax, by-ay, bz-az
				wx, wy, wz := cx-ax, cy-ay, cz-az
				nx := uy*wz - uz*wy
				ny := uz*wx - ux*wz
				nz := ux*wy - uy*wx
				ln := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
				if ln < eps {
					continue
				}
				nx, ny, nz = nx/ln, ny/ln, nz/ln
				d := nx*ax + ny*ay + nz*az
				if d < 0 { // orient outward (origin is inside)
					nx, ny, nz, d = -nx, -ny, -nz, -d
				}
				supporting := true
				for m := 0; m < n; m++ {
					mx, my, mz := px(m)
					if nx*mx+ny*my+nz*mz > d+eps {
						supporting = false
						break
					}
				}
				if !supporting {
					continue
				}
				key := [4]int{int(nx * 1000), int(ny * 1000), int(nz * 1000), int(d * 1000)}
				if seen[key] {
					continue
				}
				seen[key] = true
				var face []int
				for m := 0; m < n; m++ {
					mx, my, mz := px(m)
					if float32(math.Abs(float64(nx*mx+ny*my+nz*mz-d))) < eps {
						face = append(face, m)
					}
				}
				if len(face) >= 3 {
					orderFaceRing(verts, face, nx, ny, nz)
					faces = append(faces, face)
				}
			}
		}
	}
	return faces
}

// orderFaceRing sorts a face's vertex indices into a consistent ring around
// their centroid, using an in-plane basis derived from the face normal.
func orderFaceRing(verts []float32, face []int, nx, ny, nz float32) {
	var cx, cy, cz float32
	for _, vi := range face {
		cx += verts[vi*3]
		cy += verts[vi*3+1]
		cz += verts[vi*3+2]
	}
	inv := 1.0 / float32(len(face))
	cx, cy, cz = cx*inv, cy*inv, cz*inv
	// in-plane basis: e1 from centroid to first vertex, e2 = n x e1
	e1x, e1y, e1z := verts[face[0]*3]-cx, verts[face[0]*3+1]-cy, verts[face[0]*3+2]-cz
	l := float32(math.Sqrt(float64(e1x*e1x + e1y*e1y + e1z*e1z)))
	if l > 0 {
		e1x, e1y, e1z = e1x/l, e1y/l, e1z/l
	}
	e2x, e2y, e2z := ny*e1z-nz*e1y, nz*e1x-nx*e1z, nx*e1y-ny*e1x
	angle := func(vi int) float64 {
		dx, dy, dz := verts[vi*3]-cx, verts[vi*3+1]-cy, verts[vi*3+2]-cz
		return math.Atan2(float64(dx*e2x+dy*e2y+dz*e2z), float64(dx*e1x+dy*e1y+dz*e1z))
	}
	sort.Slice(face, func(a, b int) bool { return angle(face[a]) < angle(face[b]) })
}
