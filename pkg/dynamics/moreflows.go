package dynamics

// More chaotic flows, from the physics, engineering, biology and economics
// literature: each a published system at its published constants, checked to
// be chaotic there (TestEveryDynamicalModeMeasuresChaotic), integrated by the
// shared RK4 loop, which puts a state that escapes back at its initial
// condition. One table, so a system is one row and one field.

// MoreFlow is one of them: its mode key, the step and the field, its initial
// condition and its knobs.
type MoreFlow struct {
	Key   string
	DT    *float32
	Deriv Deriv
	IC    [3]float32
	Knobs []Param
}

// The constants, by system. Each is a knob (MoreFlows), so each is a
// variable the field reads.
var (
	RucklidgeDT float32 = 0.005
	RucklidgeK  float32 = 2
	RucklidgeL  float32 = 6.7

	GenesioDT float32 = 0.01
	GenesioA  float32 = 0.44
	GenesioB  float32 = 1.1
	GenesioC  float32 = 1

	ShimizuDT float32 = 0.01
	ShimizuA  float32 = 0.375
	ShimizuL  float32 = 0.81

	RikitakeDT float32 = 0.005
	RikitakeMu float32 = 2
	RikitakeA  float32 = 5

	MooreDT float32 = 0.005
	MooreT  float32 = 6
	MooreR  float32 = 20

	BoualiDT    float32 = 0.005
	BoualiA     float32 = 4
	BoualiB     float32 = 1
	BoualiC     float32 = 1.5
	BoualiS     float32 = 1
	BoualiAlpha float32 = 0.3
	BoualiBeta  float32 = 0.05
	Lorenz84DT  float32 = 0.005
	Lorenz84A   float32 = 0.25
	Lorenz84B   float32 = 4
	Lorenz84F   float32 = 8
	Lorenz84G   float32 = 1
	FinanceDT   float32 = 0.005
	FinanceA    float32 = 0.9
	FinanceB    float32 = 0.2
	FinanceC    float32 = 1.2
	HindmarshDT float32 = 0.01
	HindmarshI  float32 = 3.25
	HindmarshR  float32 = 0.006
	HindmarshS  float32 = 4
	WeiDT       float32 = 0.005
	WeiA        float32 = 0.35
	JerkDT      float32 = 0.005
	JerkA       float32 = 2.017
)

// MoreFlows are the systems, in the order the MODEL knob turns through them.
var MoreFlows = []MoreFlow{
	// Rucklidge (1992): double convection, a fluid layer heated from below
	// in a magnetic field or a rotating frame.
	{"rucklidge", &RucklidgeDT, func(x, y, z float64) (float64, float64, float64) {
		k, l := float64(RucklidgeK), float64(RucklidgeL)
		return -k*x + l*y - y*z, x, -z + y*y
	}, [3]float32{1, 0, 4.5}, []Param{
		{"rucklidge-dt", "dt", &RucklidgeDT, 0.005, 0.001, 0.02, 0.001},
		{"rucklidge-k", "κ", &RucklidgeK, 2, 0.5, 5, 0.01},
		{"rucklidge-l", "λ", &RucklidgeL, 6.7, 2, 12, 0.01},
	}},
	// Genesio and Tesi (1992): a jerk system from control theory, the
	// harmonic-balance study of when an oscillator turns chaotic.
	{"genesiotesi", &GenesioDT, func(x, y, z float64) (float64, float64, float64) {
		a, b, c := float64(GenesioA), float64(GenesioB), float64(GenesioC)
		return y, z, -c*x - b*y - a*z + x*x
	}, [3]float32{0.1, 0.1, 0.1}, []Param{
		{"genesio-dt", "dt", &GenesioDT, 0.01, 0.001, 0.05, 0.001},
		{"genesio-a", "a", &GenesioA, 0.44, 0.1, 1, 0.01},
		{"genesio-b", "b", &GenesioB, 1.1, 0.5, 2, 0.01},
		{"genesio-c", "c", &GenesioC, 1, 0.5, 2, 0.01},
	}},
	// Shimizu and Morioka (1980): a Lorenz-like system whose butterfly
	// grows a lip.
	{"shimizumorioka", &ShimizuDT, func(x, y, z float64) (float64, float64, float64) {
		a, l := float64(ShimizuA), float64(ShimizuL)
		return y, x - l*y - x*z, -a*z + x*x
	}, [3]float32{0.1, 0, 0}, []Param{
		{"shimizu-dt", "dt", &ShimizuDT, 0.01, 0.001, 0.05, 0.001},
		{"shimizu-a", "α", &ShimizuA, 0.375, 0.1, 1, 0.005},
		{"shimizu-l", "λ", &ShimizuL, 0.81, 0.3, 1.5, 0.01},
	}},
	// Rikitake (1958): two coupled disk dynamos, a model of the Earth's
	// magnetic field reversing at irregular intervals.
	{"rikitake", &RikitakeDT, func(x, y, z float64) (float64, float64, float64) {
		mu, a := float64(RikitakeMu), float64(RikitakeA)
		return -mu*x + z*y, -mu*y + (z-a)*x, 1 - x*y
	}, [3]float32{1, 0, 0.5}, []Param{
		{"rikitake-dt", "dt", &RikitakeDT, 0.005, 0.001, 0.02, 0.001},
		{"rikitake-mu", "μ", &RikitakeMu, 2, 0.5, 5, 0.01},
		{"rikitake-a", "a", &RikitakeA, 5, 1, 10, 0.05},
	}},
	// Moore and Spiegel (1966): a parcel of gas oscillating in a star's
	// convecting, radiating atmosphere.
	{"moorespiegel", &MooreDT, func(x, y, z float64) (float64, float64, float64) {
		t, r := float64(MooreT), float64(MooreR)
		return y, z, -z - (t-r+r*x*x)*y - t*x
	}, [3]float32{0.1, 0, 0}, []Param{
		{"moore-dt", "dt", &MooreDT, 0.005, 0.001, 0.02, 0.001},
		{"moore-t", "T", &MooreT, 6, 1, 15, 0.05},
		{"moore-r", "R", &MooreR, 20, 5, 40, 0.1},
	}},
	// Bouali (2012): a strange attractor with a stretched loop.
	{"bouali", &BoualiDT, func(x, y, z float64) (float64, float64, float64) {
		a, b, c, s := float64(BoualiA), float64(BoualiB), float64(BoualiC), float64(BoualiS)
		al, be := float64(BoualiAlpha), float64(BoualiBeta)
		return x*(a-y) + al*z, -y * (b - x*x), -x*(c-s*z) - be*z
	}, [3]float32{1, 1, 0}, []Param{
		{"bouali-dt", "dt", &BoualiDT, 0.005, 0.001, 0.02, 0.001},
		{"bouali-a", "a", &BoualiA, 4, 1, 8, 0.05},
		{"bouali-b", "b", &BoualiB, 1, 0.2, 3, 0.01},
		{"bouali-c", "c", &BoualiC, 1.5, 0.5, 3, 0.01},
		{"bouali-s", "s", &BoualiS, 1, 0.2, 3, 0.01},
		{"bouali-alpha", "α", &BoualiAlpha, 0.3, 0, 1, 0.01},
		{"bouali-beta", "β", &BoualiBeta, 0.05, 0, 0.5, 0.005},
	}},
	// Lorenz (1984): the general circulation of the atmosphere, the westerly
	// jet and its eddies, forced by the pole-to-equator heating.
	{"lorenz84", &Lorenz84DT, func(x, y, z float64) (float64, float64, float64) {
		a, b, f, g := float64(Lorenz84A), float64(Lorenz84B), float64(Lorenz84F), float64(Lorenz84G)
		return -y*y - z*z - a*x + a*f, x*y - b*x*z - y + g, b*x*y + x*z - z
	}, [3]float32{0, 0, 1.3}, []Param{
		{"lorenz84-dt", "dt", &Lorenz84DT, 0.005, 0.001, 0.02, 0.001},
		{"lorenz84-a", "a", &Lorenz84A, 0.25, 0.05, 1, 0.01},
		{"lorenz84-b", "b", &Lorenz84B, 4, 1, 8, 0.05},
		{"lorenz84-f", "F", &Lorenz84F, 8, 2, 15, 0.05},
		{"lorenz84-g", "G", &Lorenz84G, 1, 0, 3, 0.01},
	}},
	// The finance system (Ma and Chen, 2001): interest rate, investment
	// demand and price index.
	{"finance", &FinanceDT, func(x, y, z float64) (float64, float64, float64) {
		a, b, c := float64(FinanceA), float64(FinanceB), float64(FinanceC)
		return z + (y-a)*x, 1 - b*y - x*x, -x - c*z
	}, [3]float32{1, 2, 0.5}, []Param{
		{"finance-dt", "dt", &FinanceDT, 0.005, 0.001, 0.02, 0.001},
		{"finance-a", "a", &FinanceA, 0.9, 0.1, 3, 0.01},
		{"finance-b", "b", &FinanceB, 0.2, 0.05, 1, 0.01},
		{"finance-c", "c", &FinanceC, 1.2, 0.2, 3, 0.01},
	}},
	// Hindmarsh and Rose (1984): a neuron, spiking in bursts, the bursts
	// chaotic at this current.
	{"hindmarshrose", &HindmarshDT, func(x, y, z float64) (float64, float64, float64) {
		i, r, s := float64(HindmarshI), float64(HindmarshR), float64(HindmarshS)
		return y - x*x*x + 3*x*x - z + i, 1 - 5*x*x - y, r * (s*(x+1.6) - z)
	}, [3]float32{-1.6, -10, 2}, []Param{
		{"hindmarsh-dt", "dt", &HindmarshDT, 0.01, 0.002, 0.05, 0.001},
		{"hindmarsh-i", "I", &HindmarshI, 3.25, 1, 4, 0.01},
		{"hindmarsh-r", "r", &HindmarshR, 0.006, 0.001, 0.02, 0.0005},
		{"hindmarsh-s", "s", &HindmarshS, 4, 1, 6, 0.05},
	}},
	// Wei (2011): chaos with no equilibrium at all, a hidden attractor.
	{"wei", &WeiDT, func(x, y, z float64) (float64, float64, float64) {
		return -y, x + z, 2*y*y + x*z - float64(WeiA)
	}, [3]float32{0, 0.9, 0}, []Param{
		{"wei-dt", "dt", &WeiDT, 0.005, 0.001, 0.02, 0.001},
		{"wei-a", "a", &WeiA, 0.35, 0.1, 0.6, 0.005},
	}},
	// Sprott (1997): the simplest dissipative chaotic flow, a jerk
	// equation with one quadratic term, x‴ = −A x″ + x′² − x.
	{"sprottjerk", &JerkDT, func(x, y, z float64) (float64, float64, float64) {
		return y, z, -float64(JerkA)*z + y*y - x
	}, [3]float32{-1, 0, 0}, []Param{
		{"jerk-dt", "dt", &JerkDT, 0.005, 0.001, 0.02, 0.001},
		{"jerk-a", "A", &JerkA, 2.017, 1.9, 2.1, 0.001},
	}},
}

func init() {
	for _, f := range MoreFlows {
		InitCond[f.Key] = f.IC
		registerFlow64(f.Key, f.DT, f.Deriv)
		params[f.Key] = f.Knobs
	}
}
