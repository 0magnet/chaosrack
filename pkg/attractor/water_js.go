//go:build js && wasm

package attractor

import (
	"math"
	"math/rand/v2"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/ripple"
	"github.com/go-gl/mathgl/mgl32"
)

// The water lens: the finished frame, seen through a disturbed fluid surface.
//
// This is a LENS rather than a model. Everything else in the rack draws
// something; this one takes whatever was drawn and looks at it through a
// medium, so it applies to the attractor, the terminal, the desk and the
// spectrogram alike without any of them knowing.
//
// It is the oldest kind of analog computer in here. A ripple tank computed
// diffraction and interference by having water do it, and was read by looking
// at the pattern — the answer was the image, not a number. The same tank is
// underneath this: pkg/ripple steps the surface, and the shader below reads
// the answer the way you read a tank, by looking through it.
//
// HOW THE FRAME IS RE-READ
//
// The scene is already on the drawing buffer by the time this runs, so it is
// copied into a texture with copyTexImage2D and the full-screen quad samples
// that. An offscreen framebuffer would let the scene be rendered straight into
// a texture and save the copy, but it would also mean every model's draw path
// having to target it — the copy costs one blit and changes nothing else, and
// this layer stays something that can be switched on over anything.
//
// WHERE THE SURFACE IS
//
// The water is looked down into, so nearer the eye is higher: the surface is
// the Fore knob's plane, the rack's own depth. What Fore puts behind the rack
// is under water and seen through it; what it brings in front stands out of
// the water, drawn sharp on the canvas above the panel and mirrored, faintly,
// in the surface below it; and where the model crosses the plane is the
// waterline, whose movement makes waves (traceWaterline).
//
// WHY THE GRADIENT AND NOT THE HEIGHT
//
// Refraction bends light by the surface SLOPE, so the sample offset comes from
// the gradient of the field, not its value. Offsetting by the height instead
// looks like a heat haze: the image swims where the water is high rather than
// where it is tilted, and still water with a standing offset is displaced when
// it should be clear.

// waterLens is the water lens: its program and textures, the height field it
// simulates, and the pointer and rain that disturb it.
type waterLens struct {
	field     ripple.Surface
	half      bool // the tank's step is begun and is finished this frame
	fresh     bool // the surface has moved since it was last uploaded
	ready     bool
	program   js.Value
	quadBuf   js.Value
	aPos      js.Value
	uScene    js.Value
	uHeight   js.Value
	uAmount   js.Value
	uTexel    js.Value
	sceneTex  js.Value
	heightTex js.Value
	heightBuf []byte // the field, quantized for upload

	// What stands out of the water: the near pass, copied off before the far
	// one overwrites it, for the surface to reflect. above says this frame
	// has one — with Fore at its far end the whole model is under water and
	// there is nothing over the surface to see in it.
	aboveTex    js.Value
	above       bool
	uAbove      js.Value
	uRefl       js.Value
	uReflScale  js.Value
	uReflCenter js.Value
	uFoam       js.Value // the bubbles' bend (water_foam_js.go)
	uFoamScale  js.Value
	uAspect2    js.Value // a cell's width over its height, squared

	// The waterline: the tank's cells the model crosses the surface in
	// (traceWaterline), this frame and the last.
	lit    []bool
	wasLit []bool
	// lined says this frame traced one. A frame that did not (the model all
	// on one side of the surface) empties wasLit, so the model next crossing
	// comes into the water with a splash rather than being compared with
	// where it crossed some time ago.
	lined bool

	// Drag state. The previous point is kept so a fast pointer leaves a wake
	// rather than a dotted line — see ripple.Line.
	dragging     bool
	lastX, lastY float32
	downFn       js.Func
	moveFn       js.Func
	upFn         js.Func

	// The controls. Named so that the panel and the permalink agree, and kept in
	// the same units the physics uses so a knob position means one thing.
	amount    float32 // how hard the lens bends, in texels at unit slope
	speed     float32 // wave speed, clamped to the CFL limit in Step
	damp      float32 // per-mille retained per step; see water.damping
	spread    float32 // per-cent viscous smoothing
	edge      float32 // per-cent of each wave the walls send back
	drive     float32 // audio drive: how hard the sound pushes the surface
	srcs      float32 // how many speakers are in the tank
	rain      float32 // random drips per second, for a surface with weather
	foam      float32 // FOAM in Layers · Colors: per-cent of the bubbles the waterline makes (water_foam_js.go)
	heightArr js.Value

	froth waterFoam // the bubbles on the surface (water_foam_js.go)
}

var water = waterLens{ // waterlens.go's defaults, which a native rack starts at too
	amount: WaterDefaults.Amount,
	speed:  WaterDefaults.Speed,
	damp:   WaterDefaults.Damp,
	spread: WaterDefaults.Spread,
	edge:   WaterDefaults.Edge,
	drive:  WaterDefaults.Drive,
	srcs:   WaterDefaults.Srcs,
	rain:   WaterDefaults.Rain,
}

var (

	// waterW and waterH are the SIMULATION grid, not the canvas. A tank the
	// size of the viewport would be a million cells stepped every frame for a
	// pattern whose features are tens of pixels across. WaterGrid for the
	// tank; WaterGridFD when the page asks for the old wave equation
	// (oldWaves), which ran at that.
	waterW = WaterGrid
	waterH = WaterGrid
)

// oldWaves reports whether the page was opened with waves=fd, which steps
// the old wave equation (ripple.Field) in place of the tank, to compare the
// two.
func oldWaves() bool {
	loc := js.Global().Get("location")
	return loc.Truthy() && js.Global().Get("URLSearchParams").New(loc.Get("search")).Call("get", "waves").String() == "fd"
}

// wcells is r cells of the grid the lens's disturbances were sized for
// (WaterGridFD), in this grid's cell heights: a splash is the same size on
// the screen whatever the tank is stepped at.
func wcells(r float32) float32 { return r * float32(waterH) / WaterGridFD }

// damping converts the knob to the retained fraction. The useful range is
// narrow and near the top — 0.99 rings for seconds, 0.9 dies before a wave
// crosses the tank — so the knob is per-mille rather than a 0..1 dial nobody
// could place.
func (wa *waterLens) damping() float32 { return wa.damp / 1000 }

// lensAmount is the bend knob in texels of this grid at unit slope. The knob
// counts texels of WaterGridFD's, the grid it was set on, so that a bend of
// 25 bends the frame as far whatever the tank is stepped at.
func (wa *waterLens) lensAmount() float32 { return wa.amount * wcells(1) }

func init() {
	// The Water group's knobs, from the shared table (waterlens.go).
	field := map[string]*float32{
		"water-amount": &water.amount, "water-speed": &water.speed, "water-damp": &water.damp,
		"water-spread": &water.spread, "water-edge": &water.edge, "water-drive": &water.drive,
		"water-srcs": &water.srcs, "water-rain": &water.rain,
	}
	for _, k := range WaterKnobs {
		attractorParams["water"] = append(attractorParams["water"], paramDef{k.ID, k.Label, field[k.ID], k.Def, k.Min, k.Max, k.Step})
		// The lens reads its knobs every frame and the model was made from
		// none of them, so turning one must not reset the model. It did:
		// every move reseeded the attractor to its initial condition and
		// restarted what is gathered from it — the live Lyapunov estimate,
		// a section's scatter, a map's orbit — and marked a mesh for
		// rebuilding.
		quietParams[k.ID] = true
	}
	// DRIVE's P: whether a drag across the picture makes waves. Off, the
	// pointer only turns the model, and the water moves as the model, the
	// sound and the rain move it, for watching the model and the water
	// alone.
	trioPrograms["water-drive"] = trioProgram{
		keys:     []string{"P", "", ""},
		switches: []string{"water-touch"},
		help:     []string{doc("trio.water-drive=0")},
		press:    func(int) { setSwitch("water-touch", !checkedOn("water-touch")) },
		lits:     func() []bool { return []bool{checkedOn("water-touch"), false, false} },
		drive:    []string{"water-touch"},
	}
}

// waterActive reports whether the lens is switched on.
func waterActive() bool { return bgVisual == "water" }

func (wa *waterLens) initWater() {
	if wa.ready {
		return
	}
	if oldWaves() {
		waterW, waterH = WaterGridFD, WaterGridFD
		wa.field = ripple.New(waterW, waterH)
	} else {
		waterW, waterH = tankAcross(float64(gpu.width)/float64(gpu.height)), WaterGrid
		wa.field = ripple.NewTank(waterW, waterH)
	}
	wa.heightBuf = make([]byte, waterW*waterH*4)

	vs := `attribute vec2 aPos; varying vec2 vUV;
void main(){ vUV = aPos*0.5 + 0.5; gl_Position = vec4(aPos,0.0,1.0); }`

	// The height field arrives as RGBA bytes because WebGL1 without extensions
	// cannot sample a float texture. R and G carry the signed height split
	// across two bytes, which is more precision than the eye needs but keeps
	// the gradient smooth — quantizing to one byte puts visible stair-steps in
	// the refraction wherever the surface is nearly flat, which is most of it.
	// There is no light but the model's. A glint used to brighten the faces
	// tilted toward one corner, from a lamp nothing in the scene stood for;
	// the model is what shines here, so what the surface shows besides the
	// refraction is the model's own light, reflected.
	fs := `precision highp float;
uniform sampler2D uScene;
uniform sampler2D uHeight;
uniform sampler2D uAbove;
uniform float uAmount;
uniform vec2 uTexel;
uniform float uRefl;
uniform float uReflScale;
uniform vec2 uReflCenter;
uniform sampler2D uFoam;
uniform float uFoamScale;
uniform float uAspect2;
varying vec2 vUV;

float h(vec2 uv){
  vec4 t = texture2D(uHeight, uv);
  return (t.r + t.g/255.0) * 2.0 - 1.0;
}

void main(){
  // Central differences: the slope, which is what bends light.
  float hx = h(vUV + vec2(uTexel.x,0.0)) - h(vUV - vec2(uTexel.x,0.0));
  float hy = h(vUV + vec2(0.0,uTexel.y)) - h(vUV - vec2(0.0,uTexel.y));
  // A cell is uAspect wide for 1 tall, so a difference across is that
  // much less steep, and a texel across that much more of the frame: the
  // same tilt bends as far either way only with the across scaled by both.
  vec2 off = vec2(hx / uAspect2, hy) * uAmount * uTexel;
  // The bubbles' menisci, each a small lens on the surface: their bend,
  // drawn already (water_foam_js.go), plus and minus each way.
  vec4 fo = texture2D(uFoam, vUV);
  off += vec2(fo.r - fo.g, fo.b - fo.a) * uFoamScale;
  vec2 uv = clamp(vUV + off, vec2(0.0), vec2(1.0));

  vec3 c = texture2D(uScene, uv).rgb;

  // What stands over the water, mirrored in it. The mirror image is as far
  // below the surface as the thing is above, so it is farther from the eye
  // and drawn smaller, toward the point the view looks at (uReflScale < 1).
  // A mirror turns a ray by twice the tilt, and the other way from the
  // refraction, so it wobbles with the same ripples, harder.
  if (uRefl > 0.0) {
    vec2 ruv = uReflCenter + (vUV - uReflCenter) / uReflScale - 2.0 * off;
    if (ruv.x >= 0.0 && ruv.x <= 1.0 && ruv.y >= 0.0 && ruv.y <= 1.0) {
      c += texture2D(uAbove, ruv).rgb * uRefl;
    }
  }
  gl_FragColor = vec4(c, 1.0);
}`

	wa.program = compileProgram(vs, fs)
	wa.aPos = glctx.GL.Call("getAttribLocation", wa.program, "aPos")
	wa.uScene = glctx.GL.Call("getUniformLocation", wa.program, "uScene")
	wa.uHeight = glctx.GL.Call("getUniformLocation", wa.program, "uHeight")
	wa.uAmount = glctx.GL.Call("getUniformLocation", wa.program, "uAmount")
	wa.uTexel = glctx.GL.Call("getUniformLocation", wa.program, "uTexel")
	wa.uAbove = glctx.GL.Call("getUniformLocation", wa.program, "uAbove")
	wa.uRefl = glctx.GL.Call("getUniformLocation", wa.program, "uRefl")
	wa.uReflScale = glctx.GL.Call("getUniformLocation", wa.program, "uReflScale")
	wa.uReflCenter = glctx.GL.Call("getUniformLocation", wa.program, "uReflCenter")
	wa.uFoam = glctx.GL.Call("getUniformLocation", wa.program, "uFoam")
	wa.uFoamScale = glctx.GL.Call("getUniformLocation", wa.program, "uFoamScale")
	wa.uAspect2 = glctx.GL.Call("getUniformLocation", wa.program, "uAspect2")

	verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	wa.quadBuf = glctx.GL.Call("createBuffer")
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, wa.quadBuf)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, glctx.SliceToTypedArray(verts), glctx.Types.StaticDraw)

	wa.sceneTex = newClampedTexture()
	wa.heightTex = newClampedTexture()
	wa.aboveTex = newClampedTexture()
	wa.lit = make([]bool, waterW*waterH)
	wa.wasLit = make([]bool, waterW*waterH)
	wa.initFoam()

	wa.wireWaterPointer()
	wa.ready = true
}

// compileProgram compiles and links a vertex and fragment shader, and says so
// on the console when either does not compile or the two do not link: a
// program that failed draws nothing, silently, which looks the same as a
// layer with nothing to show.
func compileProgram(vs, fs string) js.Value {
	gl := glctx.GL
	shader := func(kind js.Value, src string) js.Value {
		s := gl.Call("createShader", kind)
		gl.Call("shaderSource", s, src)
		gl.Call("compileShader", s)
		if !gl.Call("getShaderParameter", s, gl.Get("COMPILE_STATUS")).Bool() {
			js.Global().Get("console").Call("error", "shader:", gl.Call("getShaderInfoLog", s))
		}
		return s
	}
	p := gl.Call("createProgram")
	gl.Call("attachShader", p, shader(glctx.Types.VertexShader, vs))
	gl.Call("attachShader", p, shader(glctx.Types.FragmentShader, fs))
	gl.Call("linkProgram", p)
	if !gl.Call("getProgramParameter", p, gl.Get("LINK_STATUS")).Bool() {
		js.Global().Get("console").Call("error", "program:", gl.Call("getProgramInfoLog", p))
	}
	return p
}

// newClampedTexture makes a texture that samples cleanly at the edges. The
// default wrap is REPEAT, and a lens that samples past the edge would then
// show the opposite side of the screen smeared along the border.
func newClampedTexture() js.Value {
	t := glctx.GL.Call("createTexture")
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), t)
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MIN_FILTER"), glctx.GL.Get("LINEAR"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MAG_FILTER"), glctx.GL.Get("LINEAR"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_S"), glctx.GL.Get("CLAMP_TO_EDGE"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_T"), glctx.GL.Get("CLAMP_TO_EDGE"))
	return t
}

// wireWaterPointer lets a pointer drag through the surface.
//
// The listeners are installed once and read waterActive() rather than being
// added and removed with the layer: adding a listener per toggle is how a
// page ends up with forty of them, and a drag that starts before the layer is
// on should not be half-tracked.
func (wa *waterLens) wireWaterPointer() {
	el := glctx.GL.Get("canvas")
	if !el.Truthy() {
		return
	}
	pos := func(ev js.Value) (float32, float32) {
		r := el.Call("getBoundingClientRect")
		w, h := r.Get("width").Float(), r.Get("height").Float()
		if w <= 0 || h <= 0 {
			return 0, 0
		}
		x := (ev.Get("clientX").Float() - r.Get("left").Float()) / w
		// Up the screen, as the tank is read: its first row is uploaded
		// unflipped, so the shader finds it at the bottom of the frame.
		y := 1 - (ev.Get("clientY").Float()-r.Get("top").Float())/h
		return float32(x) * float32(waterW), float32(y) * float32(waterH)
	}
	wa.downFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		// DRIVE's P off: the pointer leaves the water alone.
		if !waterActive() || !checkedOn("water-touch") || len(a) == 0 {
			return nil
		}
		wa.dragging = true
		wa.lastX, wa.lastY = pos(a[0])
		wa.field.Drop(wa.lastX, wa.lastY, wcells(4), 0.6)
		return nil
	})
	wa.moveFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if !waterActive() || !wa.dragging || len(a) == 0 {
			return nil
		}
		x, y := pos(a[0])
		// A wake along the whole movement, not a dot at the end of it.
		wa.field.Line(wa.lastX, wa.lastY, x, y, wcells(3), 0.5)
		wa.lastX, wa.lastY = x, y
		return nil
	})
	wa.upFn = js.FuncOf(func(js.Value, []js.Value) any {
		wa.dragging = false
		return nil
	})
	el.Call("addEventListener", "pointerdown", wa.downFn)
	js.Global().Call("addEventListener", "pointermove", wa.moveFn)
	js.Global().Call("addEventListener", "pointerup", wa.upFn)
}

// waterSourcePos places the speakers in the tank. Spread around a circle
// rather than clustered, because two sources close together interfere at a
// scale finer than the grid and the pattern is just noise.
func waterSourcePos(i, n int) (float32, float32) {
	if n <= 1 {
		return float32(waterW) / 2, float32(waterH) / 2
	}
	th := 2 * math.Pi * float64(i) / float64(n)
	r := float64(waterW) * 0.28
	return float32(float64(waterW)/2 + r*math.Cos(th)),
		float32(float64(waterH)/2 + r*math.Sin(th))
}

// driveWaterFromAudio pushes the surface with the sound, as a speaker in the
// tank would. The envelope drives amplitude rather than the raw sample: a
// sample is as often negative as positive and at frame rate would push and
// pull at random, which averages to a surface that shivers without ever making
// a wave.
func (wa *waterLens) driveWaterFromAudio() {
	if wa.drive <= 0 {
		return
	}
	// "amp" is the smoothed level the other audio-reactive controls use, so the
	// surface responds to the same loudness everything else does.
	env := af.feat["amp"]
	if env <= 0 {
		return
	}
	amp := env * wa.drive / 100
	n := int(wa.srcs)
	for i := range n {
		x, y := waterSourcePos(i, n)
		wa.field.Drop(x, y, wcells(5), amp)
	}
}

// rainOnWater drips at random, so a surface with nothing else happening still
// has weather on it.
func (wa *waterLens) rainOnWater() {
	if wa.rain <= 0 {
		return
	}
	// The knob is drips per second; at frame rate that is a probability.
	//
	// math/rand, deliberately: this is where raindrops land on a decorative
	// surface, and crypto/rand would buy nothing but a syscall per frame.
	if rand.Float64() > float64(wa.rain)/60 { //nolint:gosec // decorative, not security
		return
	}
	wa.field.Drop(
		rand.Float32()*float32(waterW), //nolint:gosec // decorative, not security
		rand.Float32()*float32(waterH), //nolint:gosec // decorative, not security
		wcells(3), 0.5)
}

// drawWaterLens re-reads the finished frame through the surface.
func (wa *waterLens) drawWaterLens() {
	if !wa.ready {
		wa.initWater()
	}
	// 100 on the walls is a tank with walls and its own echoes; 0 is open
	// water, where a wave leaves and does not come back. Which one is right
	// depends on whether the disturbance or the interference is the thing
	// being looked at.
	wa.field.Tune(wa.speed, wa.damping(), wa.spread/100, wa.edge/100)
	if tk, ok := wa.field.(*ripple.Tank); ok {
		// The grid is square and the picture is not: its cells are as
		// wide as the picture is, against its height.
		tk.Aspect = float32(gpu.width) / float32(gpu.height) * float32(waterH) / float32(waterW)
	}

	wa.driveWaterFromAudio()
	wa.rainOnWater()
	if tk, ok := wa.field.(*ripple.Tank); ok {
		// Half a step a frame, each step two frames long: the tank's
		// step is too much for one frame (ripple.Tank.Begin).
		if wa.half {
			tk.Finish()
			wa.fresh = true
		} else {
			tk.Begin(2)
		}
		wa.half = !wa.half
	} else {
		wa.field.Step()
		wa.fresh = true
	}
	wa.froth.Step(wa.field)

	// The scene as it stands, into a texture.
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.sceneTex)
	glctx.GL.Call("copyTexImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"), 0, 0, gpu.width, gpu.height, 0)

	if wa.fresh {
		// Only a surface that has moved: on the frame a tank's step is
		// begun it has not, and the upload was 1.5 ms of a frame.
		wa.uploadWaterHeight()
		wa.fresh = false
	}
	foamScale := wa.bendFoam() // the bubbles' lenses, before the lens reads them

	glctx.GL.Call("disable", glctx.Types.DepthTest)
	glctx.GL.Call("useProgram", wa.program)
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, wa.quadBuf)
	glctx.GL.Call("enableVertexAttribArray", wa.aPos)
	glctx.GL.Call("vertexAttribPointer", wa.aPos, 2, glctx.Types.Float, false, 0, 0)

	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.sceneTex)
	glctx.GL.Call("uniform1i", wa.uScene, 0)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE1"))
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.heightTex)
	glctx.GL.Call("uniform1i", wa.uHeight, 1)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))

	glctx.GL.Call("uniform1f", wa.uAmount, wa.lensAmount())
	glctx.GL.Call("uniform2f", wa.uTexel, 1/float32(waterW), 1/float32(waterH))

	refl, scale := float32(0), float32(1)
	if wa.above {
		refl, scale = waterReflectance, waterMirrorScale()
		glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+waterAboveUnit)
		glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.aboveTex)
		glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	}
	glctx.GL.Call("uniform1i", wa.uAbove, waterAboveUnit)
	glctx.GL.Call("uniform1f", wa.uRefl, refl)
	glctx.GL.Call("uniform1f", wa.uReflScale, scale)
	glctx.GL.Call("uniform2f", wa.uReflCenter, 0.5, 0.5)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+waterFoamUnit)
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.froth.bendTex)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	glctx.GL.Call("uniform1i", wa.uFoam, waterFoamUnit)
	glctx.GL.Call("uniform1f", wa.uFoamScale, foamScale)
	asp := wa.field.CellAspect()
	glctx.GL.Call("uniform1f", wa.uAspect2, asp*asp)
	glctx.GL.Call("drawArrays", glctx.GL.Get("TRIANGLE_STRIP"), 0, 4)
	wa.drawFoam()    // the films' faint shine, on the surface
	wa.above = false // the next frame's near pass says again
	if !wa.lined {
		clear(wa.wasLit)
	}
	wa.lined = false
}

// waterAboveUnit is the texture unit the water's reflection reads from: past
// the trace's three (the picture, the colormap, the backdrop's palette), so
// copying the near pass onto it between the split passes disturbs none of
// what the far pass still has bound.
const waterAboveUnit = 3

// waterReflectance is how much of the model's light the surface sends back.
//
// Water looked at straight down reflects about 2% (Fresnel, n = 1.33), and 2%
// of a trace is a few levels of gray that nobody would see. This is that
// reflection made visible, not that reflection's strength: about seven times
// it, so the glow over the water reads while the model above it still
// plainly outshines its image.
const waterReflectance = 0.15

// waterMirrorScale is how much smaller the mirror image is drawn than what it
// mirrors. The surface is the Fore plane, D from the eye; what stands out of
// it reaches toward the eye by up to the model's extent, and its image lies as
// far behind the surface as it stands in front. Taking the half of the model
// above the water at its middle height h, the image is at D+h where the thing
// is at D-h, and perspective draws it smaller by (D-h)/(D+h).
func waterMirrorScale() float32 {
	d := -splitPlaneZ()
	h := splitExtent() * (1 + splitFrac) / 2
	if d <= 0 || h <= 0 || h >= d {
		return 1
	}
	return (d - h) / (d + h)
}

// captureAboveWater keeps what the near pass drew, which is all of the model
// that stands out of the water, for the surface to reflect.
func (wa *waterLens) captureAboveWater() {
	if !wa.ready {
		wa.initWater()
	}
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+waterAboveUnit)
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.aboveTex)
	glctx.GL.Call("copyTexImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"), 0, 0, gpu.width, gpu.height, 0)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	wa.above = true
}

// waterlineDepth is how far either side of the surface a POINT counts as at
// it, as a fraction of the model's extent. A line or a face crosses the
// surface somewhere exact; a map's dots never land on it, and are in the
// water's skin when they are this close.
const waterlineDepth = 0.03

// waterlinePush is how hard the model moves the water where it crosses into
// or out of a cell: far under a dragged finger's wake (wireWaterPointer),
// since a waterline is many cells long where a finger is one, and a moving
// attractor's changes every frame. At 0.15 a Lorenz figure half in the water
// churned all of it into marbling.
const waterlinePush = 0.04

// traceWaterline makes the model's waves: it finds where the model crosses
// the surface, from the vertices the frame was drawn from, and moves the
// water there.
//
// It is the waterline's MOVEMENT that makes waves, not the waterline. Water a
// still post stands in is still; a post moving through it pushes water aside
// in front and leaves a hollow behind. So a cell the model has come into this
// frame is pushed down and one it has left rises, and a model that stands
// still leaves the surface alone. Plunging it in (turning Fore) is a lot of
// cells at once: a splash, as it should be.
//
// On the CPU, not the card. Drawing the model once more around the surface
// and reading the picture back was the first way, and the read made the page
// wait for the card to finish everything queued: 2.1 s of every 3 in
// readPixels, 60 → 34 frames a second. The vertices are here anyway — they
// were uploaded from here — and an edge crosses the surface exactly where its
// two ends lie either side of it, which a picture could only approximate.
//
// z is the surface, in view space.
func (wa *waterLens) traceWaterline(z float32) {
	if !wa.ready {
		wa.initWater()
	}
	if splitAllInFront() {
		// The surface is behind all of the model: nothing crosses it, and
		// drawWaterLens forgets the last waterline.
		return
	}
	wa.lined = true
	clear(wa.lit)
	wa.markWaterline(z)

	var in, out []int
	for i, lit := range wa.lit {
		if lit != wa.wasLit[i] {
			if lit {
				in = append(in, i)
			} else {
				out = append(out, i)
			}
		}
	}
	wa.lit, wa.wasLit = wa.wasLit, wa.lit
	wa.makeFoam(in, out)
	wa.pushWaterline(in, -waterlinePush)
	wa.pushWaterline(out, waterlinePush)
}

// markWaterline sets wa.lit at each cell where the model last drawn crosses
// the plane z, read from what was uploaded for it: the trace's window of an
// attractor's buffer, or the mesh and the way its indices join.
//
// A model drawn by a path of its own (several draws a frame, its own
// program) leaves no waterline here, and so no waves but the pointer's and
// the rain's; its refraction and reflection are unaffected.
func (wa *waterLens) markWaterline(z float32) {
	v, s := gpu.verts, gpu.stride
	if s < 3 || len(v) < 2*s {
		return
	}
	mv := view.viewMat.Mul4(view.modelMat)
	pmv := gpu.proj.Mul4(mv)
	// Depth from the surface: the view-space z, the matrices' third row.
	depth := func(i int) float32 {
		o := i * s
		return mv[2]*v[o] + mv[6]*v[o+1] + mv[10]*v[o+2] + mv[14] - z
	}
	mark := func(x, y, zz float32) {
		c := pmv.Mul4x1(mgl32.Vec4{x, y, zz, 1})
		if c[3] <= 0 {
			return
		}
		// The bottom row first, as the tank's is: the pointer and the shader
		// both find its first row at the bottom of the frame.
		cx := int((c[0]/c[3]*0.5 + 0.5) * float32(waterW))
		cy := int((c[1]/c[3]*0.5 + 0.5) * float32(waterH))
		if cx >= 0 && cy >= 0 && cx < waterW && cy < waterH {
			wa.lit[cy*waterW+cx] = true
		}
	}
	n := len(v) / s
	edge := func(a, b int) {
		if a >= n || b >= n {
			return
		}
		da, db := depth(a), depth(b)
		if (da < 0) == (db < 0) {
			return
		}
		t := da / (da - db)
		oa, ob := a*s, b*s
		mark(v[oa]+t*(v[ob]-v[oa]), v[oa+1]+t*(v[ob+1]-v[oa+1]), v[oa+2]+t*(v[ob+2]-v[oa+2]))
	}
	point := func(i int) {
		if i >= n {
			return
		}
		if d := depth(i); d > -splitExtent()*waterlineDepth && d < splitExtent()*waterlineDepth {
			o := i * s
			mark(v[o], v[o+1], v[o+2])
		}
	}
	gl := glctx.Types

	if s == 4 { // an attractor's trace: drawArrays over a window of the buffer
		t := gpu.lastTrace
		if !t.ok {
			return
		}
		last := min(t.first+t.n, n)
		if t.mode.Equal(gl.Points) {
			for i := t.first; i < last; i++ {
				point(i)
			}
			return
		}
		for i := t.first; i+1 < last; i++ {
			edge(i, i+1)
		}
		return
	}

	ix := gpu.indices
	switch m := gpu.indexMode; {
	case m.Equal(gl.Triangles):
		for k := 0; k+2 < len(ix); k += 3 {
			a, b, c := int(ix[k]), int(ix[k+1]), int(ix[k+2])
			edge(a, b)
			edge(b, c)
			edge(c, a)
		}
	case m.Equal(gl.Lines):
		for k := 0; k+1 < len(ix); k += 2 {
			edge(int(ix[k]), int(ix[k+1]))
		}
	case m.Equal(gl.LineStrip), m.Equal(gl.LineLoop):
		for k := 0; k+1 < len(ix); k++ {
			edge(int(ix[k]), int(ix[k+1]))
		}
		if m.Equal(gl.LineLoop) && len(ix) > 2 {
			edge(int(ix[len(ix)-1]), int(ix[0]))
		}
	case m.Equal(gl.Points):
		for _, i := range ix {
			point(int(i))
		}
	}
}

// waterlineMaxDrops bounds the cells pushed a frame. A splash of thousands
// of cells is no more legible than one of a few hundred spread across it,
// and each push touches a dozen cells of the field.
const waterlineMaxDrops = 400

// pushWaterline moves the water at the cells given. Each is a small smooth
// bump rather than one cell, for the reason ripple.Drop gives: a single cell
// holds the wavelengths the grid cannot carry, and they shimmer in place.
func (wa *waterLens) pushWaterline(cells []int, amp float32) {
	if len(cells) == 0 {
		return
	}
	stride := (len(cells) + waterlineMaxDrops - 1) / waterlineMaxDrops
	for k := 0; k < len(cells); k += stride {
		i := cells[k]
		wa.field.Drop(float32(i%waterW), float32(i/waterW), wcells(2), amp)
	}
}

// uploadWaterHeight quantizes the field into the two-byte encoding the shader
// reads. Heights are clamped to ±1: a surface driven past that is already
// past where the refraction means anything, and letting it wrap would turn a
// loud passage into a field of tearing discontinuities.
func (wa *waterLens) uploadWaterHeight() {
	src := wa.field.Heights()
	for i, v := range src {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		u := (v + 1) * 0.5 * 65535
		hi := int(u) >> 8
		lo := int(u) & 0xff
		j := i * 4
		// hi is 0..255: v is clamped to ±1 above, so u is 0..65535.
		wa.heightBuf[j] = byte(hi) //nolint:gosec // range proved by the clamp above
		wa.heightBuf[j+1] = byte(lo)
		wa.heightBuf[j+2] = 0
		wa.heightBuf[j+3] = 255
	}
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.heightTex)
	js.CopyBytesToJS(wa.heightJS(), wa.heightBuf)
	glctx.GL.Call("texImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"),
		waterW, waterH, 0, glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), wa.heightJS())
}

func (wa *waterLens) heightJS() js.Value {
	if !wa.heightArr.Truthy() {
		wa.heightArr = js.Global().Get("Uint8Array").New(len(wa.heightBuf))
	}
	return wa.heightArr
}

// tankAcross is how many cells across the tank is on a picture aspect times
// as wide as it is tall, WaterGrid down: the power of two that comes nearest
// to square cells. The tank's waves are worked out in meters, so cells of
// any shape carry them true; but a cell twice as wide as it is tall is a
// grid half as fine across, and a splash small enough to be one cell wide
// was a spike that way, its fine fast ripples spreading it twice as far
// across as up.
func tankAcross(aspect float64) int {
	if !(aspect > 0) {
		return WaterGrid
	}
	n := WaterGrid * math.Exp2(math.Round(math.Log2(aspect)))
	return min(max(int(n), 32), 512)
}
