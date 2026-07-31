package attractor

// The "Edit equations" seed table: parseable (explicit-operator) forms of the
// built-in systems, untagged so the native guard in chaos_test.go can compare
// each seed against the mode's actual vector field — transcription drift here
// used to be invisible (the sprott seed kept pre-fix periodic parameters, and
// aizawa/dadras used a parameter named e, which the engine reads as Euler's
// constant). Note: e / pi / tau are RESERVED constant names — never use them
// as a parameter in a seed.

type builtinEq struct {
	eq     [4]string
	params map[string]float32
	useW   bool
	dt     float32
}

// builtinEquations gives parseable (explicit-operator) forms of the built-in
// systems so "Edit equations" can seed the editor. Not every mode is here;
// missing ones fall back to the default template.
var builtinEquations = map[string]builtinEq{
	"lorenz":        {[4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", ""}, map[string]float32{"sigma": 10, "rho": 28, "beta": 2.7}, false, 0.005},
	"rossler":       {[4]string{"-(y + z)", "x + a*y", "b + z*(x - c)", ""}, map[string]float32{"a": 0.2, "b": 0.2, "c": 5.7}, false, 0.005},
	"thomas":        {[4]string{"-b*x + sin(y)", "-b*y + sin(z)", "-b*z + sin(x)", ""}, map[string]float32{"b": 0.19}, false, 0.05},
	"halvorsen":     {[4]string{"-a*x - 4*y - 4*z - y^2", "-a*y - 4*z - 4*x - z^2", "-a*z - 4*x - 4*y - x^2", ""}, map[string]float32{"a": 1.4}, false, 0.003},
	"chen":          {[4]string{"a*(y - x)", "(c - a)*x - x*z + c*y", "x*y - b*z", ""}, map[string]float32{"a": 35, "b": 3, "c": 28}, false, 0.0005},
	"aizawa":        {[4]string{"(z - b)*x - d*y", "d*x + (z - b)*y", "c + a*z - z^3/3 - (x^2 + y^2)*(1 + k*z) + f*z*x^3", ""}, map[string]float32{"a": 0.95, "b": 0.7, "c": 0.6, "d": 3.5, "k": 0.25, "f": 0.1}, false, 0.0052},
	"hyperrossler":  {[4]string{"-y - z", "x + a*y + w", "b + x*z", "-c*z + d*w"}, map[string]float32{"a": 0.25, "b": 3, "c": 0.5, "d": 0.05}, true, 0.001},
	"chua":          {[4]string{"alpha*(y - x - (m1*x + 0.5*(m0 - m1)*(abs(x + 1) - abs(x - 1))))", "x - y + z", "-beta*y", ""}, map[string]float32{"alpha": 15.6, "beta": 28.0, "m0": -1.143, "m1": -0.714}, false, 0.005},
	"dadras":        {[4]string{"y - p*x + q*y*z", "r*y - x*z + z", "s*x*y - k*z", ""}, map[string]float32{"p": 3, "q": 2.7, "r": 1.7, "s": 2, "k": 9}, false, 0.005},
	"rabinovich":    {[4]string{"y*(z - 1 + x^2) + gamma*x", "x*(3*z + 1 - x^2) + gamma*y", "-2*z*(alpha + x*y)", ""}, map[string]float32{"alpha": 1.1, "gamma": 0.87}, false, 0.001},
	"burkeshaw":     {[4]string{"-s*(x + y)", "-y - s*x*z", "s*x*y + v", ""}, map[string]float32{"s": 10, "v": 4.272}, false, 0.005},
	"lu":            {[4]string{"a*(y - x)", "c*y - x*z", "x*y - b*z", ""}, map[string]float32{"a": 36, "b": 3, "c": 20}, false, 0.005},
	"sprotta":       {[4]string{"y", "-x + y*z", "1 - y^2", ""}, nil, false, 0.01},
	"newtonleipnik": {[4]string{"-a*x + y + 10*y*z", "-x - 0.4*y + 5*x*z", "b*z - 5*x*y", ""}, map[string]float32{"a": 0.4, "b": 0.175}, false, 0.005},
	"sprott":        {[4]string{"y + a*x*y + x*z", "1 - b*x^2 + y*z", "x - x^2 - y^2", ""}, map[string]float32{"a": 1.6, "b": 1.85}, false, 0.005},
	// Sprott 1994 catalog (explicit operators)
	"sprottb": {[4]string{"y*z", "x - y", "1 - x*y", ""}, nil, false, 0.01},
	"sprottc": {[4]string{"y*z", "x - y", "1 - x^2", ""}, nil, false, 0.01},
	"sprottd": {[4]string{"-y", "x + z", "x*z + 3*y^2", ""}, nil, false, 0.01},
	"sprotte": {[4]string{"y*z", "x^2 - y", "1 - 4*x", ""}, nil, false, 0.01},
	"sprottf": {[4]string{"y + z", "-x + 0.5*y", "x^2 - z", ""}, nil, false, 0.01},
	"sprottg": {[4]string{"0.4*x + z", "x*z - y", "-x + y", ""}, nil, false, 0.01},
	"sprotth": {[4]string{"-y + z^2", "x + 0.5*y", "x - z", ""}, nil, false, 0.01},
	"sprotti": {[4]string{"-0.2*y", "x + z", "x + y^2 - z", ""}, nil, false, 0.01},
	"sprottj": {[4]string{"2*z", "-2*y + z", "-x + y + y^2", ""}, nil, false, 0.01},
	"sprottk": {[4]string{"x*y - z", "x - y", "x + 0.3*z", ""}, nil, false, 0.01},
	"sprottl": {[4]string{"y + 3.9*z", "0.9*x^2 - y", "1 - x", ""}, nil, false, 0.01},
	"sprottm": {[4]string{"-z", "-x^2 - y", "1.7 + 1.7*x + y", ""}, nil, false, 0.005},
	"sprottn": {[4]string{"-2*y", "x + z^2", "1 + y - 2*z", ""}, nil, false, 0.01},
	"sprotto": {[4]string{"y", "x - z", "x + x*z + 2.7*y", ""}, nil, false, 0.005},
	"sprottp": {[4]string{"2.7*y + z", "-x + y^2", "x + y", ""}, nil, false, 0.01},
	"sprottq": {[4]string{"-z", "x - y", "3.1*x + y^2 + 0.5*z", ""}, nil, false, 0.002},
	"sprottr": {[4]string{"0.9 - y", "0.4 + z", "x*y - z", ""}, nil, false, 0.01},
	"sprotts": {[4]string{"-x - 4*y", "x + z^2", "1 + x", ""}, nil, false, 0.002},
}
