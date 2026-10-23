package attractor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestEveryLinkKeyIsOneControls: a link restores a key into the first control
// that claims it, so two controls under one key lose one of them from every
// link. The Trail knob and the distortion meter's RATE were both "tr", so a
// link with a long trail opened on the default one; the Lattice's ROWS and
// the loudness meter's RATE were both "lr". The keys are declared in
// several places — a control's PermaKey, a meter's switch, the switches'
// table, Model Out's, the synth's, the keys the restore reads itself — so they are
// gathered from the source, as written, and compared.
func TestEveryLinkKeyIsOneControls(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Keys applyStateFrom and applyControl read before any control does.
	seen := map[string]string{}
	for _, k := range []string{"rot", "drag", "q", "pl", "am", "bk", "eq", "fr", "mx"} {
		seen[k] = "the restore itself"
	}
	add := func(key, where string) {
		if prev, dup := seen[key]; dup {
			t.Errorf("link key %q is %s and %s", key, prev, where)
			return
		}
		seen[key] = where
	}
	str := func(e ast.Expr) (string, bool) {
		if b, ok := e.(*ast.BasicLit); ok && b.Kind == token.STRING {
			s, err := strconv.Unquote(b.Value)
			return s, err == nil
		}
		return "", false
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				// The synth's tables name their keys "key".
				if id, ok := n.Type.(*ast.Ident); ok && (id.Name == "synthKnob" || id.Name == "synthSelect") {
					for _, e := range n.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "key" {
								if s, ok := str(kv.Value); ok {
									add(s, "the synth's at "+fset.Position(kv.Pos()).String())
								}
							}
						}
					}
				}
			case *ast.KeyValueExpr:
				if id, ok := n.Key.(*ast.Ident); ok && id.Name == "PermaKey" {
					if s, ok := str(n.Value); ok {
						add(s, "a PermaKey at "+fset.Position(n.Pos()).String())
					}
				}
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "addMeterSwitch" && len(n.Args) > 4 {
					if s, ok := str(n.Args[4]); ok {
						add(s, "a meter switch at "+fset.Position(n.Pos()).String())
					}
				}
			case *ast.ValueSpec:
				for i, name := range n.Names {
					if i >= len(n.Values) {
						continue
					}
					lit, ok := n.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					switch name.Name {
					case "permaCtls":
						for _, e := range lit.Elts {
							if row, ok := e.(*ast.CompositeLit); ok && len(row.Elts) > 0 {
								if s, ok := str(row.Elts[0]); ok {
									add(s, "permaCtls at "+fset.Position(row.Pos()).String())
								}
							}
						}
					case "modelOutKeys":
						for _, e := range lit.Elts {
							if s, ok := str(e); ok {
								add(s, "Model Out's")
							}
						}
					}
				}
			}
			return true
		})
	}
	if len(seen) < 90 {
		t.Fatalf("found only %d link keys: the scan has stopped seeing where they are declared", len(seen))
	}
	// And none of the written ones lands in a family made at run time: the
	// generators' g1f…, the scopes' inputs si1a… and settings s2v…, the routes
	// and parameters.
	made := regexp.MustCompile(`^(g[1-4][a-z]|si[1-4][ab]|s[1-4][a-z]|p\.|m\.|vm\.|cp\.)`)
	for k, where := range seen {
		if made.MatchString(k) {
			t.Errorf("link key %q (%s) is one a family of keys made at run time can take", k, where)
		}
	}
}
