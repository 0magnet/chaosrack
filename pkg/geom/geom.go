// Package geom holds the Geometry-class wireframes — sphere, torus,
// globe, magnetosphere — as plain math with no renderer attached, for
// hosts without a GPU: a terminal UI hands them to pkg/rasterview, a
// test asserts on them directly.
//
// The math (float32 rounding included) matches what pkg/attractor
// uploads to WebGL at its shipped defaults, so the models look the same
// everywhere they appear. pkg/attractor keeps its own per-frame
// generators, which carry knob-driven variants (poloidal torus roll,
// the globe's spiral and twist) and scratch-buffer reuse tuned against
// TinyGo's collector; folding the two together without disturbing that
// tuning is future work.
package geom

import "math"

// Lines is an indexed line list: Vertices holds xyz triples, Indices
// holds endpoint pairs — exactly what gl.LINES draws.
type Lines struct {
	Vertices []float32
	Indices  []uint16
}

// Sphere generates a latitude/longitude grid sphere as vertical line
// segments between adjacent stacks. baseIdx offsets the indices so the
// result can be appended to an existing buffer.
func Sphere(radius float32, stacks, slices int, baseIdx uint16) ([]float32, []uint16) {
	var vertices []float32
	var indices []uint16
	for i := 0; i <= stacks; i++ {
		phi := float32(i) * float32(math.Pi) / float32(stacks)
		for j := 0; j <= slices; j++ {
			theta := float32(j) * 2.0 * float32(math.Pi) / float32(slices)
			xv := radius * float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := radius * float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := radius * float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j <= slices; j++ {
			indices = append(indices, baseIdx+uint16(i*(slices+1)+j), baseIdx+uint16((i+1)*(slices+1)+j))
		}
	}
	return vertices, indices
}

// Torus generates a torus wireframe: horizontal ring edges and vertical
// edges between adjacent stacks.
func Torus(R, r float32, stacks, slices int, baseIdx uint16) ([]float32, []uint16) {
	var vertices []float32
	var indices []uint16
	for i := 0; i <= stacks; i++ {
		theta := float32(i) * 2.0 * math.Pi / float32(stacks)
		for j := 0; j <= slices; j++ {
			phi := float32(j) * 2.0 * math.Pi / float32(slices)
			xv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Cos(float64(theta)))
			yv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Sin(float64(theta)))
			zv := r * float32(math.Sin(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j < slices; j++ {
			cur := baseIdx + uint16(i*(slices+1)+j)
			next := cur + 1
			below := baseIdx + uint16((i+1)*(slices+1)+j)
			// Horizontal ring edge
			indices = append(indices, cur, next)
			// Vertical edge
			indices = append(indices, cur, below)
		}
	}
	return vertices, indices
}

// Globe generates a unit globe as lat latitude circles (the poles
// excluded) and lon meridians, each drawn with pts points per circle.
// The attractor's Globe mode uses Globe(18, 36, 60).
func Globe(lat, lon, pts int) Lines {
	var vertices []float32
	var indices []uint16

	// Latitude lines
	for i := 1; i < lat; i++ {
		phi := float32(i) * float32(math.Pi) / float32(lat)
		base := uint16(len(vertices) / 3)
		for j := 0; j <= pts; j++ {
			theta := float32(j) * 2.0 * float32(math.Pi) / float32(pts)
			xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
			if j > 0 {
				indices = append(indices, base+uint16(j-1), base+uint16(j))
			}
		}
	}

	// Longitude lines
	for j := 0; j < lon; j++ {
		theta := float32(j) * 2.0 * float32(math.Pi) / float32(lon)
		base := uint16(len(vertices) / 3)
		for i := 0; i <= pts; i++ {
			phi := float32(i) * float32(math.Pi) / float32(pts)
			xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
			if i > 0 {
				indices = append(indices, base+uint16(i-1), base+uint16(i))
			}
		}
	}

	return Lines{vertices, indices}
}

// Magnetosphere generates a central sphere ringed by dipole field lines
// (r = R·cos²θ).
func Magnetosphere() Lines {
	var allVerts []float32
	var allIdx []uint16

	// Central sphere
	sv, si := Sphere(0.5, 16, 16, 0)
	allVerts = append(allVerts, sv...)
	allIdx = append(allIdx, si...)

	// Magnetic field lines — dipole field: r = R*cos²(θ)
	nLines := 12
	ptsPerLine := 80
	for i := 0; i < nLines; i++ {
		angle := float32(i) * 2.0 * math.Pi / float32(nLines)
		base := uint16(len(allVerts) / 3)
		R := float32(3.0)
		for j := 0; j <= ptsPerLine; j++ {
			theta := float32(-math.Pi/2) + float32(j)*float32(math.Pi)/float32(ptsPerLine)
			ct := float32(math.Cos(float64(theta)))
			r := R * ct * ct
			xv := r * ct * float32(math.Cos(float64(angle)))
			yv := r * ct * float32(math.Sin(float64(angle)))
			zv := r * float32(math.Sin(float64(theta)))
			allVerts = append(allVerts, xv, yv, zv)
			if j > 0 {
				allIdx = append(allIdx, base+uint16(j-1), base+uint16(j))
			}
		}
	}

	return Lines{allVerts, allIdx}
}
