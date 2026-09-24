//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
	"math/rand/v2"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/ripple"
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
	field     *ripple.Field
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
	damp      float32 // per-mille retained per step; see waterDamping
	spread    float32 // per-cent viscous smoothing
	edge      float32 // per-cent of each wave the walls send back
	drive     float32 // audio drive: how hard the sound pushes the surface
	srcs      float32 // how many speakers are in the tank
	rain      float32 // random drips per second, for a surface with weather
	heightArr js.Value
}

var water = waterLens{
	amount: 25,
	speed:  0.5,
	damp:   985,
	spread: 12,
	edge:   100,
	srcs:   1,
}

var (

	// waterW and waterH are the SIMULATION grid, not the canvas. A tank the
	// size of the viewport would be a million cells stepped every frame for a
	// pattern whose features are tens of pixels across; at 192 the wavelengths
	// that matter are still many cells wide and the step is a rounding error
	// in the frame budget.
	waterW = 192
	waterH = 192
)

// waterDamping converts the knob to the retained fraction. The useful range is
// narrow and near the top — 0.99 rings for seconds, 0.9 dies before a wave
// crosses the tank — so the knob is per-mille rather than a 0..1 dial nobody
// could place.
func (wa *waterLens) damping() float32 { return wa.damp / 1000 }

func init() {
	attractorParams["water"] = []paramDef{
		{"water-amount", "bend", &water.amount, 25, 0, 200, 1},
		{"water-speed", "speed", &water.speed, 0.5, 0.05, 0.7, 0.01},
		{"water-damp", "damp", &water.damp, 985, 900, 1000, 1},
		{"water-spread", "visc", &water.spread, 12, 0, 60, 1},
		{"water-edge", "walls", &water.edge, 100, 0, 100, 1},
		{"water-drive", "drive", &water.drive, 0, 0, 100, 1},
		{"water-srcs", "spkrs", &water.srcs, 1, 1, 4, 1},
		{"water-rain", "rain", &water.rain, 0, 0, 40, 1},
	}
}

// waterActive reports whether the lens is switched on.
func waterActive() bool { return bgVisual == "water" }

func (wa *waterLens) initWater() {
	if wa.ready {
		return
	}
	wa.field = ripple.New(waterW, waterH)
	wa.heightBuf = make([]byte, waterW*waterH*4)

	vs := `attribute vec2 aPos; varying vec2 vUV;
void main(){ vUV = aPos*0.5 + 0.5; gl_Position = vec4(aPos,0.0,1.0); }`

	// The height field arrives as RGBA bytes because WebGL1 without extensions
	// cannot sample a float texture. R and G carry the signed height split
	// across two bytes, which is more precision than the eye needs but keeps
	// the gradient smooth — quantizing to one byte puts visible stair-steps in
	// the refraction wherever the surface is nearly flat, which is most of it.
	fs := `precision highp float;
uniform sampler2D uScene;
uniform sampler2D uHeight;
uniform float uAmount;
uniform vec2 uTexel;
varying vec2 vUV;

float h(vec2 uv){
  vec4 t = texture2D(uHeight, uv);
  return (t.r + t.g/255.0) * 2.0 - 1.0;
}

void main(){
  // Central differences: the slope, which is what bends light.
  float hx = h(vUV + vec2(uTexel.x,0.0)) - h(vUV - vec2(uTexel.x,0.0));
  float hy = h(vUV + vec2(0.0,uTexel.y)) - h(vUV - vec2(0.0,uTexel.y));
  vec2 off = vec2(hx,hy) * uAmount * uTexel;
  vec2 uv = clamp(vUV + off, vec2(0.0), vec2(1.0));

  vec3 c = texture2D(uScene, uv).rgb;

  // A specular glint along the slope. Without it the surface is only a
  // distortion and reads as a bad lens; water is legible because the light it
  // reflects tells you where it is tilted.
  float g = clamp((hx+hy)*8.0, -1.0, 1.0);
  c += vec3(0.10,0.14,0.20) * max(g, 0.0);
  gl_FragColor = vec4(c, 1.0);
}`

	vsh := glctx.GL.Call("createShader", glctx.Types.VertexShader)
	glctx.GL.Call("shaderSource", vsh, vs)
	glctx.GL.Call("compileShader", vsh)
	fsh := glctx.GL.Call("createShader", glctx.Types.FragmentShader)
	glctx.GL.Call("shaderSource", fsh, fs)
	glctx.GL.Call("compileShader", fsh)
	wa.program = glctx.GL.Call("createProgram")
	glctx.GL.Call("attachShader", wa.program, vsh)
	glctx.GL.Call("attachShader", wa.program, fsh)
	glctx.GL.Call("linkProgram", wa.program)
	wa.aPos = glctx.GL.Call("getAttribLocation", wa.program, "aPos")
	wa.uScene = glctx.GL.Call("getUniformLocation", wa.program, "uScene")
	wa.uHeight = glctx.GL.Call("getUniformLocation", wa.program, "uHeight")
	wa.uAmount = glctx.GL.Call("getUniformLocation", wa.program, "uAmount")
	wa.uTexel = glctx.GL.Call("getUniformLocation", wa.program, "uTexel")

	verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	wa.quadBuf = glctx.GL.Call("createBuffer")
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, wa.quadBuf)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(verts), glctx.Types.StaticDraw)

	wa.sceneTex = newClampedTexture()
	wa.heightTex = newClampedTexture()

	wa.wireWaterPointer()
	wa.ready = true
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
		y := (ev.Get("clientY").Float() - r.Get("top").Float()) / h
		return float32(x) * float32(waterW), float32(y) * float32(waterH)
	}
	wa.downFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if !waterActive() || len(a) == 0 {
			return nil
		}
		wa.dragging = true
		wa.lastX, wa.lastY = pos(a[0])
		wa.field.Drop(wa.lastX, wa.lastY, 4, 0.6)
		return nil
	})
	wa.moveFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if !waterActive() || !wa.dragging || len(a) == 0 {
			return nil
		}
		x, y := pos(a[0])
		// A wake along the whole movement, not a dot at the end of it.
		wa.field.Line(wa.lastX, wa.lastY, x, y, 3, 0.5)
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
	for i := 0; i < n; i++ {
		x, y := waterSourcePos(i, n)
		wa.field.Drop(x, y, 5, amp)
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
		3, 0.5)
}

// drawWaterLens re-reads the finished frame through the surface.
func (wa *waterLens) drawWaterLens() {
	if !wa.ready {
		wa.initWater()
	}
	wa.field.Speed = wa.speed
	wa.field.Damping = wa.damping()
	wa.field.Spread = wa.spread / 100
	// 100 is a tank with walls and its own echoes; 0 is open water, where a wave
	// leaves and does not come back. Which one is right depends on whether the
	// disturbance or the interference is the thing being looked at.
	wa.field.Reflect = wa.edge / 100

	wa.driveWaterFromAudio()
	wa.rainOnWater()
	wa.field.Step()

	// The scene as it stands, into a texture.
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), wa.sceneTex)
	glctx.GL.Call("copyTexImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"), 0, 0, gpu.width, gpu.height, 0)

	wa.uploadWaterHeight()

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

	glctx.GL.Call("uniform1f", wa.uAmount, wa.amount)
	glctx.GL.Call("uniform2f", wa.uTexel, 1/float32(waterW), 1/float32(waterH))
	glctx.GL.Call("drawArrays", glctx.GL.Get("TRIANGLE_STRIP"), 0, 4)
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
