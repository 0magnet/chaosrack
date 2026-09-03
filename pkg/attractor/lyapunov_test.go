package attractor

import (
	"math"
	"testing"
)

// The estimator is checked against values that were published before this
// program existed. Everything else here is self-consistent — the same code
// measuring systems this same code integrates — so an independent number is
// the only thing that can catch a mistake shared by both.
func TestLyapunovMatchesPublishedValues(t *testing.T) {
	cases := []struct {
		mode string
		want float64
		tol  float64
		src  string
	}{
		// Hénon's map at a=1.4, b=0.3. The canonical figure.
		{"henon", 0.419, 0.02, "Hénon (1976); λ₁ ≈ 0.41922"},
		// Lorenz at the usual σ=10, ρ=28, β=8/3.
		{"lorenz", 0.906, 0.12, "Lorenz (1963); λ₁ ≈ 0.9056"},
	}
	for _, c := range cases {
		r := LyapunovFor(c.mode)
		if !r.OK {
			t.Errorf("%s: could not be measured", c.mode)
			continue
		}
		if math.Abs(r.Lambda-c.want) > c.tol {
			t.Errorf("%s: λ = %.4f, want %.3f ± %.3f — %s",
				c.mode, r.Lambda, c.want, c.tol, c.src)
		}
	}
}

// Hénon's exponents are also known exactly in SUM: the two of them add to
// ln|det J| = ln(b), because the Jacobian determinant is −b everywhere. That
// gives a second, independent check on λ₁ that needs no published figure — if
// λ₁ were wrong, the implied λ₂ would be too.
func TestHenonExponentsSumToLogB(t *testing.T) {
	r := LyapunovForMap("henon")
	if !r.OK {
		t.Fatal("henon could not be measured")
	}
	sum := math.Log(float64(henonB)) // λ₁ + λ₂
	lam2 := sum - r.Lambda
	// λ₂ must be the strongly contracting one, and well below zero.
	if lam2 > -1.0 {
		t.Errorf("λ₁=%.4f implies λ₂=%.4f; with λ₁+λ₂=ln(b)=%.4f the second exponent should be strongly negative",
			r.Lambda, lam2, sum)
	}
	// And λ₁ must be the larger, or they have been swapped.
	if r.Lambda <= lam2 {
		t.Errorf("λ₁=%.4f is not larger than λ₂=%.4f", r.Lambda, lam2)
	}
}

// The unit differs by kind and the readout says which, because 0.42 per
// ITERATE and 0.42 per unit TIME are not comparable quantities and showing
// them in one column without a unit would invite exactly that comparison.
func TestLyapunovUnitsAreLabelled(t *testing.T) {
	if r := LyapunovFor("henon"); !r.PerStep {
		t.Error("a map's exponent is per iterate and must say so")
	}
	if r := LyapunovFor("lorenz"); r.PerStep {
		t.Error("a flow's exponent is per unit time, not per iterate")
	}
}

// A polyhedron has no Lyapunov exponent. Printing 0.0000 next to a cube would
// be a category error dressed up as a measurement.
func TestLyapunovDeclinesNonDynamicalModes(t *testing.T) {
	for _, mode := range []string{"cube", "globe", "torus", "spectrogram", "stlfile"} {
		r := LyapunovFor(mode)
		if r.Verdict != "n/a" {
			t.Errorf("%s: verdict %q, want n/a — it is not a dynamical system", mode, r.Verdict)
		}
		if r.OK {
			t.Errorf("%s: reported OK for a mode with no dynamics", mode)
		}
	}
}

// Every model that IS a dynamical system must be measurable, and at its
// shipped defaults must be chaotic. This is the mode-defaults guard extended
// to the maps, which arrived with three bad defaults between them.
func TestEveryDynamicalModeMeasuresChaotic(t *testing.T) {
	measured := 0
	for _, k := range CatalogKeys() {
		r := LyapunovFor(k)
		if r.Verdict == "n/a" {
			continue
		}
		measured++
		if !r.OK {
			t.Errorf("%s: could not be measured (%s)", k, r.Verdict)
			continue
		}
		if r.Verdict != "chaotic" {
			t.Errorf("%s: λ=%.4f reads %q at its defaults", k, r.Lambda, r.Verdict)
		}
	}
	if measured < 30 {
		t.Errorf("only %d modes were measurable; the registries are probably not loaded", measured)
	}
}

// classify is the layer that turns a number into a word, and the word is what
// most people will read. Its thresholds are the whole interface.
func TestClassifyReadsTheExponent(t *testing.T) {
	for _, c := range []struct {
		lam  float64
		want string
	}{
		{0.9, "chaotic"}, {0.006, "chaotic"},
		{0.0, "periodic"}, {0.001, "periodic"}, {-0.001, "periodic"},
		{-0.5, "converging"},
		{math.NaN(), "diverged"}, {math.Inf(1), "diverged"},
	} {
		if got := classify(c.lam); got != c.want {
			t.Errorf("classify(%v) = %q, want %q", c.lam, got, c.want)
		}
	}
}
