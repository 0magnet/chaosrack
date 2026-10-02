//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/conway"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
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
// texp.program, so it rotates/zooms/auto-rotates via the normal render path.

// skinSurface is the surface the spectrogram skin is painted on.
type skinSurface struct {
	// skin.source is WHICH picture is painted on the surface: "" for none, or
	// "spectrogram", "terminal" or "desk".
	//
	// It was a switch, and a switch could only mean the spectrogram. But the skin
	// is one of three places a second picture can go — behind the model, ON it, or
	// as it — and the other two already take any of the sources. There was no
	// reason for this one to take only the audio: the mesh does not care what is on
	// it, and each of these already keeps a canvas that is a texture.
	source   string
	dirty    bool
	vBuf     js.Value
	iBuf     js.Value
	idxCount int
}

var skin = skinSurface{
	dirty: true,
}

// spectroSkin reports whether anything is painted on the surface.
func (s *skinSurface) spectroSkin() bool { return s.source != "" }

// renderSkinnedMode keeps the spectrogram texture current and draws the
// current surface model as a filled, textured mesh. Called from
// generateForMode when the skin is on and the mode is skinnable.
func (s *skinSurface) renderSkinnedMode(mode string, nowMs float64) {
	tex, offset, ok := s.texture(nowMs)
	if !ok {
		return // the source has nothing to give yet; it says why itself
	}
	if s.dirty || s.vBuf.IsUndefined() {
		s.buildSkinMesh(mode)
		s.dirty = false
	}
	texp.drawTexturedMesh(s.vBuf, s.iBuf, s.idxCount, tex, offset)
}

// texture is the picture to paint and how far it has scrolled.
//
// THE OFFSET IS THE SPECTROGRAM'S ALONE. Its texture is a ring buffer written
// one column at a time, so the seam has to be walked round as it fills;
// everything else is an ordinary picture that starts at its own left edge. That
// is the only thing the three sources do differently here — the mesh, the
// program and the draw are identical, which is why this was worth generalising
// rather than writing twice more.
func (s *skinSurface) texture(nowMs float64) (js.Value, float32, bool) {
	switch s.source {
	case "terminal":
		t, ok := termPane.terminalTexture()
		return t, 0, ok
	case "desk":
		t, ok := deskModelCanvas()
		return t, 0, ok
	default:
		if !spect.ready {
			spect.initSpectrogram()
		}
		aud.ensureAudioSource()
		spect.updateSpectrogramTexture(nowMs)
		aud.maybeShowAudioStatus()
		return spect.texture, float32(spect.texCol) / float32(spectTexW), true
	}
}

// buildSkinMesh (re)generates and uploads the interleaved pos+uv vertex
// buffer and triangle index buffer for the current mode's surface.
func (s *skinSurface) buildSkinMesh(mode string) {
	var verts []float32
	var idx []uint16
	switch mode {
	case "torus":
		verts, idx = torusSkinMesh(torus.major, torus.minor, int(torus.stacksF), int(torus.slicesF))
	case "globe":
		verts, idx = sphereSkinMesh(1.0, int(globe.latF)*2, int(globe.lonF))
	case "nestedcube":
		verts, idx = cubeSkinMesh(verticesCube, indicesCube)
	case "polyhedron":
		verts, idx = polyhedronSkinMesh()
	default: // a round surface, for a mode with no skin of its own
		verts, idx = sphereSkinMesh(1.0, 30, 30)
	}
	if s.vBuf.IsUndefined() {
		s.vBuf = glctx.GL.Call("createBuffer")
	}
	if s.iBuf.IsUndefined() {
		s.iBuf = glctx.GL.Call("createBuffer")
	}
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, s.vBuf)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(verts), glctx.Types.StaticDraw)
	glctx.GL.Call("bindBuffer", glctx.Types.ElementArrayBuffer, s.iBuf)
	glctx.GL.Call("bufferData", glctx.Types.ElementArrayBuffer, SliceToTypedArray(idx), glctx.Types.StaticDraw)
	s.idxCount = len(idx)
}

// gridTriangles emits two triangles per (stacks x slices) grid quad for a
// vertex layout of (slices+1) columns per row.
func gridTriangles(stacks, slices int) []uint16 {
	idx := make([]uint16, 0, stacks*slices*6)
	row := slices + 1
	for i := range stacks {
		for j := range slices {
			a := uint16(i*row + j) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
			b := a + 1
			c := uint16((i+1)*row + j) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
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
func torusSkinMesh(major, minor float32, stacks, slices int) ([]float32, []uint16) {
	verts := make([]float32, 0, (stacks+1)*(slices+1)*5)
	for i := 0; i <= stacks; i++ {
		theta := float64(i) * 2.0 * math.Pi / float64(stacks)
		u := float32(i) / float32(stacks)
		for j := 0; j <= slices; j++ {
			// The same poloidal roll the wireframe takes, so a skinned torus turns
			// with it rather than sitting still while its wireframe rolls. The UV
			// below deliberately does NOT take the roll: the texture stays put on
			// the surface, and it is the surface that moves under it.
			phi := float64(j)*2.0*math.Pi/float64(slices) + float64(torus.rollPhi)
			x := (float64(major) + float64(minor)*math.Cos(phi)) * math.Cos(theta)
			y := (float64(major) + float64(minor)*math.Cos(phi)) * math.Sin(theta)
			z := float64(minor) * math.Sin(phi)
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
	for i := range n {
		uv := quadUV[i%4]
		out = append(out, cubeVerts[i*3], cubeVerts[i*3+1], cubeVerts[i*3+2], uv[0], uv[1])
	}
	// Generate FILL triangulation from the 4-verts-per-face layout: (0,1,2)
	// + (0,2,3) tiles each quad. The cube's own index table is the WIREFRAME
	// pattern (0,1,2, 1,2,3) — fine for lines, where edges overdraw, but as
	// filled triangles abc+bcd don't tile the quad: the corner triangle at
	// vertex d is left uncovered (an unskinned triangular hole on every face).
	idx := make([]uint16, 0, (n/4)*6)
	for f := range n / 4 {
		b := uint16(f * 4)
		idx = append(idx, b, b+1, b+2, b, b+2, b+3)
	}
	return out, idx
}

// polyhedronSkinMesh wraps the spectrogram onto the Polyhedron model: its own
// faces, which morph and kis can make anything but convex, so they are taken
// from the solid rather than recovered from a hull. A tiling is flat, and is
// mapped flat.
func polyhedronSkinMesh() ([]float32, []uint16) {
	s, solid := polySolid()
	verts := make([]float32, 0, len(s.Verts)*3)
	for _, v := range s.Verts {
		verts = append(verts, float32(v.X), float32(v.Y), float32(v.Z))
	}
	faces := s.Faces
	if !solid && len(faces) > 0 && conway.CurvatureOf(int(poly.pF+0.5), int(poly.qF+0.5)) == conway.Hyperbolic {
		faces = faces[:len(faces)-1] // the disk's rim is drawn, not a face
	}
	return faceSkinMesh(verts, faces, !solid)
}

// faceSkinMesh wraps the spectrogram onto a polyhedron given its faces, each
// fan-triangulated. UVs use a spherical projection (u = longitude / time,
// v = latitude / frequency) matching the sphere skin, with a per-triangle
// seam fix so faces spanning the u-wrap don't smear — or, planar, the x and y
// of a flat figure. Vertices are duplicated per triangle (non-indexed soup)
// to keep UVs independent.
func faceSkinMesh(verts []float32, faces [][]int, planar bool) ([]float32, []uint16) {
	out := make([]float32, 0, 256)
	var idx []uint16
	emit := func(vi int, u, v float32) {
		out = append(out, verts[vi*3], verts[vi*3+1], verts[vi*3+2], u, v)
		idx = append(idx, uint16(len(idx))) //nolint:gosec // a mesh index, bounded by pkg/conway's patch cap
	}
	if planar {
		for _, face := range faces {
			for t := 1; t < len(face)-1; t++ {
				for _, vi := range [3]int{face[0], face[t], face[t+1]} {
					emit(vi, (verts[vi*3]+1)/2, (verts[vi*3+1]+1)/2)
				}
			}
		}
		return out, idx
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
						us[a]++
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
