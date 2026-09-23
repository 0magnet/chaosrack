package dynamics

import (
	"fmt"
	"sort"
)

// The tunable constants of the systems, as data.
//
// These rows were in pkg/attractor, in a file tagged js && wasm, which is the
// only reason a host could not read them: nothing in a range and a default
// needs a browser. They describe pointers that already live here — LorenzR is
// in lorenz.go — so the table had been separated from the variables it
// describes by a build tag and a package boundary, and the one thing that
// could not be done without a browser was the one thing worth doing from a
// test: set a constant, integrate, and look at what changed.
//
// The panel still builds its knobs from these; see paramdefs_js.go, which now
// reads this table instead of restating it. There is one list.

// Param is a tunable constant: what to call it, where it lives, and the range
// a control may move it over.
type Param struct {
	ID    string // "lorenz-r", the id the panel, permalink and MIDI map use
	Label string // "ρ", what is printed under the knob
	Value *float32
	Def   float32
	Min   float32
	Max   float32
	Step  float32
}

// params is the registry, keyed by mode.
var params = map[string][]Param{
	"lorenz": {
		{"lorenz-dt", "dt", &LorenzDT, 0.005, 0.001, 0.05, 0.001},
		{"lorenz-s", "σ", &LorenzS, 10.0, 1, 30, 0.1},
		{"lorenz-r", "ρ", &LorenzR, 28.0, 1, 60, 0.1},
		{"lorenz-b", "β", &LorenzB, 2.7, 0.1, 10, 0.1},
	},
	"rossler": {
		{"rossler-dt", "dt", &RosslerDT, 0.005, 0.001, 0.05, 0.001},
		{"rossler-a", "a", &RosslerA, 0.2, 0.01, 1, 0.01},
		{"rossler-b", "b", &RosslerB, 0.2, 0.01, 1, 0.01},
		{"rossler-c", "c", &RosslerC, 5.7, 1, 20, 0.1},
	},
	"chua": {
		{"chua-dt", "dt", &ChuaDT, 0.005, 0.001, 0.05, 0.001},
		{"chua-alpha", "α", &ChuaAlpha, 15.6, 5, 30, 0.1},
		{"chua-beta", "β", &ChuaBeta, 28.0, 10, 50, 0.1},
		{"chua-m0", "m0", &ChuaM0, -1.143, -2, 0, 0.001},
		{"chua-m1", "m1", &ChuaM1, -0.714, -2, 0, 0.001},
	},
	"aizawa": {
		{"aizawa-dt", "dt", &AizawaDT, 0.0052, 0.001, 0.02, 0.0001},
		{"aizawa-a", "a", &AizawaA, 0.95, 0.1, 2, 0.01},
		{"aizawa-b", "b", &AizawaB, 0.7, 0.1, 2, 0.01},
		{"aizawa-c", "c", &AizawaC, 0.6, 0.1, 2, 0.01},
		{"aizawa-d", "d", &AizawaD, 3.5, 0.1, 8, 0.01},
		{"aizawa-e", "e", &AizawaE, 0.25, 0.01, 1, 0.01},
		{"aizawa-f", "f", &AizawaF, 0.1, 0.01, 1, 0.01},
	},
	"sprott": {
		{"sprott-dt", "dt", &SprottDT, 0.005, 0.001, 0.05, 0.001},
		{"sprott-a", "a", &SprottA, 1.6, 0.1, 5, 0.01},
		{"sprott-b", "b", &SprottB, 1.85, 0.1, 5, 0.01},
	},
	"thomas": {
		{"thomas-dt", "dt", &ThomasDT, 0.05, 0.001, 0.1, 0.001},
		{"thomas-b", "b", &ThomasB, 0.185, 0.01, 1.0, 0.001},
	},
	"halvorsen": {
		{"halvorsen-dt", "dt", &HalvorsenDT, 0.003, 0.001, 0.05, 0.001},
		{"halvorsen-a", "a", &HalvorsenA, 1.4, 0.1, 5, 0.01},
	},
	"chen": {
		{"chen-dt", "dt", &ChenDT, 0.0005, 0.0001, 0.005, 0.0001},
		{"chen-a", "a", &ChenA, 35.0, 10, 50, 0.1},
		{"chen-b", "b", &ChenB, 3.0, 0.1, 10, 0.1},
		{"chen-c", "c", &ChenC, 28.0, 10, 40, 0.1},
	},
	"dadras": {
		{"dadras-dt", "dt", &DadrasDT, 0.005, 0.001, 0.05, 0.001},
		{"dadras-p", "p", &DadrasP, 3.0, 0.1, 10, 0.1},
		{"dadras-q", "q", &DadrasQ, 2.7, 0.1, 10, 0.1},
		{"dadras-r", "r", &DadrasR, 1.7, 0.1, 10, 0.1},
		{"dadras-s", "s", &DadrasS, 2.0, 0.1, 10, 0.1},
		{"dadras-e", "e", &DadrasE, 9.0, 0.1, 20, 0.1},
	},
	"rabinovich": {
		{"rab-dt", "dt", &RabDT, 0.001, 0.0001, 0.01, 0.0001},
		{"rab-alpha", "α", &RabAlpha, 1.1, 0.01, 2, 0.01},
		{"rab-gamma", "γ", &RabGamma, 0.87, 0.01, 1, 0.01},
	},
	"burkeshaw": {
		{"burke-dt", "dt", &BurkeDT, 0.005, 0.001, 0.05, 0.001},
		{"burke-s", "S", &BurkeS, 10.0, 1, 20, 0.1},
		{"burke-v", "V", &BurkeV, 4.272, 1, 10, 0.001},
	},
}

// Params are the tunable constants of one mode, in panel order.
func Params(mode string) []Param { return params[mode] }

// ParamModes lists the modes that have tunable constants, in stable order.
func ParamModes() []string {
	out := make([]string, 0, len(params))
	for k := range params {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ParamByID finds a parameter by the id a control addresses it with.
func ParamByID(id string) (Param, bool) {
	for _, ps := range params {
		for _, p := range ps {
			if p.ID == id {
				return p, true
			}
		}
	}
	return Param{}, false
}

// SetParam moves a parameter the way a knob would, refusing a value the knob
// could not have produced.
//
// Clamping silently would make a test that sets rho to 600 pass while
// measuring rho=60, which is a worse answer than an error: the caller asked
// for something the instrument cannot do and should be told so.
func SetParam(id string, v float32) error {
	p, ok := ParamByID(id)
	if !ok {
		return fmt.Errorf("no parameter %q", id)
	}
	if v < p.Min || v > p.Max {
		return fmt.Errorf("%s = %g is outside %g..%g", id, v, p.Min, p.Max)
	}
	*p.Value = v
	return nil
}

// ResetParams puts every parameter back to its default, so a test that moved
// one does not leave it moved for the next.
func ResetParams() {
	for _, ps := range params {
		for _, p := range ps {
			*p.Value = p.Def
		}
	}
}
