package attractor

// The pure half of the 3-D desktops: which ones exist, what they are called,
// and the arithmetic. No build tag, so all of it can be checked without a
// browser — the same split as catShortLabels, and for the same reason. A table
// and a clamp are exactly the parts that go wrong quietly.

// The four reproductions, and the flat desktop they are reproductions against.
// Dates are the originals', because that is the point of them.
const (
	deskFlat    = "flat"    // ordinary windows
	deskGlass   = "glass"   // Sun's Project Looking Glass, 2003
	deskCube    = "cube"    // Compiz's desktop cube, 2006
	deskMetisse = "metisse" // Metisse, 2004
	deskBump    = "bump"    // BumpTop, 2009
)

// deskStyleOrder is the order the selector turns through, flat first so that
// the desk opens as an ordinary window manager and becomes a curiosity only
// when asked.
var deskStyleOrder = []string{deskFlat, deskGlass, deskCube, deskMetisse, deskBump}

// deskStyleLabel is what each is called in the selector. The selector is built
// from these two together — see buildDeskStyleSelect — rather than being
// written out again in the panel's HTML, which is how the category tooltip
// managed to stop mentioning Maps.
var deskStyleLabel = map[string]string{
	deskFlat:    "Flat",
	deskGlass:   "Looking Glass",
	deskCube:    "Cube",
	deskMetisse: "Metisse",
	deskBump:    "BumpTop",
}

// deskFaces is how many workspaces the cube has. Four, because it is a cube.
const deskFaces = 4

// faceOf assigns a window to a workspace: round-robin by the order it was
// opened, which is what an unconfigured Compiz did with new windows too.
func faceOf(i int) int {
	if i < 0 {
		i = -i
	}
	return i % deskFaces
}

// clampDeg confines an angle.
//
// Metisse clamps short of ninety degrees because edge-on a window is a line,
// and a window you cannot see is one you cannot turn back; BumpTop clamps its
// tilt so a pile leans rather than lies down.
func clampDeg(v, lim float64) float64 {
	if v > lim {
		return lim
	}
	if v < -lim {
		return -lim
	}
	return v
}
