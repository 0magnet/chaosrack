//go:build js && wasm

package attractor

import "syscall/js"

// The js-side half of the mode registry: the generate dispatch map and the
// <select> builder. Generators register here (one call, next to the mode's
// code) instead of adding a case to a 30-arm switch.

var modeGenerate = map[string]func(){}

func registerGenerate(key string, fn func()) { modeGenerate[key] = fn }

func init() {
	// The classics all share one loop, keyed by the flow registry.
	for _, k := range []string{"lorenz", "rossler", "chua", "aizawa", "sprott",
		"thomas", "halvorsen", "chen", "dadras", "burkeshaw"} {
		k := k
		registerGenerate(k, func() { generateClassic(k) })
	}
	registerGenerate("lissajou", generateLissajou)
	registerGenerate("graphicartist", generateGraphicArtist)
	registerGenerate("hyperrossler", generateHyperRossler)
	registerGenerate("lu", generateLu)
	registerGenerate("sprotta", generateSprottA)
	registerGenerate("newtonleipnik", generateNewtonLeipnik)
	registerGenerate("rabinovich", generateRabinovich)
	registerGenerate("custom", generateCustom)
	registerGenerate("tetrahedron", generateTetrahedron)
	registerGenerate("cube", generateCube)
	registerGenerate("octahedron", generateOctahedron)
	registerGenerate("dodecahedron", generateDodecahedron)
	registerGenerate("icosahedron", generateIcosahedron)
	registerGenerate("nestedcube", generateNestedCube)
	registerGenerate("globe", generateGlobe)
	registerGenerate("sphere", generateSphere)
	registerGenerate("torus", generateTorus)
	registerGenerate("magnetosphere", generateMagnetosphere)
	// spectrogram / fvf / xy are dispatched before the generate lookup
	// (textured plane / bypass paths in generateForMode).
}

// buildModeSelect populates the (empty in the template) #mode-select from the
// registry's modeGroups layout — the nested category/model knobs then derive
// their structure from the same element, as before. Takes the panel node
// because it runs while the panel is still DETACHED from the document
// (getElementById can't see it yet).
func buildModeSelect(panel js.Value) {
	sel := panel.Call("querySelector", "#mode-select")
	if !sel.Truthy() {
		return
	}
	for _, g := range modeGroups {
		og := doc.Call("createElement", "optgroup")
		og.Set("label", g.Label)
		for _, k := range g.Keys {
			opt := doc.Call("createElement", "option")
			opt.Set("value", k)
			opt.Set("textContent", modeInfo[k].Label)
			if k == defaultMode {
				opt.Set("selected", true)
			}
			og.Call("appendChild", opt)
		}
		sel.Call("appendChild", og)
	}
}
