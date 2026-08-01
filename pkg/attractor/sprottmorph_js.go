//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// Mode wiring for Sprott Morph (the coefficient-space machinery lives in
// sprottmorph.go). One continuous trajectory integrates while the 30
// quadratic coefficients slide between catalog systems — the self-
// programming analog computer re-patching itself mid-run. The sys knob
// parks the machine anywhere in the A…S cycle (fractional = between two
// systems); the rate knob makes it step itself, in systems per minute.

var (
	morphSysKnob float32 = 3 // catalog position the sys knob requests (3 = D)
	morphRate    float32 = 3 // self-programming speed, systems/minute

	morphSystems []sprottMorphSys
	morphM       float64 = 3 // live catalog position (knob + auto-advance)
	morphKnobPrv float32 = 3
	morphSX      float64
	morphSY      float64
	morphSZ      float64
	morphRing    []float64
	morphHead    int
	morphFill    int
	morphActive  bool
	morphLED     js.Value
	morphTick    int
)

// morphStep advances the trajectory one Euler tick under the blended flow,
// with a divergence guard that reseeds onto the blend's home IC.
func morphStep(c *[30]float64, dt float64) {
	dx, dy, dz := evalQuad(c, morphSX, morphSY, morphSZ)
	morphSX += dx * dt
	morphSY += dy * dt
	morphSZ += dz * dt
	bad := morphSX != morphSX || morphSY != morphSY || morphSZ != morphSZ ||
		morphSX < -60 || morphSX > 60 || morphSY < -60 || morphSY > 60 || morphSZ < -60 || morphSZ > 60
	if bad {
		i := int(morphM) % len(morphSystems)
		ic := morphSystems[i].ic
		morphSX = float64(ic[0]) + 0.01*jamRand()
		morphSY = float64(ic[1]) + 0.01*jamRand()
		morphSZ = float64(ic[2]) + 0.01*jamRand()
	}
}

// generateSprottMorph integrates the blended flow into the trail ring and
// keeps the PATCH readout current.
func generateSprottMorph() {
	if morphSystems == nil {
		morphSystems = sprottMorphSystems()
	}
	// The sys knob seizes the position when the user moves it; otherwise the
	// machine advances itself at the rate knob's systems-per-minute.
	if morphKnobPrv != morphSysKnob {
		morphKnobPrv = morphSysKnob
		morphM = float64(morphSysKnob)
	}
	n := speedSteps
	if n < 1 {
		n = 1
	}
	morphM += float64(morphRate) / 60 / 60 * float64(n) * float64(speedScale)
	for morphM >= float64(len(morphSystems)) {
		morphM -= float64(len(morphSystems))
	}
	c, dt, i, j, frac := morphBlend(morphSystems, morphM)
	if len(morphRing) != steps*3 {
		morphRing = make([]float64, steps*3)
		morphHead, morphFill = 0, 0
	}
	for s := 0; s < n; s++ {
		morphStep(&c, dt*float64(speedScale))
		morphRing[morphHead*3] = morphSX
		morphRing[morphHead*3+1] = morphSY
		morphRing[morphHead*3+2] = morphSZ
		morphHead = (morphHead + 1) % steps
		if morphFill < steps {
			morphFill++
		}
	}
	if morphFill < 2 {
		return
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	for k := 0; k < steps; k++ {
		age := steps - 1 - k
		idx := 0
		if age < morphFill {
			idx = (morphHead - 1 - age + steps + steps) % steps
		} else {
			idx = (morphHead - morphFill + steps + steps) % steps
		}
		v := k * 4
		vertices[v] = float32(morphRing[idx*3])
		vertices[v+1] = float32(morphRing[idx*3+1])
		vertices[v+2] = float32(morphRing[idx*3+2])
		vertices[v+3] = float32(k) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
	// PATCH readout: "D→E 42%" (throttled — DOM writes are not free).
	morphTick++
	if morphLED.Truthy() && morphTick%10 == 0 {
		// A dash, not an arrow — the DSEG LED font has no → glyph.
		txt := morphSystems[i].letter
		if frac >= 0.005 {
			txt += "-" + morphSystems[j].letter + " " + strconv.Itoa(int(frac*100+0.5)) + "%"
		}
		morphLED.Set("textContent", txt)
	}
}

// syncSprottMorphExtras: entry warms the ring on the blend at the knob and
// frames the camera from the warmed extent; while active, a PATCH readout
// joins the Console (the Graphic Artist extras pattern).
func syncSprottMorphExtras(mode string) {
	if ex := doc.Call("getElementById", "smorph-ui"); ex.Truthy() {
		ex.Get("parentNode").Call("removeChild", ex)
	}
	morphLED = js.Undefined()
	if mode != "sprottmorph" {
		morphActive = false
		return
	}
	if !morphActive {
		morphActive = true
		if morphSystems == nil {
			morphSystems = sprottMorphSystems()
		}
		morphM = float64(morphSysKnob)
		c, dt, _, _, _ := morphBlend(morphSystems, morphM)
		i := int(morphM) % len(morphSystems)
		ic := morphSystems[i].ic
		morphSX, morphSY, morphSZ = float64(ic[0]), float64(ic[1]), float64(ic[2])
		morphRing = make([]float64, steps*3)
		ext := 0.0
		for k := 0; k < steps; k++ {
			morphStep(&c, dt)
			morphRing[k*3] = morphSX
			morphRing[k*3+1] = morphSY
			morphRing[k*3+2] = morphSZ
			for _, v := range []float64{morphSX, morphSY, morphSZ} {
				if v > ext {
					ext = v
				}
				if -v > ext {
					ext = -v
				}
			}
		}
		morphHead, morphFill = 0, steps
		// Frame the warmed structure directly (no fit-ordering dependence).
		if ext < 0.5 {
			ext = 0.5
		}
		fitExtentOverride = float32(ext)
		dist := fitExtentOverride * 3
		if dist < 5 {
			dist = 5
		}
		initCameraDist = dist
		defaultCameraDist = dist
		updateViewMatrix()
	}
	swrow := doc.Call("querySelector", ".swrow")
	if !swrow.Truthy() {
		return
	}
	wrap := doc.Call("createElement", "div")
	wrap.Set("id", "smorph-ui")
	wrap.Set("className", "ga-waves grp")
	hdr := doc.Call("createElement", "div")
	hdr.Set("className", "ga-waves-hdr")
	hdr.Set("textContent", "PATCH")
	wrap.Call("appendChild", hdr)
	led := doc.Call("createElement", "span")
	led.Set("className", "led")
	led.Set("id", "smorph-led")
	led.Set("title", "Current patch — which catalog systems the machine is wired between right now (sys knob parks it, rate knob makes it step itself)")
	led.Set("textContent", "D")
	wrap.Call("appendChild", led)
	swrow.Call("appendChild", wrap)
	morphLED = led
}
