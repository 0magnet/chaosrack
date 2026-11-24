package uitool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/chester"
	"github.com/0magnet/chaosrack/pkg/sacred"
)

// Some models are many figures behind one knob — the Chestahedron's FORM
// picks among fourteen of Frank Chester's demonstrations — and one still of
// the default says nothing about the other thirteen. Each such figure gets a
// still of its own and a link that opens the page with it up, from the one
// list below, so the pictures and the links cannot disagree.

// formPreset is one figure of a model: its name (the knob's label for it)
// and the controls that bring it up, in the order they are set.
type formPreset struct {
	name string
	set  [][2]string // control id, value
}

// modelForms are the models shown figure by figure.
var modelForms = map[string][]formPreset{
	"chestahedron": chestForms(),
	"sacred":       sacredForms(),
}

// chestForms is every FORM at the MORPH and DETAIL that show it best: the
// form as Chester shows it (MORPH 1) with what he finds inside or around it.
func chestForms() []formPreset {
	show := map[string][2]float64{ // morph, detail
		"solid": {1, 3}, "star": {1, 1}, "inner": {1, 4}, "sphere": {1, 4},
		"bell": {1.5, 2}, "air": {1, 0}, "deca": {1, 1}, "heart": {1, 1},
		"spin": {0.5, 2}, "first": {1, 1}, "roll": {1, 6}, "fire": {1, 2},
		"beat": {1, 2}, "bubble": {1, 2},
	}
	out := make([]formPreset, 0, len(chester.FormNames))
	for i, n := range chester.FormNames {
		md, ok := show[n]
		if !ok {
			md = [2]float64{1, 1}
		}
		num := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
		out = append(out, formPreset{n, [][2]string{
			{"chestahedron-form", strconv.Itoa(i)},
			{"chestahedron-morph", num(md[0])},
			{"chestahedron-detail", num(md[1])},
		}})
	}
	return out
}

// formStill is a figure's still in dir.
func formStill(dir, key, name string) string {
	return filepath.Join(dir, key+"-form-"+name+".jpg")
}

// formLink is the page with the figure up: the model, then each control as
// the page writes it into a link (p.<id less the model's prefix>=<value>).
func formLink(key string, f formPreset) string {
	parts := make([]string, 0, len(f.set))
	for _, s := range f.set {
		parts = append(parts, "p."+strings.TrimPrefix(s[0], key+"-")+"="+s[1])
	}
	return appLink(key, strings.Join(parts, "&"))
}

// renderForms draws each of key's figures with chaosrack render, posed and
// colored as the model's own still.
func renderForms(key string, idx int) error {
	forms := modelForms[key]
	if len(forms) == 0 {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	vars := portraitVariations(idx)
	walk := fmt.Sprintf("%d/%s", vars[0].gc, [...]string{"x", "y", "z", "trail"}[vars[0].gs])
	for _, f := range forms {
		args := []string{"render", "--model", key, "--crop", "--angle", formAngle(key, f),
			"-o", formStill(*portDir, key, f.name), "--width", "300", "--height", "300"}
		if *portColors {
			args = append(args, "--walk", walk)
		}
		for _, s := range portraitSets[key] {
			args = append(args, "--set", s)
		}
		for _, s := range f.set {
			args = append(args, "--set", s[0]+"="+s[1])
		}
		if out, err := exec.Command(self, args...).CombinedOutput(); err != nil { //nolint:gosec // this binary, with arguments built here
			return fmt.Errorf("%s form %s: %w: %s", key, f.name, err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("  +%s", f.name)
	}
	return nil
}

// formsGallery is the README's table of a model's figures, four to a row,
// each a still (when captured) and its name, both linked to the page.
func formsGallery(key string) string {
	forms := modelForms[key]
	if len(forms) == 0 {
		return ""
	}
	const cols = 4
	var b strings.Builder
	b.WriteString("Its forms, each opening the page with that form up:\n\n")
	b.WriteString("|" + strings.Repeat("   |", cols) + "\n|" + strings.Repeat(" --- |", cols) + "\n")
	for i := 0; i < len(forms); i += cols {
		row := make([]string, cols)
		for j := range cols {
			if i+j >= len(forms) {
				continue
			}
			f := forms[i+j]
			link := formLink(key, f)
			cell := fmt.Sprintf("[%s](%s)", f.name, link)
			if p := formStill(*modelImgDir, key, f.name); fileExists(p) {
				cell = fmt.Sprintf("[![%s](%s)](%s)<br>", f.name, filepath.ToSlash(p), link) + cell
			}
			row[j] = cell
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return b.String() + "\n"
}

// formAngle is the pose a form is rendered in: a solid tilted so its depth
// shows, a flat construction face-on, as it is drawn on paper.
func formAngle(key string, f formPreset) string {
	if key == "sacred" {
		if f.name == "sriyantra-3d" {
			return "-1.1,0,0.3" // the Meru from the side, rising to its summit
		}
		for _, s := range f.set {
			if s == [2]string{"sacred-dim", "3"} {
				return "0.5,0.6,0"
			}
		}
		return "0,0,0"
	}
	return "0.349066,0,0"
}

// sacredForms is every Sacred Geometry figure drawn whole (MORPH 1), at the
// DETAIL that shows it as it is usually drawn: flat, and then in space.
func sacredForms() []formPreset {
	flat := map[string]float64{
		"vesica": 2, "seed": 1, "flower": 1, "fruit": 0, "metatron": 1, "golden": 3, "sunflower": 3, "merkaba": 1, "kepler": 0, "penrose": 2, "sriyantra": 3,
	}
	space := map[string]float64{
		"vesica": 0, "seed": 0, "flower": 0, "fruit": 1, "metatron": 0, "golden": 1, "sunflower": 3, "merkaba": 0, "kepler": 0, "penrose": 0, "sriyantra": 3,
	}
	var out []formPreset
	for _, dim := range []struct {
		n      string
		suffix string
		detail map[string]float64
	}{{"2", "", flat}, {"3", "-3d", space}} {
		for i, n := range sacred.FormNames {
			out = append(out, formPreset{n + dim.suffix, [][2]string{
				{"sacred-form", strconv.Itoa(i)},
				{"sacred-morph", "1"},
				{"sacred-detail", strconv.FormatFloat(dim.detail[n], 'g', -1, 64)},
				{"sacred-dim", dim.n},
			}})
		}
	}
	return out
}
