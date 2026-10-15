//go:build js && wasm

package attractor

import (
	"github.com/0magnet/pisano/pkg/pisano"
	"github.com/go-gl/mathgl/mgl32"
)

// The Turtle's physics on the page: the room it lives in is the view, and the
// hand that pulls it is the pointer (turtlebody.go is the body itself).

// turtleCamDist is where the camera ACTUALLY is: the fitted distance less the
// zoom control, which is exactly what the render loop uses to build the view
// matrix.
//
// The room used the fitted distance alone and ignored the zoom, so zooming
// changed the picture without changing the room. The floor stopped being the
// bottom of the screen, and the figure appeared to move rather than simply get
// bigger — which is not what a zoom does to a thing sitting on a floor.
func turtleCamDist() float32 {
	d := view.initDist - view.ctl.zoom
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
	if gpu.height > 0 {
		aspect = float32(gpu.width) / float32(gpu.height)
	}
	if halfH <= 0 {
		halfH = box / 2
	}
	// Panning moves the camera, so it moves the room with it — the floor stays
	// the bottom of the picture rather than sliding off it.
	return -view.panX, -view.panY, halfH * aspect, halfH
}

// physOn reports whether the walk is being simulated. Gravity was the switch
// for a while, which stopped being tenable the moment it could pull upward:
// zero then means "no pull", not "no physics", and floating in a box is
// somewhere you might want to be.
func physOn() bool { return turtlePhysOn && run.selectedMode == "turtle" }

// turtlePhysOn is the Physics switch on the Motion panel.
var turtlePhysOn bool

// pageTurtleWorld is the body's world on the page: the view's room and
// camera, the Physics knobs, the Speed knob's step and the pointer's hand.
func pageTurtleWorld() turtleWorld {
	cx, cy, halfW, halfH := turtleRoom()
	return turtleWorld{
		cx: cx, cy: cy, halfW: halfW, halfH: halfH,
		camDist: turtleCamDist(),
		grav:    turtle.gravF, fric: turtle.fricF, bounce: turtle.bounceF, spin: turtle.spinF,
		dt: 0.016 * float32(sim.speedSteps) * sim.speedScale,
		grab: func(b *turtleBody, t *turtleWalk, pts []pisano.Pt3, mass float32) (fx, fy, torque float32) {
			return b.grabForce(t, pts, mass)
		},
	}
}

// turtleTiltBy turns the figure about the SCREEN axes, which is what a drag
// near the middle of the picture means.
func (b *turtleBody) turtleTiltBy(dax, day float32) {
	b.ensureTilt()
	inc := mgl32.HomogRotate3DX(dax).Mul4(mgl32.HomogRotate3DY(day))
	b.tilt = inc.Mul4(b.tilt)
}
