//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/ripple"
)

// The foam on the water: bubbles the waterline whips up, floating on the
// surface and lit by the model.
//
// The waves the waterline makes spread off into the dark and are gone in a
// second. Foam is what stays: pkg/ripple's Foam floats each bubble where it
// was made and lets the waves carry it until it bursts. This file throws
// them off the waterline and draws them: inflating, rocking on the waves,
// and tearing open when they burst.
//
// LIT BY THE MODEL
//
// There is no lamp. A bubble is a thin film, and what a film shows is the
// light falling on it, so each one takes its color and brightness from the
// model around it: the scene seen through the water, and what stands over
// the surface. The rim facing the brightest of that is the brightest, the
// way a bubble's highlight sits on the side toward the light, and a bubble
// far from the model is dark however much foam there is.
//
// A bubble is a point sprite, a ring with a faint film across it, added onto
// the frame: a film adds light, and the thinner it is the more of what is
// behind it shows through.

// waterFoam is the foam's surface and what draws it.
type waterFoam struct {
	surf    *ripple.Foam
	program js.Value
	buf     js.Value
	aB      js.Value // x, y (0..1 of the frame), diameter (pixels), presence
	aP      js.Value // how far through its burst, where on the rim it tears
	uScene  js.Value
	uAbove  js.Value
	uHasAb  js.Value
	uView   js.Value
	uGain   js.Value
	verts   []float32
	vertsJS js.Value
}

// waterFoamMax is the most bubbles on the surface at once. A surface carrying
// this many is a sheet; more would not read as more foam.
const waterFoamMax = 1500

// waterFoamRate is the chance a cell the waterline moves through makes a
// bubble, with the knob at 100.
const waterFoamRate = 0.1

// waterFoamThrow is how fast a bubble is thrown off the waterline, away from
// the model, in cells a step. It is what makes foam come FROM the model: a
// bubble left where it was made reads as a dot painted on the water.
const waterFoamThrow = 0.25

// waterFoamStride is the floats a bubble takes in the vertex buffer.
const waterFoamStride = 6

// waterFoamGain is how much of the light falling on a film it sends to the
// eye, at its rim.
const waterFoamGain = 0.9

func (wa *waterLens) initFoam() {
	fm := &wa.froth
	fm.surf = ripple.NewFoam(waterFoamMax, 1)
	fm.verts = make([]float32, 0, waterFoamMax*waterFoamStride)
	fm.vertsJS = js.Global().Get("Uint8Array").New(waterFoamMax * waterFoamStride * 4)

	vs := `attribute vec4 aB; attribute vec2 aP;
varying float vAlpha; varying vec2 vP;
void main(){
  gl_Position = vec4(aB.xy*2.0-1.0, 0.0, 1.0);
  gl_PointSize = aB.z;
  vAlpha = aB.w;
  vP = aP;
}`
	// The light at a bubble: the model around it, through the water and over
	// it, sampled in two rings; and which way the brightest of it lies.
	fs := `precision mediump float;
uniform sampler2D uScene;
uniform sampler2D uAbove;
uniform float uHasAbove;
uniform vec2 uView;
uniform float uGain;
varying float vAlpha;
varying vec2 vP; // burst 0..1, tear angle

vec3 at(vec2 uv){
  return texture2D(uScene, uv).rgb + texture2D(uAbove, uv).rgb * uHasAbove;
}

void main(){
  vec2 pc = vec2(gl_PointCoord.x, 1.0 - gl_PointCoord.y) * 2.0 - 1.0;
  float d = length(pc);
  if (d > 1.0) { discard; }
  // A burst: the film tears at one point of the rim and the tear runs round
  // both ways, so what is left is an arc closing up into nothing.
  float burst = vP.x;
  if (burst > 0.0) {
    float a = atan(pc.y, pc.x) - vP.y;
    a = abs(mod(a + 3.14159265, 6.2831853) - 3.14159265);
    if (a < burst * 3.2) { discard; }
  }

  vec2 uv = gl_FragCoord.xy / uView;
  vec3 light = vec3(0.0);
  vec2 toward = vec2(0.0);
  for (int i = 0; i < 8; i++) {
    float a = float(i) * 0.785398;
    vec2 dir = vec2(cos(a), sin(a));
    vec3 l = at(uv + dir * 18.0 / uView) + 0.5 * at(uv + dir * 54.0 / uView);
    light += l;
    toward += dir * dot(l, vec3(0.299, 0.587, 0.114));
  }
  // The light FALLING on the film: all of it, summed, not averaged — a
  // trace is thin lines on black, and an average of what is around a
  // bubble is mostly the black between them.
  light /= 3.0;
  float facing = 1.0;
  if (length(toward) > 0.0001 && d > 0.0001) {
    facing = 0.35 + 0.65 * max(dot(pc / d, normalize(toward)), 0.0);
  }

  // The wall of the film, seen edge-on at the rim, is where it is thickest
  // to the eye; across the middle it is nearly clear.
  // Bursting, the film is gone from the middle and the rim gathers into a
  // thin bright thread as it pulls back.
  float inner = mix(0.55, 0.8, burst);
  float rim = smoothstep(inner, 0.9, d) * (1.0 - smoothstep(0.9, 1.0, d)) * (1.0 + burst);
  float film = 0.06 * (1.0 - step(0.001, burst));
  gl_FragColor = vec4(light * (rim * facing + film) * vAlpha * uGain, 1.0);
}`
	fm.program = compileProgram(vs, fs)
	fm.aB = glctx.GL.Call("getAttribLocation", fm.program, "aB")
	fm.aP = glctx.GL.Call("getAttribLocation", fm.program, "aP")
	fm.uScene = glctx.GL.Call("getUniformLocation", fm.program, "uScene")
	fm.uAbove = glctx.GL.Call("getUniformLocation", fm.program, "uAbove")
	fm.uHasAb = glctx.GL.Call("getUniformLocation", fm.program, "uHasAbove")
	fm.uView = glctx.GL.Call("getUniformLocation", fm.program, "uView")
	fm.uGain = glctx.GL.Call("getUniformLocation", fm.program, "uGain")
	fm.buf = glctx.GL.Call("createBuffer")
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, fm.buf)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, len(fm.verts[:cap(fm.verts)])*4, glctx.GL.Get("DYNAMIC_DRAW"))
}

// makeFoam whips bubbles up where the waterline moved this frame: the cells
// the model came into and left. A model standing still makes none, as it
// makes no waves. Each is thrown outward, away from the middle of where the
// model cuts the water this frame, which is the way the water it churned
// goes.
func (wa *waterLens) makeFoam(in, out []int) {
	p := wa.foam / 100 * waterFoamRate
	if p <= 0 || len(in)+len(out) == 0 {
		return
	}
	var cx, cy float32
	for _, cells := range [2][]int{in, out} {
		for _, i := range cells {
			cx += float32(i % waterW)
			cy += float32(i / waterW)
		}
	}
	n := float32(len(in) + len(out))
	cx, cy = cx/n, cy/n
	fm := wa.froth.surf
	for _, cells := range [2][]int{in, out} {
		for _, i := range cells {
			if !fm.Chance(p) {
				continue
			}
			x, y := float32(i%waterW)+0.5, float32(i/waterW)+0.5
			dx, dy := x-cx, y-cy
			if l := float32(math.Hypot(float64(dx), float64(dy))); l > 0.5 {
				dx, dy = dx/l*waterFoamThrow, dy/l*waterFoamThrow
			} else {
				dx, dy = 0, 0
			}
			fm.Spawn(x, y, dx, dy)
		}
	}
}

// Step moves the foam one frame on the surface.
func (fm *waterFoam) Step(f *ripple.Field) {
	if fm.surf != nil {
		fm.surf.Step(f)
	}
}

// drawFoam adds the bubbles onto the frame the lens just drew. The scene
// texture still holds the frame as it was before the lens, which is the
// light the bubbles are lit by.
func (wa *waterLens) drawFoam() {
	fm := &wa.froth
	if fm.surf == nil || len(fm.surf.Bubbles) == 0 {
		return
	}
	// A cell is not square on a wide canvas; a bubble is round, so it is
	// sized by the cells' mean side.
	cellPx := float32(math.Sqrt(float64(gpu.width) / float64(waterW) * float64(gpu.height) / float64(waterH)))
	fm.verts = fm.verts[:0]
	for _, b := range fm.surf.Bubbles {
		// A burst throws the rim out a little as it pulls back.
		burst := b.Burst()
		d := 2 * b.R * b.Size() * (1 + 0.4*burst) * cellPx
		in := min(float32(b.Age)/6, 1)
		fm.verts = append(fm.verts, b.X/float32(waterW), b.Y/float32(waterH), max(d, 2), in, burst, b.Tear)
	}
	n := len(fm.verts) / waterFoamStride
	js.CopyBytesToJS(fm.vertsJS, sliceToByteSlice(fm.verts))

	gl := glctx.GL
	gl.Call("useProgram", fm.program)
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, fm.buf)
	gl.Call("bufferSubData", glctx.Types.ArrayBuffer, 0, fm.vertsJS.Call("subarray", 0, n*waterFoamStride*4))
	gl.Call("enableVertexAttribArray", fm.aB)
	gl.Call("enableVertexAttribArray", fm.aP)
	gl.Call("vertexAttribPointer", fm.aB, 4, glctx.Types.Float, false, waterFoamStride*4, 0)
	gl.Call("vertexAttribPointer", fm.aP, 2, glctx.Types.Float, false, waterFoamStride*4, 16)

	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), wa.sceneTex)
	gl.Call("uniform1i", fm.uScene, 0)
	has := float32(0)
	if wa.above {
		has = 1
	}
	gl.Call("activeTexture", gl.Get("TEXTURE0").Int()+waterAboveUnit)
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), wa.aboveTex)
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("uniform1i", fm.uAbove, waterAboveUnit)
	gl.Call("uniform1f", fm.uHasAb, has)
	gl.Call("uniform2f", fm.uView, gpu.width, gpu.height)
	gl.Call("uniform1f", fm.uGain, waterFoamGain)

	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("ONE"), gl.Get("ONE")) // a film adds light
	gl.Call("drawArrays", glctx.Types.Points, 0, n)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
	gl.Call("disableVertexAttribArray", fm.aB)
	gl.Call("disableVertexAttribArray", fm.aP)
}
