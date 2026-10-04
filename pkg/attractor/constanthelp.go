package attractor

import (
	"regexp"
	"strings"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The tooltips of the dynamical systems' constants, written from their
// equations rather than by hand.
//
// A constant of a vector field has no meaning of its own to spell out: "a"
// is the a in the equations and nothing else. What someone hovering it needs
// is WHERE it is — which of the derivatives it enters and how — and that is in
// builtinEquations already, so the tooltip quotes the lines it appears in. A
// hand-written sentence per constant would be a second copy of the equations
// that could disagree with the first.

// greekNames spells a Greek label the way the equations table has to: its
// expressions are parsed, and the parser reads ASCII names.
var greekNames = map[string]string{
	"α": "alpha", "β": "beta", "γ": "gamma", "δ": "delta", "ε": "epsilon",
	"κ": "kappa", "λ": "lambda", "μ": "mu", "ν": "nu", "ρ": "rho",
	"σ": "sigma", "ω": "omega",
}

var derivNames = [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}

// dtHelp is every integrated system's step, which means the same everywhere.
var dtHelp = doc("dt-help")

// constantHelp is the tooltip sentence for one of a system's constants, or ""
// when mode is not an integrated system or label is not one of its constants.
func constantHelp(mode, label string) string {
	if h := quotedConstantHelp(mode, label); h != "" {
		return h
	}
	be, ok := builtinEquations[mode]
	if !ok {
		return ""
	}
	// A spelling equationNames does not know (the test says there is none
	// today): say what it is and show the whole system rather than guess.
	var lines []string
	for i, eq := range be.eq {
		if eq != "" {
			lines = append(lines, derivNames[i]+" = "+prettyEquation(eq))
		}
	}
	return label + " — a constant of the " + modeLabel(mode) + " system:\n" + strings.Join(lines, "\n")
}

// quotedConstantHelp is constantHelp when label is found in the equations,
// and "" when it is not — for a caller that would rather say nothing than
// show the whole system beside a knob that is not one of its constants.
func quotedConstantHelp(mode, label string) string {
	if label == "dt" {
		return dtHelp
	}
	be, ok := builtinEquations[mode]
	if !ok {
		return printedConstantHelp(mode, label)
	}
	var lines []string
	for _, name := range equationNames(label) {
		word := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		for i, eq := range be.eq {
			if eq != "" && word.MatchString(eq) {
				lines = append(lines, derivNames[i]+" = "+prettyEquation(eq))
			}
		}
		if len(lines) > 0 {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return label + " — a constant of the " + modeLabel(mode) + " system. It enters\n" + strings.Join(lines, "\n")
}

// printedConstantHelp quotes a constant from the equations printed in its
// model's manual entry, for the models with no parseable table: the maps.
// Their equations are written for reading (x' = 1 − a·x² + y), so a name is
// matched between non-letters rather than at \b, which is ASCII-only and
// would never find μ.
func printedConstantHelp(mode, label string) string {
	e, ok := manual.Lookup("model." + mode)
	if !ok {
		return ""
	}
	word := regexp.MustCompile(`(^|[^\pL\pN_])` + regexp.QuoteMeta(label) + `($|[^\pL\pN_])`)
	var lines []string
	for _, l := range printedEquations(e.Markdown) {
		if word.MatchString(l) {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	name := modeLabel(mode)
	switch {
	case modeInfo[mode].Class != ClassMap:
		name += " system"
	case !strings.HasSuffix(strings.ToLower(name), " map"):
		name += " map"
	}
	return label + " — a constant of the " + name + ". It enters\n" + strings.Join(lines, "\n")
}

// printedEquations is the lines of the first ```text block in an entry.
func printedEquations(md string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(md, "\n") {
		switch t := strings.TrimSpace(l); {
		case !in && t == "```text":
			in = true
		case in && t == "```":
			return out
		case in && t != "":
			out = append(out, t)
		}
	}
	return nil
}

// prettyEquation writes a parseable expression the way it is printed:
// Greek letters as themselves and multiplication as a dot.
func prettyEquation(eq string) string {
	for g, n := range greekNames {
		eq = regexp.MustCompile(`\b`+n+`\b`).ReplaceAllString(eq, g)
	}
	return strings.ReplaceAll(eq, "*", "·")
}

// systemConstantHelp is constantHelp keyed by a parameter id, over every
// integrated system's constants.
var systemConstantHelp = func() map[string]string {
	out := map[string]string{}
	for _, mode := range dynamics.ParamModes() {
		for _, p := range dynamics.Params(mode) {
			if h := constantHelp(mode, p.Label); h != "" {
				out[p.ID] = h
			}
		}
	}
	return out
}()

// equationNames is how a panel label may be spelled in the equations table,
// most likely first: a Greek letter by its name, a capital as the lower case
// the table uses (Burke–Shaw's S and V), and e as k, because e is the
// parser's Euler's number and the table had to rename it (Aizawa, Dadras).
func equationNames(label string) []string {
	if n, ok := greekNames[label]; ok {
		return []string{n}
	}
	names := []string{label}
	if l := strings.ToLower(label); l != label {
		names = append(names, l)
	}
	if label == "e" {
		names = append(names, "k")
	}
	return names
}
