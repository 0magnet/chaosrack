//go:build js && wasm

package attractor

import (
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

var (
	waterField *ripple.Field
	waterReady bool

	waterProgram js.Value
	waterQuadBuf js.Value
	waterAPos    js.Value
	waterUScene  js.Value
	waterUHeight js.Value
	waterUAmount js.Value
	waterUTexel  js.Value

	waterSceneTex  js.Value
	waterHeightTex js.Value
	waterHeightBuf []byte // the field, quantized for upload

	// waterW and waterH are the SIMULATION grid, not the canvas. A tank the
	// size of the viewport would be a million cells stepped every frame for a
	// pattern whose features are tens of pixels across; at 192 the wavelengths
	// that matter are still many cells wide and the step is a rounding error
	// in the frame budget.
	waterW = 192
	waterH = 192

	// Drag state. The previous point is kept so a fast pointer leaves a wake
	// rather than a dotted line — see ripple.Line.
	waterDragging          bool
	waterLastX, waterLastY float32
	waterDownFn            js.Func
	waterMoveFn            js.Func
	waterUpFn              js.Func
)

// The controls. Named so that the panel and the permalink agree, and kept in
// the same units the physics uses so a knob position means one thing.
var (
	waterAmount float32 = 25  // how hard the lens bends, in texels at unit slope
	waterSpeed  float32 = 0.5 // wave speed, clamped to the CFL limit in Step
	waterDamp   float32 = 985 // per-mille retained per step; see waterDamping
	waterSpread float32 = 12  // per-cent viscous smoothing
	waterEdge   float32 = 100 // per-cent of each wave the walls send back
	waterDrive  float32 = 0   // audio drive: how hard the sound pushes the surface
	waterSrcs   float32 = 1   // how many speakers are in the tank
	waterRain   float32 = 0   // random drips per second, for a surface with weather
)

// waterDamping converts the knob to the retained fraction. The useful range is
// narrow and near the top — 0.99 rings for seconds, 0.9 dies before a wave
// crosses the tank — so the knob is per-mille rather than a 0..1 dial nobody
// could place.
func waterDamping() float32 { return waterDamp / 1000 }

func init() {
	attractorParams["water"] = []paramDef{
		{"water-amount", "bend", &waterAmount, 25, 0, 200, 1},
		{"water-speed", "speed", &waterSpeed, 0.5, 0.05, 0.7, 0.01},
		{"water-damp", "damp", &waterDamp, 985, 900, 1000, 1},
		{"water-spread", "visc", &waterSpread, 12, 0, 60, 1},
		{"water-edge", "walls", &waterEdge, 100, 0, 100, 1},
		{"water-drive", "drive", &waterDrive, 0, 0, 100, 1},
		{"water-srcs", "spkrs", &waterSrcs, 1, 1, 4, 1},
		{"water-rain", "rain", &waterRain, 0, 0, 40, 1},
	}
}

// waterActive reports whether the lens is switched on.
func waterActive() bool { return bgVisual == "water" }

func initWater() {
	if waterReady {
		return
	}
	waterField = ripple.New(waterW, waterH)
	waterHeightBuf = make([]byte, waterW*waterH*4)

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

	vsh := gl.Call("createShader", glTypes.VertexShader)
	gl.Call("shaderSource", vsh, vs)
	gl.Call("compileShader", vsh)
	fsh := gl.Call("createShader", glTypes.FragmentShader)
	gl.Call("shaderSource", fsh, fs)
	gl.Call("compileShader", fsh)
	waterProgram = gl.Call("createProgram")
	gl.Call("attachShader", waterProgram, vsh)
	gl.Call("attachShader", waterProgram, fsh)
	gl.Call("linkProgram", waterProgram)
	waterAPos = gl.Call("getAttribLocation", waterProgram, "aPos")
	waterUScene = gl.Call("getUniformLocation", waterProgram, "uScene")
	waterUHeight = gl.Call("getUniformLocation", waterProgram, "uHeight")
	waterUAmount = gl.Call("getUniformLocation", waterProgram, "uAmount")
	waterUTexel = gl.Call("getUniformLocation", waterProgram, "uTexel")

	verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	waterQuadBuf = gl.Call("createBuffer")
	gl.Call("bindBuffer", glTypes.ArrayBuffer, waterQuadBuf)
	gl.Call("bufferData", glTypes.ArrayBuffer, SliceToTypedArray(verts), glTypes.StaticDraw)

	waterSceneTex = newClampedTexture()
	waterHeightTex = newClampedTexture()

	wireWaterPointer()
	waterReady = true
}

// newClampedTexture makes a texture that samples cleanly at the edges. The
// default wrap is REPEAT, and a lens that samples past the edge would then
// show the opposite side of the screen smeared along the border.
func newClampedTexture() js.Value {
	t := gl.Call("createTexture")
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), t)
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
	return t
}

// wireWaterPointer lets a pointer drag through the surface.
//
// The listeners are installed once and read waterActive() rather than being
// added and removed with the layer: adding a listener per toggle is how a
// page ends up with forty of them, and a drag that starts before the layer is
// on should not be half-tracked.
func wireWaterPointer() {
	el := gl.Get("canvas")
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
	waterDownFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if !waterActive() || len(a) == 0 {
			return nil
		}
		waterDragging = true
		waterLastX, waterLastY = pos(a[0])
		waterField.Drop(waterLastX, waterLastY, 4, 0.6)
		return nil
	})
	waterMoveFn = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if !waterActive() || !waterDragging || len(a) == 0 {
			return nil
		}
		x, y := pos(a[0])
		// A wake along the whole movement, not a dot at the end of it.
		waterField.Line(waterLastX, waterLastY, x, y, 3, 0.5)
		waterLastX, waterLastY = x, y
		return nil
	})
	waterUpFn = js.FuncOf(func(js.Value, []js.Value) any {
		waterDragging = false
		return nil
	})
	el.Call("addEventListener", "pointerdown", waterDownFn)
	js.Global().Call("addEventListener", "pointermove", waterMoveFn)
	js.Global().Call("addEventListener", "pointerup", waterUpFn)
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
func driveWaterFromAudio() {
	if waterDrive <= 0 {
		return
	}
	// "amp" is the smoothed level the other audio-reactive controls use, so the
	// surface responds to the same loudness everything else does.
	env := afFeat["amp"]
	if env <= 0 {
		return
	}
	amp := env * waterDrive / 100
	n := int(waterSrcs)
	for i := 0; i < n; i++ {
		x, y := waterSourcePos(i, n)
		waterField.Drop(x, y, 5, amp)
	}
}

// rainOnWater drips at random, so a surface with nothing else happening still
// has weather on it.
func rainOnWater() {
	if waterRain <= 0 {
		return
	}
	// The knob is drips per second; at frame rate that is a probability.
	if rand.Float64() > float64(waterRain)/60 {
		return
	}
	waterField.Drop(
		rand.Float32()*float32(waterW),
		rand.Float32()*float32(waterH),
		3, 0.5)
}

// drawWaterLens re-reads the finished frame through the surface.
func drawWaterLens() {
	if !waterReady {
		initWater()
	}
	waterField.Speed = waterSpeed
	waterField.Damping = waterDamping()
	waterField.Spread = waterSpread / 100
	// 100 is a tank with walls and its own echoes; 0 is open water, where a wave
	// leaves and does not come back. Which one is right depends on whether the
	// disturbance or the interference is the thing being looked at.
	waterField.Reflect = waterEdge / 100

	driveWaterFromAudio()
	rainOnWater()
	waterField.Step()

	// The scene as it stands, into a texture.
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), waterSceneTex)
	gl.Call("copyTexImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), 0, 0, width, height, 0)

	uploadWaterHeight()

	gl.Call("disable", glTypes.DepthTest)
	gl.Call("useProgram", waterProgram)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, waterQuadBuf)
	gl.Call("enableVertexAttribArray", waterAPos)
	gl.Call("vertexAttribPointer", waterAPos, 2, glTypes.Float, false, 0, 0)

	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), waterSceneTex)
	gl.Call("uniform1i", waterUScene, 0)
	gl.Call("activeTexture", gl.Get("TEXTURE1"))
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), waterHeightTex)
	gl.Call("uniform1i", waterUHeight, 1)
	gl.Call("activeTexture", gl.Get("TEXTURE0"))

	gl.Call("uniform1f", waterUAmount, waterAmount)
	gl.Call("uniform2f", waterUTexel, 1/float32(waterW), 1/float32(waterH))
	gl.Call("drawArrays", gl.Get("TRIANGLE_STRIP"), 0, 4)
}

// uploadWaterHeight quantizes the field into the two-byte encoding the shader
// reads. Heights are clamped to ±1: a surface driven past that is already
// past where the refraction means anything, and letting it wrap would turn a
// loud passage into a field of tearing discontinuities.
func uploadWaterHeight() {
	src := waterField.Heights()
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
		waterHeightBuf[j] = byte(hi)
		waterHeightBuf[j+1] = byte(lo)
		waterHeightBuf[j+2] = 0
		waterHeightBuf[j+3] = 255
	}
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), waterHeightTex)
	js.CopyBytesToJS(waterHeightJS(), waterHeightBuf)
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"),
		waterW, waterH, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), waterHeightJS())
}

var waterHeightArr js.Value

func waterHeightJS() js.Value {
	if !waterHeightArr.Truthy() {
		waterHeightArr = js.Global().Get("Uint8Array").New(len(waterHeightBuf))
	}
	return waterHeightArr
}
