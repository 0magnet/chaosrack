//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/ripple"
)

// The foam on the water: bubbles the waterline brings up, floating on the
// surface, seen the way floating bubbles are seen.
//
// The waves the waterline makes spread off into the dark and are gone in a
// second. Foam is what stays: pkg/ripple's Foam floats each bubble where it
// came up and lets the water carry it until it bursts. This file brings
// them up along the waterline and draws them.
//
// HOW A FLOATING BUBBLE IS SEEN
//
// Not as a ring. A bubble's cap is a film a micron thick, and light goes
// through a film almost unbent, so the cap itself is all but invisible. What
// shows is the water round it (Selmke, "Bubble optics", 2020):
//
//   - the meniscus, the collar of water the bubble lifts round itself out
//     to about the capillary length, which bends what is under it inward
//     like a lens, so the scene beneath gathers into a bright rim; and
//   - the hollow the bubble makes under the surface, which bends the other
//     way, so through the bubble the scene is seen shrunk, as in a fisheye.
//
// That is exactly the refraction the water lens already does, with a sharper
// surface. So a bubble is drawn as its bend: each one adds the tilt of its
// meniscus and hollow to a texture (bendFoam), and the lens adds that to the
// waves' own tilt when it reads the scene. It is the same bending that made
// the first foam anyone saw here — the waterline's small domes, lenses on
// the trail, read as bubbles before any bubble was modeled — kept on the
// water and carried by it.
//
// The only light a bubble adds is the film's faint reflection at its rim,
// where it is seen edge-on (drawFoam): of the model's light, since there is
// no lamp.
//
// The texture holds the bend as four bytes — plus and minus along each axis
// — because WebGL1 cannot blend into a float target, and the bends of
// touching bubbles must add.

// waterFoam is the foam's surface and what draws it.
type waterFoam struct {
	surf *ripple.Foam

	// The bend: bubbles drawn as their meniscus's tilt into bendTex.
	bend      js.Value
	bendTex   js.Value
	bendFB    js.Value
	bendW     int
	bendH     int
	bB, bP    js.Value // x, y, sprite diameter, risen; radius and reach in pixels
	uBendGain js.Value
	uBendMax  js.Value
	uBendAsp  js.Value

	// The shine: the films' rims.
	shine   js.Value
	sB      js.Value // x, y, diameter, risen
	uScene  js.Value
	uAbove  js.Value
	uHasAb  js.Value
	uView   js.Value
	uGain   js.Value
	buf     js.Value
	verts   []float32
	vertsJS js.Value
}

// waterFoamMax is the most bubbles on the surface at once. A surface carrying
// this many is a sheet; more would not read as more foam.
const waterFoamMax = 1500

// waterFoamRate is the chance a cell the waterline moves through brings up a
// bubble, with the knob at 100.
const waterFoamRate = 0.1

// waterFoamStride is the floats a bubble takes in the vertex buffer.
const waterFoamStride = 6

// waterFoamUnit is the texture unit the lens reads the bubbles' bend from:
// past the water's reflection (waterAboveUnit).
const waterFoamUnit = 4

// The bubbles' bend, in the lens's units: a meniscus as high at its rim as
// waterMeniscus of the bubble's radius, for a bubble as wide as the
// capillary length (smaller ones lift less water, as their caps barely clear
// the surface); a hollow waterHollow of its radius deep; and the bend
// stored up to waterBendMax of the frame, which is also as far as it is
// taken.
const (
	waterMeniscus = 0.1
	waterHollow   = 0.1
	waterBendMax  = 0.03
)

// waterCapillary is the capillary length in the tank's cells (2.7 mm), how
// far a meniscus reaches.
const waterCapillary = 1.2

// waterShine is how much of the light falling on a film its rim sends to
// the eye: a film reflects a few per cent, more edge-on.
const waterShine = 0.25

func (wa *waterLens) initFoam() {
	fm := &wa.froth
	fm.surf = ripple.NewFoam(waterFoamMax, 1)
	fm.verts = make([]float32, 0, waterFoamMax*waterFoamStride)
	fm.vertsJS = js.Global().Get("Uint8Array").New(waterFoamMax * waterFoamStride * 4)
	gl := glctx.GL

	// The bend. A point sprite out to the meniscus's reach; inside the
	// bubble the hollow, outside it the meniscus, falling off over the
	// capillary length as a K0 Bessel function does, roughly. Height rising
	// outward bends the reading outward, which shrinks what is seen; falling
	// outward gathers it in.
	bvs := `attribute vec4 aB; attribute vec2 aP;
varying vec2 vP; varying float vRisen; varying float vD;
void main(){
  gl_Position = vec4(aB.xy*2.0-1.0, 0.0, 1.0);
  gl_PointSize = aB.z;
  vD = aB.z;
  vRisen = aB.w;
  vP = aP;
}`
	bfs := `precision highp float;
uniform float uGain; // tilt (height per cell) to bend (frame), over the most stored
uniform float uMax;
uniform float uAspect; // a cell's width over its height: a frame across is that many more cells
varying vec2 vP;     // radius, capillary length: in pixels
varying float vRisen;
varying float vD;
void main(){
  vec2 pc = vec2(gl_PointCoord.x, 1.0 - gl_PointCoord.y) * 2.0 - 1.0;
  float r = length(pc) * vD * 0.5;
  float rb = vP.x, a = vP.y;
  if (r > vD * 0.5 || r < 0.001) { discard; }
  vec2 dir = normalize(pc);
  // Heights and tilts in the bubble's radius; uGain carries the rest.
  float tilt;
  if (r < rb) {
    float q = r / rb;
    tilt = ` + glFloat(waterHollow) + ` * q / sqrt(max(1.0 - q*q, 0.05));
  } else {
    float lift = ` + glFloat(waterMeniscus) + ` * min(rb / a, 1.0);
    float h = lift * exp(-(r - rb) / a) * sqrt(rb / r);
    tilt = -h * rb * (1.0 / a + 0.5 / r);
    // Faded out at the sprite's edge rather than cut off there.
    tilt *= 1.0 - smoothstep(0.8, 1.0, r / (vD * 0.5));
  }
  vec2 b = clamp(dir / vec2(uAspect, 1.0) * tilt * uGain * vRisen / uMax, -1.0, 1.0);
  gl_FragColor = vec4(max(b.x, 0.0), max(-b.x, 0.0), max(b.y, 0.0), max(-b.y, 0.0));
}`
	fm.bend = compileProgram(bvs, bfs)
	fm.bB = gl.Call("getAttribLocation", fm.bend, "aB")
	fm.bP = gl.Call("getAttribLocation", fm.bend, "aP")
	fm.uBendGain = gl.Call("getUniformLocation", fm.bend, "uGain")
	fm.uBendMax = gl.Call("getUniformLocation", fm.bend, "uMax")
	fm.uBendAsp = gl.Call("getUniformLocation", fm.bend, "uAspect")
	fm.bendTex = newClampedTexture()
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), 1, 1, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), js.Global().Get("Uint8Array").New(4))
	fm.bendFB = gl.Call("createFramebuffer")

	// The shine: the rim of the film, lit by the model round it.
	svs := `attribute vec4 aB;
varying float vRisen;
void main(){
  gl_Position = vec4(aB.xy*2.0-1.0, 0.0, 1.0);
  gl_PointSize = aB.z;
  vRisen = aB.w;
}`
	sfs := `precision mediump float;
uniform sampler2D uScene;
uniform sampler2D uAbove;
uniform float uHasAbove;
uniform vec2 uView;
uniform float uGain;
varying float vRisen;

vec3 at(vec2 uv){
  return texture2D(uScene, uv).rgb + texture2D(uAbove, uv).rgb * uHasAbove;
}

void main(){
  vec2 pc = vec2(gl_PointCoord.x, 1.0 - gl_PointCoord.y) * 2.0 - 1.0;
  float d = length(pc);
  if (d > 1.0) { discard; }
  // The light falling on the film: the model round it, summed over a ring.
  vec2 uv = gl_FragCoord.xy / uView;
  vec3 light = vec3(0.0);
  for (int i = 0; i < 8; i++) {
    float a = float(i) * 0.785398;
    light += at(uv + vec2(cos(a), sin(a)) * 18.0 / uView);
  }
  light /= 3.0;
  // Only edge-on, at the rim, does a film reflect enough to see.
  float rim = smoothstep(0.75, 0.95, d) * (1.0 - smoothstep(0.95, 1.0, d));
  gl_FragColor = vec4(light * rim * vRisen * uGain, 1.0);
}`
	fm.shine = compileProgram(svs, sfs)
	fm.sB = gl.Call("getAttribLocation", fm.shine, "aB")
	fm.uScene = gl.Call("getUniformLocation", fm.shine, "uScene")
	fm.uAbove = gl.Call("getUniformLocation", fm.shine, "uAbove")
	fm.uHasAb = gl.Call("getUniformLocation", fm.shine, "uHasAbove")
	fm.uView = gl.Call("getUniformLocation", fm.shine, "uView")
	fm.uGain = gl.Call("getUniformLocation", fm.shine, "uGain")

	fm.buf = gl.Call("createBuffer")
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, fm.buf)
	gl.Call("bufferData", glctx.Types.ArrayBuffer, len(fm.verts[:cap(fm.verts)])*4, gl.Get("DYNAMIC_DRAW"))
}

// glFloat writes v as a GLSL float literal.
func glFloat(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

// makeFoam brings bubbles up where the waterline moved this frame: the cells
// the model came into and left, where it churns air into the water. A model
// standing still brings up none, as it makes no waves. They come up where
// they were made, and the water takes them from there.
func (wa *waterLens) makeFoam(in, out []int) {
	p := wa.foam / 100 * waterFoamRate
	if p <= 0 || len(in)+len(out) == 0 {
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
func (fm *waterFoam) Step(s ripple.Surface) {
	if fm.surf != nil {
		fm.surf.Step(s)
	}
}

// cellPx is a cell's height in pixels, which is what a bubble's radius is
// measured in: a bubble is round, and a cell on a wide canvas is not.
func cellPx() float32 {
	return float32(gpu.height) / float32(waterH)
}

// fillFoam puts each bubble in the vertex buffer: where it is, its sprite's
// diameter in pixels and how far it has risen, then its radius and the
// capillary length in pixels. reach says whether the sprite covers the
// meniscus too, as the bend's does, or just the bubble, as the shine's.
func (fm *waterFoam) fillFoam(reach bool) int {
	px := cellPx()
	a := waterCapillary * float32(waterH) / WaterGrid * px // the same 2.7 mm on either grid
	fm.verts = fm.verts[:0]
	for _, b := range fm.surf.Bubbles {
		rb := b.R * px
		d := 2 * rb
		if reach {
			d = 2 * (rb + 3*a)
		}
		fm.verts = append(fm.verts, b.X/float32(waterW), b.Y/float32(waterH), max(d, 2), b.Risen(), rb, a)
	}
	n := len(fm.verts) / waterFoamStride
	js.CopyBytesToJS(fm.vertsJS, sliceToByteSlice(fm.verts))
	gl := glctx.GL
	gl.Call("bindBuffer", glctx.Types.ArrayBuffer, fm.buf)
	gl.Call("bufferSubData", glctx.Types.ArrayBuffer, 0, fm.vertsJS.Call("subarray", 0, n*waterFoamStride*4))
	return n
}

// bendFoam draws the bubbles' bend into the texture the lens reads, and
// returns what the lens scales it by: 0 with no bubbles, when the texture
// is not drawn and not read.
func (wa *waterLens) bendFoam() float32 {
	fm := &wa.froth
	if fm.surf == nil || len(fm.surf.Bubbles) == 0 {
		return 0
	}
	gl := glctx.GL
	w, h := gpu.width, gpu.height
	if w != fm.bendW || h != fm.bendH {
		fm.bendW, fm.bendH = w, h
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), fm.bendTex)
		gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), w, h, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), js.Null())
		gl.Call("bindFramebuffer", gl.Get("FRAMEBUFFER"), fm.bendFB)
		gl.Call("framebufferTexture2D", gl.Get("FRAMEBUFFER"), gl.Get("COLOR_ATTACHMENT0"), gl.Get("TEXTURE_2D"), fm.bendTex, 0)
	}
	n := fm.fillFoam(true)

	// Everything else draws to the canvas itself, so that is what is put
	// back after, and the clear color the page keeps (clearScreen). Asking
	// the context what was bound instead made the page wait on the card
	// each frame: getParameter was a third of the frame.
	gl.Call("bindFramebuffer", gl.Get("FRAMEBUFFER"), fm.bendFB)
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", gl.Get("COLOR_BUFFER_BIT"))
	gl.Call("disable", glctx.Types.DepthTest)
	gl.Call("useProgram", fm.bend)
	gl.Call("enableVertexAttribArray", fm.bB)
	gl.Call("enableVertexAttribArray", fm.bP)
	gl.Call("vertexAttribPointer", fm.bB, 4, glctx.Types.Float, false, waterFoamStride*4, 0)
	gl.Call("vertexAttribPointer", fm.bP, 2, glctx.Types.Float, false, waterFoamStride*4, 16)
	// A tilt of one cell a cell bends the lens's reading by twice the lens's
	// amount in texels, as its two-cell difference does (water_js.go).
	gl.Call("uniform1f", fm.uBendGain, 2*wa.lensAmount()/float32(waterH))
	gl.Call("uniform1f", fm.uBendMax, waterBendMax)
	gl.Call("uniform1f", fm.uBendAsp, wa.field.CellAspect())
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("ONE"), gl.Get("ONE")) // touching bubbles' bends add
	gl.Call("drawArrays", glctx.Types.Points, 0, n)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
	gl.Call("disableVertexAttribArray", fm.bB)
	gl.Call("disableVertexAttribArray", fm.bP)
	gl.Call("bindFramebuffer", gl.Get("FRAMEBUFFER"), js.Null())
	gl.Call("clearColor", style.bgColor[0], style.bgColor[1], style.bgColor[2], 0)
	return waterBendMax
}

// drawFoam adds the films' rims onto the frame the lens just drew. The
// scene texture still holds the frame as it was before the lens, which is
// the light they are lit by.
func (wa *waterLens) drawFoam() {
	fm := &wa.froth
	if fm.surf == nil || len(fm.surf.Bubbles) == 0 {
		return
	}
	n := fm.fillFoam(false)
	gl := glctx.GL
	gl.Call("useProgram", fm.shine)
	gl.Call("enableVertexAttribArray", fm.sB)
	gl.Call("vertexAttribPointer", fm.sB, 4, glctx.Types.Float, false, waterFoamStride*4, 0)

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
	gl.Call("uniform1f", fm.uGain, waterShine)

	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("ONE"), gl.Get("ONE")) // a film adds light
	gl.Call("drawArrays", glctx.Types.Points, 0, n)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
	gl.Call("disableVertexAttribArray", fm.sB)
}
