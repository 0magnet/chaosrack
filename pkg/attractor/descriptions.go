package attractor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The mode descriptions — the prose the info overlay shows and the README's
// model reference is generated from. They are the manual's (models.md, each
// as model.<mode>), and this is them by mode. Untagged, next to the mode
// registry in modes.go, so the native tools can read them: cmd/uitool writes
// the README from this map. Modes whose description is composed from data
// (the Sprott catalog) register in the init below.
var attractorDescriptions = func() map[string]string {
	m := map[string]string{}
	for _, k := range manual.Keys() {
		if mode, ok := strings.CutPrefix(k, "model."); ok {
			m[mode] = manual.Text(k)
		}
	}
	return m
}()

func init() {
	// Composed from the catalog data rather than written out eighteen times,
	// and what tells one apart from the next is read off its equations.
	for _, c := range dynamics.SprottCases {
		attractorDescriptions[c.Key] = c.Name +
			" — one of J. C. Sprott's simple chaotic flows (1994), realized as an" +
			" analog circuit at glensstuff.com. Found by systematic search for the" +
			" algebraically simplest systems that still produce chaos.\n\n" +
			anatomyProse(c.Anatomy()) + "\n\n" + c.Eq
	}
}

// anatomyProse says what a Sprott system is made of: its terms, how it treats
// a volume of starting points, and its symmetry.
func anatomyProse(a dynamics.Anatomy) string {
	nl := strings.Join(a.Nonlinear, " and ")
	var b strings.Builder
	switch len(a.Nonlinear) {
	case 1:
		fmt.Fprintf(&b, "%s terms, one of them nonlinear (%s).", capitalize(countWord(a.Terms)), nl)
	default:
		fmt.Fprintf(&b, "%s terms, %s of them nonlinear (%s).", capitalize(countWord(a.Terms)), countWord(len(a.Nonlinear)), nl)
	}
	div := a.Div[0]
	v, slope := "", 0.0
	for i, name := range []string{"x", "y", "z"} {
		if a.Div[i+1] != 0 {
			v, slope = name, a.Div[i+1]
		}
	}
	switch {
	case v == "" && div < 0:
		every := "every " + divNum(-1/div) + " time units"
		if div == -1 {
			every = "every unit of time"
		}
		fmt.Fprintf(&b, " ∇·F = %s everywhere, so a cloud of starting points loses volume at one steady rate, by a factor of e %s.",
			divNum(div), every)
	case v != "":
		// The divergence is linear in one coordinate, so it changes sign on
		// a plane: volume grows on one side of it and shrinks on the other.
		at := -div / slope
		if at == 0 {
			at = 0 // not −0
		}
		grow, shrink := ">", "<"
		if slope < 0 {
			grow, shrink = shrink, grow
		}
		fmt.Fprintf(&b, " ∇·F = %s, which changes sign at %s = %s: volume grows where %s %s %s and shrinks where %s %s %s, so the flow is dissipative only on average along the attractor.",
			divExpr(div, slope, v), v, divNum(at), v, grow, divNum(at), v, shrink, divNum(at))
	}
	if a.HalfTurnZ {
		b.WriteString(" The equations are unchanged by (x, y, z) → (−x, −y, z), so the attractor is symmetric under a half turn about the z axis, or has a twin that is its half-turned copy.")
	}
	return b.String()
}

func capitalize(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func countWord(n int) string {
	if w := []string{"no", "one", "two", "three", "four", "five", "six", "seven"}; n < len(w) {
		return w[n]
	}
	return strconv.Itoa(n)
}

func divNum(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'g', 2, 64), "-", "−", 1)
}

// divExpr writes c + s·v the way the equations are printed: 2y, x, y − 0.7.
func divExpr(c, s float64, v string) string {
	t := v
	if s != 1 {
		t = divNum(s) + v
	}
	switch {
	case c > 0:
		return t + " + " + divNum(c)
	case c < 0:
		return t + " − " + divNum(-c)
	}
	return t
}

// descriptionMarkdown is a composed description as the manual's markdown, so
// it renders as the written entries do: a paragraph each, and the equations
// that close it set as a block rather than run into the prose.
func descriptionMarkdown(d string) string {
	paras := strings.Split(strings.TrimSpace(d), "\n\n")
	for i, p := range paras {
		if i == len(paras)-1 && i > 0 && isEquationBlock(p) {
			paras[i] = "```text\n" + p + "\n```"
			continue
		}
		paras[i] = manual.Escape(p)
	}
	return strings.Join(paras, "\n\n")
}

// isEquationBlock is a paragraph whose every line is an equation.
func isEquationBlock(p string) bool {
	for l := range strings.SplitSeq(p, "\n") {
		if !strings.Contains(l, " = ") {
			return false
		}
	}
	return true
}
