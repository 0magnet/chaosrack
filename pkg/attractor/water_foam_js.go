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
// was made, slides it into the hollows between waves and bursts it seconds
// later. This file draws them.
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

// waterFoamGain is how much of the light falling on a film it sends to the
// eye, at its rim.
const waterFoamGain = 0.9

func (wa *waterLens) initFoam() {
	fm := &wa.froth
	fm.surf = ripple.NewFoam(waterFoamMax, 1)
	fm.verts = make([]float32, 0, waterFoamMax*4)
	fm.vertsJS = js.Global().Get("Uint8Array").New(waterFoamMax * 4 * 4)

	vs := `attribute vec4 aB; varying float vAlpha;
void main(){
  gl_Position = vec4(aB.xy*2.0-1.0, 0.0, 1.0);
  gl_PointSize = aB.z;
  vAlpha = aB.w;
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

vec3 at(vec2 uv){
  return texture2D(uScene, uv).rgb + texture2D(uAbove, uv).rgb * uHasAbove;
}

void main(){
  vec2 pc = vec2(gl_PointCoord.x, 1.0 - gl_PointCoord.y) * 2.0 - 1.0;
  float d = length(pc);
  if (d > 1.0) { discard; }

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
  float rim = smoothstep(0.55, 0.9, d) * (1.0 - smoothstep(0.9, 1.0, d));
  float film = 0.06;
  gl_FragColor = vec4(light * (rim * facing + film) * vAlpha * uGain, 1.0);
}`
	fm.program = compileProgram(vs, fs)
	fm.aB = glctx.GL.Call("getAttribLocation", fm.program, "aB")
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
// makes no waves.
func (wa *waterLens) makeFoam(in, out []int) {
	p := wa.foam / 100 * waterFoamRate
	if p <= 0 {
		return
	}
	fm := wa.froth.surf
	for _, cells := range [2][]int{in, out} {
		for _, i := range cells {
			if fm.Chance(p) {
				fm.Spawn(float32(i%waterW)+0.5, float32(i/waterW)+0.5)
			}
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
		fm.verts = append(fm.verts, b.X/float32(waterW), b.Y/float32(waterH), max(2*b.R*cellPx, 2), b.Fade())
	}
	n := len(fm.verts) / 4
	js.CopyBytesToJS(fm.vertsJS, sliceToByteSlice(fm.verts))

	gl := glctx.GL
	gl.Call("useProgram", fm.program)
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, fm.buf)
	gl.Call("bufferSubData", glctx.Types.ArrayBuffer, 0, fm.vertsJS.Call("subarray", 0, n*16))
	gl.Call("enableVertexAttribArray", fm.aB)
	gl.Call("vertexAttribPointer", fm.aB, 4, glctx.Types.Float, false, 0, 0)

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
}
