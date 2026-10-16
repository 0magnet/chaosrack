package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// A model's parameters as a rack without the page names them: in the order
// the page lists them (attractorParams), with the labels its knobs carry.
// The Mod matrix's rows are a model's parameters in this order, so a native
// rack needs it; the ranges are the controls' own. The flows' come from
// pkg/dynamics, as the page's do; the rest are here, held to the page's
// tables by a test.

// ModelParam is one of a model's parameters: its control and its label.
type ModelParam struct{ ID, Label string }

// modelParams are the parameters of the models whose tables are the page's
// own rather than pkg/dynamics'.
var modelParams = map[string][]ModelParam{
	"bifurcation":   {{"bif-depth", "depth"}},
	"bounceball":    {{"bounce-grav", "grav"}, {"bounce-rest", "bounce"}, {"bounce-drift", "drift"}, {"bounce-height", "height"}},
	"fvf":           {{"fvf-gain", "gain"}, {"fvf-offset", "offset"}, {"fvf-fmin", "fmin"}, {"fvf-fmax", "fmax"}, {"fvf-duty", "duty"}, {"fvf-mix", "mix"}, {"fvf-glide", "glide"}},
	"globe":         {{"globe-lat", "lat"}, {"globe-lon", "lon"}, {"globe-par", "par"}, {"globe-rev", "dir"}, {"globe-twist", "twist"}},
	"graphicartist": {{"ga-la", "LvA"}, {"ga-lb", "LvB"}, {"ga-ld", "LvD"}, {"ga-hb", "HmB"}, {"ga-hc", "HmC"}, {"ga-hd", "HmD"}, {"ga-wa", "WvA"}, {"ga-wb", "WvB"}, {"ga-wc", "WvC"}, {"ga-wd", "WvD"}},
	"lattice":       {{"lattice-shape", "shp"}, {"lattice-spin", "spin"}, {"lattice-tilt", "tilt"}},
	"lissajou":      {{"lissajou-a", "a"}, {"lissajou-b", "b"}, {"lissajou-c", "c"}},
	"poincare":      {{"sect-axis", "axis"}, {"sect-pos", "pos"}, {"sect-dir", "dir"}, {"sect-view", "view"}},
	"polar":         {{"polar-chan", "src"}, {"polar-map", "map"}, {"polar-drive", "drv"}, {"takens-tau", "τ"}, {"polar-win", "win"}, {"polar-gain", "gain"}, {"takens-smooth", "smth"}},
	"polyhedron":    {{"polyhedron-p", "sides"}, {"polyhedron-q", "meet"}, {"polyhedron-morph", "morph"}, {"polyhedron-op", "op"}, {"polyhedron-kis", "kis"}},
	"pong":          {{"pong-speed", "speed"}, {"pong-paddle", "paddle"}, {"pong-skill", "skill"}, {"pong-pad-l", "left"}, {"pong-pad-r", "right"}},
	"recurrence":    {{"rec-src", "src"}, {"rec-win", "win"}, {"rec-eps", "ε"}, {"rec-dim", "m"}, {"takens-tau", "τ"}},
	"rta":           {{"rta-frac", "band"}, {"rta-chan", "src"}, {"rta-top", "top"}, {"rta-range", "rnge"}, {"rta-avg", "avg"}, {"rta-hold", "hold"}},
	"scopetext":     {{"stext-harm", "harm"}},
	"spectrogram":   {{"spect-dft", "dft"}, {"spect-ovl", "ovlp"}, {"spect-win", "wfn"}, {"spect-chan", "chan"}, {"spect-scale", "scale"}, {"spect-min", "min"}, {"spect-max", "max"}},
	"sprottmorph":   {{"smorph-sys", "sys"}, {"smorph-rate", "rate"}},
	"stereo":        {{"stereo-axes", "axes"}, {"stereo-tau", "τ"}, {"stereo-win", "win"}, {"stereo-gain", "gain"}, {"stereo-align", "algn"}, {"stereo-width", "wide"}, {"stereo-vg", "vg"}, {"stereo-span", "span"}, {"stereo-trig", "trig"}, {"stereo-lvl", "lvl"}, {"stereo-tsrc", "tsrc"}, {"stereo-tcpl", "tcpl"}, {"stereo-hyst", "hyst"}, {"stereo-trun", "run"}, {"stereo-hold", "hold"}, {"stereo-tpos", "tpos"}, {"stereo-grat", "grid"}, {"takens-smooth", "smth"}},
	"takens":        {{"takens-chan", "src"}, {"takens-tau", "τ"}, {"takens-win", "win"}, {"takens-gain", "gain"}, {"takens-smooth", "smth"}},
	"torus":         {{"torus-R", "R"}, {"torus-r", "r"}, {"torus-stacks", "stacks"}, {"torus-slices", "slices"}, {"torus-roll", "roll"}},
	"turtle":        {{"turtle-mod", "mod"}, {"turtle-seq", "seq"}, {"turtle-mul", "mul"}, {"turtle-cap", "cap"}, {"turtle-dim", "dim"}, {"turtle-tint", "tint"}, {"turtle-trail", "trail"}, {"turtle-cam", "cam"}, {"turtle-view", "view"}, {"turtle-cycle", "cycle"}, {"turtle-turn", "turn"}},
	"waterfall":     {{"wfall-src", "src"}, {"wfall-chan", "chan"}, {"wfall-lines", "line"}, {"wfall-step", "step"}, {"wfall-fft", "fft"}, {"wfall-top", "top"}, {"wfall-range", "rnge"}, {"wfall-depth", "dpth"}, {"wfall-swap", "ref"}},
	"xfer":          {{"xf-swap", "ref"}, {"xf-frac", "band"}, {"xf-avg", "avg"}, {"xf-range", "rnge"}, {"xf-coh", "coh"}, {"xf-show", "show"}},
	"xy":            {{"xy-basis", "axes"}, {"xy-gain", "gain"}, {"xy-win", "win"}, {"xy-persist", "glow"}, {"xy-lag", "lag"}, {"xy-smooth", "smth"}},
}

// ModelParams are model's parameters in the page's order; Custom's are
// whatever its equations name, and are not here.
func ModelParams(model string) []ModelParam {
	if ps, ok := modelParams[model]; ok {
		return ps
	}
	var out []ModelParam
	for _, p := range dynamics.Params(model) {
		out = append(out, ModelParam{p.ID, p.Label})
	}
	return out
}
