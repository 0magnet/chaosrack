//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// The swarm (Display, POINTS' S button): a 3-D flow drawn as TRAIL's count of
// particles carried through it at once, instead of one trail. It starts as a
// tight ball on the attractor every time it is switched on, so what it shows
// first is the ball smearing out along the flow, and after that the
// attractor as the place the particles crowd. Depth cueing comes with it:
// nearer particles are larger and brighter.
var swarm struct {
	on bool
	s  dynamics.Swarm
	// cue is the depth cue last set on the program, so the per-frame reset
	// costs a WebGL call only when it changes.
	cue float32
}

// swarmCapable reports whether mode is a flow the swarm can carry: one whose
// field the registry can hand out, which is every flow integrate3D draws.
func swarmCapable(mode string) bool {
	if modeInfo[mode].Class != ClassFlow3D {
		return false
	}
	_, _, ok := dynamics.FlowFor(mode)
	return ok
}

// swarmStepsPerFrame is how many steps of the model's dt each particle takes
// a frame at SPEED 1. One was a quarter of a Lorenz time unit a second, and a
// ball spreading over the attractor needs about eight: half a minute of a
// thin arc. Four makes the spread a matter of seconds.
const swarmStepsPerFrame = 4

// generateSwarm advances the swarm one frame through deriv and draws it.
// Each particle takes swarmStepsPerFrame times the speed knob's sub-steps,
// held to the frame budget the same way a trail's points are.
func generateSwarm(dt float64, deriv dynamics.Deriv) {
	n := max(sim.steps, 2)
	ic := dynamics.InitCondFor(run.selectedMode)
	swarm.s.Ensure(run.selectedMode, n, dt, deriv, [3]float64{float64(ic[0]), float64(ic[1]), float64(ic[2])})
	steps := effSubSteps(swarmStepsPerFrame*max(sim.speedSteps, 1), n, frameBudgetCompiled)
	swarm.s.Step(deriv, dt*float64(sim.speedScale), steps, dynamics.UsingEuler())

	vertices := sim.vertBuf[:n*4]
	invN := float32(1) / float32(n-1)
	// The bounds by plain comparison: math.Min and math.Max, which handle NaN
	// and signed zero, were a fifth of the frame at twenty thousand points.
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i, p := range swarm.s.P {
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = float32(p[0]), float32(p[1]), float32(p[2]), float32(i)*invN
		for a := range 3 {
			if p[a] < lo[a] {
				lo[a] = p[a]
			}
			if p[a] > hi[a] {
				hi[a] = p[a]
			}
		}
	}
	gpu.uploadVerticesOnly(vertices, glctx.Types.Points, n)
	ext := math.Max(math.Max(hi[0]-lo[0], hi[1]-lo[1]), hi[2]-lo[2]) / 2
	gpu.setDepthCue(1, float32(ext))
}

// setDepthCue sets the shader's depth cue (0 is none) and the half-size depth
// is measured against. The render loop clears it every frame and the swarm
// sets it again, so no other model ever draws with it.
func (r *renderer) setDepthCue(cue, ext float32) {
	if !r.ready {
		return
	}
	if cue != swarm.cue {
		glctx.GL.Call("uniform1f", r.u.depthCue, cue)
		swarm.cue = cue
	}
	if cue != 0 {
		glctx.GL.Call("uniform1f", r.u.depthExtent, max(ext, 1e-6))
	}
}

// syncCloudButton dims POINTS' third button where it does nothing: on a model
// that is neither a flow the swarm can carry nor a map.
func syncCloudButton(mode string) {
	dimTrioButton("dash-duty", 2, !swarmCapable(mode))
}

// wireSwarm follows the swarm's switch. Every switch-on starts a fresh ball.
func wireSwarm() {
	sw := dom.Doc.Call("getElementById", "swarm-sw")
	if !sw.Truthy() {
		return
	}
	dom.On(sw, "change", func(this js.Value, _ []js.Value) any {
		swarm.on = sw.Get("checked").Bool()
		swarm.s.Reseed()
		return nil
	})
}
