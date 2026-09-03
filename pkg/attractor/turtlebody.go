//go:build js && wasm

package attractor

import (
	"math"

	"github.com/0magnet/pisano/pkg/pisano"
	"github.com/go-gl/mathgl/mgl32"
)

// The turtle path as a body with weight.
//
// This is not a departure from what the rest of the panel is. An analog
// computer was a physics simulator — you patched in a mass, a spring rate and a
// damping term and watched the transient — and everything below is the same
// integrator the flows use, with the same kind of coefficients on the same kind
// of knobs. What is new is only what is being integrated: not a vector field
// but a rigid body, in the plane of the screen, with the figure's own shape as
// its mass.
//
// The figure keeps its shape exactly — it is a rigid body, three degrees of
// freedom, and the walk is what makes it interesting. New lattice points arrive
// at the head every frame and old ones fall off the tail, so the body is being
// extruded: it grows out along the direction it drifts, its mass moves, its
// balance shifts, and a thing that is heavier at one end than it was a moment
// ago will tip. Nothing had to be faked to make it march. It marches because it
// is being made, at one end, out of arithmetic.
//
// Z rides along and is never simulated. A three-dimensional figure in a
// two-dimensional world is the thing that was asked for: it falls and tips in
// the screen plane while its depth stays exactly what the walk said it was.

// turtleBody is the rigid body: where the figure's center of mass is in the
// world, how fast it is going, and how it is turned.
//
// The state is kept about the CENTER OF MASS rather than about the lattice
// origin, because that is the point the equations are simple about — a force
// through it produces no turn. The complication that buys is that the center of
// mass moves through the material as the walk extrudes, and when it does the
// material must not jump; keeping the last one is what lets that be corrected
// for rather than integrated.
type turtleBody struct {
	x, y   float32 // the center of mass, in world units
	vx, vy float32
	ang    float32 // how far the body has turned, radians
	spin   float32

	local    [2]float32 // last frame's center of mass in the body's own frame
	localZ   float32    // and the mean DEPTH, which is centered but never simulated
	placed   bool
	contacts int // how many points were touching last frame, for the contact spring

	// tilt is the figure's own THREE-dimensional orientation, turned by a drag
	// near the middle of the picture exactly as the trackball turns any other
	// model.
	//
	// It belongs to the body rather than to the camera, and that is the whole
	// idea: a three-dimensional object trapped in a two-dimensional world. What
	// falls and lands is the figure's SILHOUETTE — its shadow on the screen
	// plane — so tilting does not just change the view of it, it changes what
	// the floor has to hold up. Turn a long walk edge-on and it lands as a
	// short one, because seen from there it IS a short one.
	//
	// Rotating the camera instead would have tipped the plane the physics lives
	// in out of the screen, which is what made gravity pull sideways before.
	// The camera stays square on; the object is what turns.
	tilt mgl32.Mat4
}

// ensureTilt gives the orientation an identity to start from — a zero matrix
// would collapse the figure to a point on the first frame.
func (b *turtleBody) ensureTilt() {
	if b.tilt == (mgl32.Mat4{}) {
		b.tilt = mgl32.Ident4()
	}
}

// tiltPoint maps a lattice point into the body's own frame: scaled, y flipped
// (pisano's grows downward), and turned by the figure's 3-D orientation. Every
// part of the physics reads points through here, so the mass, the contacts, the
// reach and the drawing all agree about what shape the body currently is.
func (b *turtleBody) tiltPoint(p pisano.Pt3, sc float32) (x, y, z float32) {
	b.ensureTilt()
	v := b.tilt.Mul4x1(mgl32.Vec4{float32(p.X) * sc, -float32(p.Y) * sc, float32(p.Z) * sc, 1})
	return v.X(), v.Y(), v.Z()
}

// turtleTiltBy turns the figure about the SCREEN axes, which is what a drag
// near the middle of the picture means.
func (b *turtleBody) turtleTiltBy(dax, day float32) {
	b.ensureTilt()
	inc := mgl32.HomogRotate3DX(dax).Mul4(mgl32.HomogRotate3DY(day))
	b.tilt = inc.Mul4(b.tilt)
}

// turtleCamDist is where the camera ACTUALLY is: the fitted distance less the
// zoom control, which is exactly what the render loop uses to build the view
// matrix.
//
// The room used the fitted distance alone and ignored the zoom, so zooming
// changed the picture without changing the room. The floor stopped being the
// bottom of the screen, and the figure appeared to move rather than simply get
// bigger — which is not what a zoom does to a thing sitting on a floor.
func turtleCamDist() float32 {
	d := initCameraDist - cachedZoom
	if d < 0.1 {
		d = 0.1
	}
	return d
}

// turtleRoom is the room the body lives in: the viewport itself.
//
// The camera sits at distance d looking down −z with a 45° vertical field, so
// what is on screen at the plane the figure is drawn in spans d·tan(22.5°)
// above and below the middle, and that times the aspect to either side. Taking
// the walls from those numbers is what makes the bottom edge of the picture the
// floor and the sides the walls, at whatever zoom, instead of an invisible box
// that happened to be a constant — and "at whatever zoom" is only true now that
// the distance it asks for includes the zoom.
func turtleRoom() (cx, cy, halfW, halfH float32) {
	halfH = turtleCamDist() * 0.41421 // tan(22.5°), half the 45° vertical FOV
	aspect := float32(1.6)
	if height > 0 {
		aspect = float32(width) / float32(height)
	}
	if halfH <= 0 {
		halfH = box / 2
	}
	// Panning moves the camera, so it moves the room with it — the floor stays
	// the bottom of the picture rather than sliding off it.
	return -panX, -panY, halfH * aspect, halfH
}

// stickSlope is how quickly friction reaches full strength as a contact
// starts to slip. Coulomb friction is |f| = mu*N opposing the slip whatever
// the slip SPEED, which as written explicitly would chatter at a fixed step —
// so it is smoothed, and this is how sharp that smoothing is.
//
// It was 4, meaning friction only reached full strength at a quarter of a
// world unit per second, in a room about eight units tall. Anything slower
// than that got a fraction of the friction it should have, so the figure never
// gripped: it slid across the floor while the walk streamed through it, which
// is what "really slipping on the floor" was. At 60 a contact is holding
// properly by about a centimeter per second of slip, so the material plants
// itself and the BODY is what moves — which is what walking is.
const stickSlope float32 = 60

// turtleRoomFit is how much of the room's height the figure is drawn at once it
// has to live in it. Without it the figure is scaled to fill the frame, which
// makes a body as tall as its container: jammed against floor and ceiling at
// once, unable to turn without exceeding the room, and the opposing contacts
// wind each other up until the whole thing takes off.
const turtleRoomFit float32 = 0.55

// physScale is the lattice-to-world scale while the figure has weight.
func (t *turtleWalk) physScale() float32 { return t.scale * turtleRoomFit }

// physOn reports whether the walk is being simulated. Gravity was the switch
// for a while, which stopped being tenable the moment it could pull upward:
// zero then means "no pull", not "no physics", and floating in a box is
// somewhere you might want to be.
func physOn() bool { return turtlePhysOn && selectedMode == "turtle" }

// turtlePhysOn is the Physics switch on the Motion panel.
var turtlePhysOn bool

// step advances the body one frame, given the trail it is made of.
//
// The order matters and is the usual one: work out what the body currently IS
// (its mass, where its center of mass sits, how hard it is to turn), then what
// is pushing on it, then move it. Contact is a spring against the floor rather
// than an impulse — which is both the analog-computer way to write it and the
// stable one at a fixed frame step.
func (b *turtleBody) step(t *turtleWalk, pts []pisano.Pt3) {
	n := len(pts)
	if n < 2 {
		return
	}
	// What the body is. Every lattice point is one unit of mass, so the center
	// of mass is their average and the moment of inertia is the sum of their
	// squared distances from it — in world units, since that is what the
	// integration below is in.
	// One pass, not two. The moment of inertia is wanted about the center of
	// mass, which is not known until the pass is over — but the parallel-axis
	// theorem says the sum of squared distances about the center is the sum
	// about the origin less n times the center's own square, so both come out
	// of the same walk over the points.
	// Measured from a point INSIDE the figure, not from the lattice origin. A
	// walk that drifts is thousands of units from where it started, so summing
	// squares about the origin gives two enormous nearly-equal numbers whose
	// difference is the answer — and in single precision that difference is
	// noise. The moment of inertia came out near zero, every torque divided by
	// it was enormous, and the figure left at once. Offsets from a point of the
	// figure are the size of the figure, however far it has walked.
	sc := t.physScale()
	// Read through the figure's own 3-D orientation, so the mass and the shape
	// are the SILHOUETTE of however it is currently turned.
	ox, oy, oz := b.tiltPoint(pts[0], sc)
	var sx, sy, sz, sq float32
	for _, p := range pts {
		tx, ty, tz := b.tiltPoint(p, sc)
		x, y := tx-ox, ty-oy
		sx += x
		sy += y
		sz += tz - oz
		sq += x*x + y*y
	}
	inv := 1 / float32(n)
	cx, cy := sx*inv, sy*inv // the center of mass, from that same point
	lx, ly := ox+cx, oy+cy
	// The mean depth, offset from a point of the figure for the same precision
	// reason as above. Depth is not simulated — the room is two-dimensional and
	// the figure's z is exactly what the walk said it was — but it does have to
	// be CENTERED, and that is a different claim.
	//
	// It was not, and a 3-D walk drifts in z the same way it drifts in x and y.
	// The room holds x and y; nothing held z, so the figure walked out of the
	// view frustum in depth and the screen simply went black while the body sat
	// correctly on the floor, still being simulated, still reporting sensible
	// numbers. Measured: a dim=3 walk vanished after about seven seconds and a
	// dim=2 walk never did.
	//
	// Subtracting the mean moves the figure as a whole to the plane the camera
	// is focused on and changes nothing about its shape: relative depth, which
	// is the only part of z that means anything in a two-dimensional room, is
	// untouched.
	lz := oz + sz*inv

	if !b.placed {
		// Start where the figure was already being drawn, at rest and level, so
		// turning gravity on drops it from where it stood rather than from
		// wherever the lattice happens to be.
		b.x, b.y = 0, 0
		b.vx, b.vy, b.ang, b.spin = 0, 0, 0, 0
		b.local = [2]float32{lx, ly}
		b.localZ = lz
		b.placed = true
	}
	// The walk extruded since last frame, so the center of mass has moved
	// through the material — and it is the material that is moving, not the
	// body. New points arrive at the head and old ones leave the tail, so the
	// figure slides through its own body like a chain through a link, which is
	// the marching; where the BODY is remains a question for the forces below.
	//
	// Carrying the world position along with the center of mass instead — which
	// is what this did at first, to stop the material appearing to jump — hands
	// the body the walk's entire drift as a translation applied directly to its
	// position, every frame, forever. No wall can push back against a position
	// that is being assigned rather than integrated, so the figure simply left
	// the room and never came back.
	// How far the center of mass moved THROUGH the material since last frame,
	// captured before the new one is stored. This is the extrusion, and it is
	// the whole reason the figure walks.
	dlx, dly := lx-b.local[0], ly-b.local[1]
	b.local = [2]float32{lx, ly}
	b.localZ = lz

	cos, sin := cosf(b.ang), sinf(b.ang)

	// The step, needed here rather than at the integration below because the
	// material's speed is wanted while the contact forces are being worked out.
	dt := 0.016 * float32(speedSteps) * speedScale
	if dt > 0.05 {
		dt = 0.05 // a slow frame must not launch it
	}

	// The velocity the MATERIAL has through the world, which is not the body's.
	// A point sits at b.pos + R·(P·sc − local), so as the walk extrudes and the
	// center of mass drifts through the figure, every point of it moves at
	// −R·(d local/dt) on top of whatever the body is doing.
	//
	// Friction could not see this term and so there was nothing to push
	// against: the contact patch was computed as though it were stationary,
	// produced no traction, and the figure sat exactly where it landed while
	// material streamed through it — "no grip on the floor", which is precisely
	// what it was. Extrusion is what the marching is made of, so it has to be
	// part of the velocity the floor rubs against.
	var mvx, mvy float32
	if dt > 0 {
		mvx = -(dlx*cos - dly*sin) / dt
		mvy = -(dlx*sin + dly*cos) / dt
	}
	inertia := (sq - float32(n)*(cx*cx+cy*cy)) * sc * sc
	if inertia < 0 {
		inertia = 0 // only rounding can get here
	}
	mass := float32(n)
	inertia *= max(0.05, turtleSpinF) // SPIN dials how readily it turns
	if inertia <= 0 {
		inertia = 1
	}

	// What is pushing on it. Gravity acts through the center of mass and so
	// makes no torque; the floor acts wherever the figure touches it, and that
	// is what tips it over.
	cxR, cyR, halfW, halfH := turtleRoom()
	fx, fy := float32(0), -turtleGravF*mass
	var torque float32
	// How far the body reaches, in each direction, from its center of mass. It
	// costs nothing to note while the points are being walked anyway, and it is
	// what makes the walls solid below.
	loX, hiX := float32(1e9), float32(-1e9)
	loY, hiY := float32(1e9), float32(-1e9)
	// How hard a wall pushes back.
	//
	// A constant was wrong, and wrong in the way that stops a thing settling: a
	// figure of fifteen hundred points weighs fifteen hundred times what one
	// point does, so a spring stiff enough to hold up a short figure lets a long
	// one sink straight through. It sank, the hard projection at the end of the
	// frame yanked it back out, and the two fought — which is what "I can't tell
	// that it actually hit the floor" looks like.
	//
	// So the stiffness is whatever it takes to hold THIS body up with a
	// penetration of about a hundredth of the room. Damping is referred to the
	// same spring: critical at BOUNCE zero, none at BOUNCE one, so the knob
	// means what it says.
	//
	// It is a TOTAL, and the contacts share it. Dividing the stiffness by a
	// contact count and then applying the result at every touching point was
	// the thing that would not settle, and the count it divided by was last
	// frame's. That count is not slow-moving where it matters: on the frame the
	// figure lands it goes from none to dozens at once, so the spring was sized
	// for a single contact and then applied at all of them. The figure was
	// launched, came down with a high count, got a spring too soft to hold it,
	// sank, and did it again — floating, touching down every so often, and
	// spinning, because a normal force that is fifty times too large makes a
	// torque that is fifty times too large with it.
	//
	// Sharing one total force is also the physically right invariant: a body
	// resting on a plane is held up by its own weight in total, no matter how
	// much of it happens to be in contact.
	accel := absf(turtleGravF)
	if accel < 2 {
		accel = 2 // something to push back with even in free fall
	}
	kTot := accel * mass / (0.01 * halfH)
	dampTot := 2 * sqrtf(kTot*mass) * (1 - clamp01(turtleBounceF))
	kPer, damp := kTot, dampTot
	// Contact forces accumulate apart from gravity so they can be shared out
	// once the count is known — this frame's count, not the last one's.
	var cfx, cfy, ctorque float32
	touching := 0
	for _, p := range pts {
		tx, ty, tz := b.tiltPoint(p, sc)
		rx0 := tx - lx
		ry0 := ty - ly
		rx := rx0*cos - ry0*sin
		ry := rx0*sin + ry0*cos
		wx, wy := b.x+rx, b.y+ry
		// Where this point is DRAWN, at the focal plane, so the spring pushes
		// against the wall the eye sees rather than one at the wrong depth.
		g := b.depthGain(tz-b.localZ, turtleCamDist())
		px, py := wx*g, wy*g
		loX, hiX = min(loX, rx), max(hiX, rx)
		loY, hiY = min(loY, ry), max(hiY, ry)
		// The velocity of THIS point: the body's, plus what the spin carries it
		// at, plus the material streaming through the body as the walk extrudes.
		// Friction and damping act on the point, not the body.
		pvx := b.vx - b.spin*ry + mvx
		pvy := b.vy + b.spin*rx + mvy
		hit := false

		// Each wall pushes along its own normal and rubs along its own face, so
		// the figure can slide down a wall or skid along the ceiling exactly as
		// it slides along the floor.
		if d := ((cyR - halfH) - py) / g; d > 0 { // floor
			n := kPer*d - damp*pvy
			if n > 0 {
				f := -turtleFricF * n * clampUnit(pvx*stickSlope)
				cfx += f
				cfy += n
				ctorque += rx*n - ry*f
				hit = true
			}
		}
		if d := (py - (cyR + halfH)) / g; d > 0 { // ceiling
			n := kPer*d + damp*pvy
			if n > 0 {
				f := -turtleFricF * n * clampUnit(pvx*stickSlope)
				cfx += f
				cfy -= n
				ctorque += -rx*n - ry*f
				hit = true
			}
		}
		if d := ((cxR - halfW) - px) / g; d > 0 { // left
			n := kPer*d - damp*pvx
			if n > 0 {
				f := -turtleFricF * n * clampUnit(pvy*stickSlope)
				cfx += n
				cfy += f
				ctorque += rx*f - ry*n
				hit = true
			}
		}
		if d := (px - (cxR + halfW)) / g; d > 0 { // right
			n := kPer*d + damp*pvx
			if n > 0 {
				f := -turtleFricF * n * clampUnit(pvy*stickSlope)
				cfx -= n
				cfy += f
				ctorque += rx*f + ry*n
				hit = true
			}
		}
		if hit {
			touching++
		}
	}
	// Share the one total contact force out over whatever is touching.
	if touching > 0 {
		s := 1 / float32(touching)
		fx += cfx * s
		fy += cfy * s
		torque += ctorque * s
	}
	b.contacts = touching

	if gfx, gfy, gt := b.grabForce(t, pts, mass); gfx != 0 || gfy != 0 || gt != 0 {
		fx += gfx
		fy += gfy
		torque += gt
	}

	// Move it, on the step worked out above — a fixed step at the frame rate,
	// scaled by the Speed knob so the physics and the walk keep time.
	b.vx += fx / mass * dt
	b.vy += fy / mass * dt
	b.spin += torque / inertia * dt
	// Air drag, so a figure that is not touching anything still settles rather
	// than ringing forever. Light: at 0.4 a fall was visibly damped, which reads
	// as a heavy object moving through something thicker than air. It is here to
	// stop ringing, not to slow the drop.
	const drag = 0.12
	b.vx -= b.vx * drag * dt
	b.vy -= b.vy * drag * dt
	b.spin -= b.spin * drag * dt
	b.x += b.vx * dt
	b.y += b.vy * dt
	b.ang += b.spin * dt

	// Re-measure how far the body reaches now that it has TURNED. The extents
	// above were taken with the previous frame's angle, and the projection
	// below uses them to decide where the lowest point is — so a body that
	// rotated during this step was put back against the floor using the reach
	// it had before it rotated, and sank by the difference. It showed up as the
	// figure going through the floor a little on landing, which is exactly when
	// the spin is largest, and it got worse with any walk that lands
	// off-balance and turns hard as it settles.
	loX, hiX, loY, hiY = b.extent(pts, sc, lx, ly)

	// And then the walls are solid. Forces alone are a suggestion — a fast
	// enough body, or a stiff enough spring at a fixed frame step, goes through
	// and keeps going — so after the body has moved it is put back inside the
	// room it is in, and whatever speed it had into the wall is either returned
	// to it or absorbed.
	//
	// Measured where the points are DRAWN rather than where they are in world
	// space. The two are not the same for a figure with depth: the walls are the
	// edges of the picture, and perspective draws a nearer point further out, so
	// a point resting exactly on the floor plane but in front of it appears
	// below the bottom of the screen. That is the part of the model that could
	// go through the floor and the walls while the physics was satisfied.
	bounce := clamp01(turtleBounceF)
	up, down, right, left := b.roomFix(pts, sc, lx, ly, cxR, cyR, halfW, halfH, turtleCamDist())
	if up > 0 {
		b.y += up
		if b.vy < 0 {
			b.vy = -b.vy * bounce
		}
	}
	if down > 0 {
		b.y -= down
		if b.vy > 0 {
			b.vy = -b.vy * bounce
		}
	}
	if right > 0 {
		b.x += right
		if b.vx < 0 {
			b.vx = -b.vx * bounce
		}
	}
	if left > 0 {
		b.x -= left
		if b.vx > 0 {
			b.vx = -b.vx * bounce
		}
	}
	// If any of that ever stops being a number — a stiff contact and a slow
	// frame can do it — the body is put back rather than left as a hole where
	// the figure used to be.
	if b.x != b.x || b.y != b.y || b.ang != b.ang || b.vx != b.vx || b.vy != b.vy {
		b.placed = false
		return
	}

	// A figure wider than the room cannot be inside it; center it rather than
	// letting the two walls fight over which one wins.
	if hiX-loX > 2*halfW {
		b.x = cxR - (loX+hiX)/2
		b.vx = 0
	}
	if hiY-loY > 2*halfH {
		b.y = cyR - (loY+hiY)/2
		b.vy = 0
	}
}

// place maps a lattice point into the world through the body's transform.
func (b *turtleBody) place(t *turtleWalk, p pisano.Pt3) (x, y, z float32) {
	sc := t.physScale()
	tx, ty, tz := b.tiltPoint(p, sc)
	rx := tx - b.local[0]
	ry := ty - b.local[1]
	cos, sin := cosf(b.ang), sinf(b.ang)
	return b.x + rx*cos - ry*sin, b.y + rx*sin + ry*cos, tz - b.localZ
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func sqrtf(v float32) float32 {
	if v <= 0 {
		return 0
	}
	return float32(math.Sqrt(float64(v)))
}

func cosf(a float32) float32 { return float32(math.Cos(float64(a))) }
func sinf(a float32) float32 { return float32(math.Sin(float64(a))) }

// clampUnit holds a value to either side of zero. (clamp01, for the one-sided
// case, already exists in audiofeatures_js.go.)
func clampUnit(v float32) float32 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

// extent measures how far the body reaches from its center of mass, in each
// direction, at its CURRENT angle. The force loop works this out as it goes,
// but the projection at the end of the step needs it again after the body has
// turned — a reach taken before the rotation is the wrong reach to put a
// rotated body back inside its room with.
func (b *turtleBody) extent(pts []pisano.Pt3, sc, lx, ly float32) (loX, hiX, loY, hiY float32) {
	cos, sin := cosf(b.ang), sinf(b.ang)
	loX, hiX = float32(1e9), float32(-1e9)
	loY, hiY = float32(1e9), float32(-1e9)
	for _, p := range pts {
		tx, ty, _ := b.tiltPoint(p, sc)
		rx0 := tx - lx
		ry0 := ty - ly
		rx := rx0*cos - ry0*sin
		ry := rx0*sin + ry0*cos
		loX, hiX = min(loX, rx), max(hiX, rx)
		loY, hiY = min(loY, ry), max(hiY, ry)
	}
	return loX, hiX, loY, hiY
}

// depthGain is how much a point's DEPTH magnifies its distance from the middle
// of the picture, which is the difference between where the collision thinks a
// point is and where it is drawn.
//
// The room's walls are the edges of the PICTURE, but the collision was done in
// world space at the focal plane, and the figure has depth: it is a
// three-dimensional walk. Under perspective a point nearer the camera is drawn
// further out, so a point resting exactly on the floor plane but in front of it
// is drawn BELOW the bottom of the screen, and part of the figure goes through
// the floor and the walls while the physics is satisfied that it did not.
//
// Multiplying a point's offset by d/(d−z) gives where it would have to sit AT
// the focal plane to be drawn where it actually is, so comparing that against
// the room compares like with like.
func (b *turtleBody) depthGain(z, camDist float32) float32 {
	den := camDist - z
	if den < camDist*0.2 {
		den = camDist * 0.2 // a point almost at the lens must not blow up
	}
	if den <= 0 {
		return 1
	}
	return camDist / den
}

// roomFix returns how far the body must move along each axis for every point of
// it to be drawn inside the room — perspective included. The corrections are
// what the hard projection at the end of a step applies.
func (b *turtleBody) roomFix(pts []pisano.Pt3, sc, lx, ly, cxR, cyR, halfW, halfH, camDist float32) (up, down, right, left float32) {
	cos, sin := cosf(b.ang), sinf(b.ang)
	for _, p := range pts {
		tx, ty, tz := b.tiltPoint(p, sc)
		rx0 := tx - lx
		ry0 := ty - ly
		rx := rx0*cos - ry0*sin
		ry := rx0*sin + ry0*cos
		g := b.depthGain(tz-b.localZ, camDist)
		// Where this point is drawn, expressed at the focal plane.
		px := (b.x + rx) * g
		py := (b.y + ry) * g
		// And what the body would have to move for it to be inside. The gain
		// divides back out because the body moves in world units, not drawn
		// ones.
		if d := (cyR - halfH) - py; d > 0 {
			up = max(up, d/g)
		}
		if d := py - (cyR + halfH); d > 0 {
			down = max(down, d/g)
		}
		if d := (cxR - halfW) - px; d > 0 {
			right = max(right, d/g)
		}
		if d := px - (cxR + halfW); d > 0 {
			left = max(left, d/g)
		}
	}
	return up, down, right, left
}
