// Package lattice draws a solid as cross-sections, one terminal per section.
//
// A volume of N×N×N voxels is cut three ways: N sheets across each axis. Each
// sheet is an ordinary terminal screen showing where the shape's surface
// crosses it, drawn in characters. One sheet on its own is a flat picture of
// one slice; 3N terminals stacked in space as transparent sheets — three
// families, each perpendicular to the other two — show the solid from every
// side, because whichever way it is turned one family of sheets faces the
// viewer while the other two are edge on.
//
// The program that draws a sheet does not know whether it is in such a stack.
// Every instance computes the same shape from the same clock and draws only
// its own slice, so any number of them, in any terminals, stay in step
// without speaking to each other.
package lattice

import "math"

// Vec3 is a point or direction in the volume, which spans −1 to 1 on each
// axis: x to the right, y up, z toward a viewer in front.
type Vec3 [3]float64

// Add is a + b.
func (a Vec3) Add(b Vec3) Vec3 { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

// Scale is a × s.
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a[0] * s, a[1] * s, a[2] * s} }

// Dot is a · b.
func (a Vec3) Dot(b Vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

// Cross is a × b.
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// Len is |a|.
func (a Vec3) Len() float64 { return math.Sqrt(a.Dot(a)) }

// Axis names a family of sheets by the axis they are perpendicular to.
type Axis int

// The three families.
const (
	X Axis = iota // side sheets, seen from the right
	Y             // level sheets, seen from above
	Z             // front sheets, seen from the front
)

// String is the axis's letter.
func (a Axis) String() string { return [...]string{"x", "y", "z"}[a] }

// ParseAxis reads an axis letter.
func ParseAxis(s string) (Axis, bool) {
	switch s {
	case "x", "X":
		return X, true
	case "y", "Y":
		return Y, true
	case "z", "Z":
		return Z, true
	}
	return 0, false
}

// Basis is how a family of sheets sits in the volume: what is to the right
// and what is up on its terminals, seen face on, and the normal toward that
// viewer. Right × Up = Normal for every family, so a sheet read from its own
// side is never mirrored, and read from the other side always is — the way a
// sign painted on glass is.
type Basis struct {
	Right, Up, Normal Vec3
}

// BasisOf is the basis of axis a's sheets.
func BasisOf(a Axis) Basis {
	switch a {
	case X: // seen from +x: the front of the volume is to the left
		return Basis{Right: Vec3{0, 0, -1}, Up: Vec3{0, 1, 0}, Normal: Vec3{1, 0, 0}}
	case Y: // seen from above: the back of the volume is at the top
		return Basis{Right: Vec3{1, 0, 0}, Up: Vec3{0, 0, -1}, Normal: Vec3{0, 1, 0}}
	}
	return Basis{Right: Vec3{1, 0, 0}, Up: Vec3{0, 1, 0}, Normal: Vec3{0, 0, 1}}
}

// Sheet is one section: slice Slice of axis Axis's N, counted along the
// normal from the far side.
type Sheet struct {
	Axis  Axis
	Slice int
	N     int
}

// Cols and Rows are a sheet's terminal size. A voxel is two characters wide,
// because a terminal's character cell is about twice as tall as it is wide,
// and two side by side make it square.
func (s Sheet) Cols() int { return 2 * s.N }

// Rows is the sheet's height in lines.
func (s Sheet) Rows() int { return s.N }

// center is the coordinate of the middle of voxel i along an axis of n.
func center(i, n int) float64 { return (float64(i)+0.5)/float64(n)*2 - 1 }

// Voxel is the center of the voxel at column u (counted in voxels, from the
// left) and row v (from the top) of the sheet.
func (s Sheet) Voxel(u, v int) Vec3 {
	b := BasisOf(s.Axis)
	return b.Normal.Scale(center(s.Slice, s.N)).
		Add(b.Right.Scale(center(u, s.N))).
		Add(b.Up.Scale(-center(v, s.N)))
}
