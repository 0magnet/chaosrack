// Subcommand readme: regenerate the README's generated regions — the table of
// contents, the model reference, and the module reference.
//
//	uitool readme            # rewrite README.md in place
//	uitool readme -check     # exit 1 if it is out of date, changing nothing
//
// Three regions are marked in the file and rewritten wholesale, the way
// gendocs.sh rewrites the dependency graph — a patched section drifts, a
// replaced one cannot:
//
//	<!-- BEGIN TOC -->     … <!-- END TOC -->
//	<!-- BEGIN MODELS -->  … <!-- END MODELS -->
//	<!-- BEGIN MODULES --> … <!-- END MODULES -->
//
// The model reference comes from attractor.Catalog(), which is the same
// ordered structure the mode-selector knobs turn through, so the sections and
// their order match the control surface by construction. The module reference
// comes from the manifest `uitool modules` writes out of the live DOM.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	readmePath  = flag.String("readme", "README.md", "the file to rewrite")
	readmeCheck = flag.Bool("check", false, "report whether the file is up to date; write nothing")
	modelImgDir = flag.String("model-img", "docs/img/model", "where the per-model stills and loops live")
	moduleImgD  = flag.String("module-img", "docs/img/module", "where the module shots and their manifest live")
)

func runReadme() {
	src, err := os.ReadFile(*readmePath) //nolint:gosec // the path is the file this command was told to rewrite
	if err != nil {
		fmt.Fprintln(os.Stderr, "readme:", err)
		os.Exit(1)
	}
	out, err := regenerate(string(src))
	if err != nil {
		fmt.Fprintln(os.Stderr, "readme:", err)
		os.Exit(1)
	}
	if *readmeCheck {
		if out != string(src) {
			fmt.Fprintln(os.Stderr, "readme: out of date — run `go run ./cmd/uitool readme`")
			os.Exit(1)
		}
		fmt.Println("readme: up to date")
		return
	}
	if out == string(src) {
		fmt.Println("readme: already up to date")
		return
	}
	if err := os.WriteFile(*readmePath, []byte(out), 0o600); err != nil { //nolint:gosec // the path is the README this command was told to rewrite, from its own flag
		fmt.Fprintln(os.Stderr, "readme:", err)
		os.Exit(1)
	}
	fmt.Println("readme: rewrote", *readmePath)
}

// regenerate fills the three marked regions. The contents region is filled
// last, from the finished document, so it lists the headings the other two
// regions just produced.
func regenerate(src string) (string, error) {
	out, err := replaceRegion(src, "MODELS", modelsSection())
	if err != nil {
		return "", err
	}
	out, err = replaceRegion(out, "MODULES", modulesSection())
	if err != nil {
		return "", err
	}
	return replaceRegion(out, "TOC", tableOfContents(out))
}

// replaceRegion swaps what lies between <!-- BEGIN name --> and
// <!-- END name -->, markers included in neither.
func replaceRegion(src, name, body string) (string, error) {
	begin := "<!-- BEGIN " + name + " -->"
	end := "<!-- END " + name + " -->"
	i := strings.Index(src, begin)
	j := strings.Index(src, end)
	if i < 0 || j < 0 {
		return "", fmt.Errorf("missing %s / %s markers", begin, end)
	}
	if j < i {
		return "", fmt.Errorf("%s appears before %s", end, begin)
	}
	return src[:i+len(begin)] + "\n\n" + strings.TrimSpace(body) + "\n\n" + src[j:], nil
}

// modelsSection writes one section per category of the mode selector and one
// subsection per model, in knob order.
func modelsSection() string {
	var b strings.Builder
	b.WriteString("Every position of the model selector, in the order the knob turns through\n")
	b.WriteString("them. A model that appears under two categories — the XY scope and the\n")
	b.WriteString("Takens embedding are each both a Scope and an Audio model — is listed under\n")
	b.WriteString("both, because that is what the selector does.\n\n")
	b.WriteString("Each entry is captured from the running app by `uitool portraits`: a still,\n")
	b.WriteString("a loop turning through all four positions of the palette knob, and the\n")
	b.WriteString("**Parameters module as it stands for that model** — the knobs are the\n")
	b.WriteString("system's own, so Lorenz's σ/ρ/β and the turtle's mod/seq/dim are different\n")
	b.WriteString("panels under the same header. A model with no parameters of its own has no\n")
	b.WriteString("such column. The prose is the same text the Info overlay shows.\n")

	seen := map[string]bool{}
	for _, g := range attractor.Catalog() {
		fmt.Fprintf(&b, "\n### %s\n\n", g.Label)

		// A compact index of the category, so the contents can stay at
		// section depth without hiding sixty models behind it.
		links := make([]string, 0, len(g.Models))
		for _, m := range g.Models {
			links = append(links, fmt.Sprintf("[%s](#%s)", mdEscape(m.Label), anchor(m.Label)))
		}
		b.WriteString(strings.Join(links, " · ") + "\n")

		for _, m := range g.Models {
			if seen[m.Key] {
				fmt.Fprintf(&b, "\n#### %s\n\nSee [%s](#%s) above.\n", mdEscape(m.Label), mdEscape(m.Label), anchor(m.Label))
				continue
			}
			seen[m.Key] = true
			b.WriteString(modelEntry(m))
		}
	}
	return b.String()
}

func modelEntry(m attractor.CatalogModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n#### %s\n\n", mdEscape(m.Label))

	// The images a model can have. Every one is optional and included only if
	// it was captured, so a model with no parameters of its own simply has no
	// Parameters column rather than a broken image.
	type shot struct{ head, alt, path string }
	shots := []shot{
		{mdEscape(m.Label), mdEscape(m.Label), filepath.Join(*modelImgDir, m.Key+".jpg")},
		{"turning", mdEscape(m.Label) + " turning", filepath.Join(*modelImgDir, m.Key+".gif")},
	}
	// Named variants: a second face of the same model that a still cannot
	// show at the same time as the first. The turtle's is the big one — the
	// same walk in two dimensions and in three are different figures.
	for _, v := range modelVariants[m.Key] {
		shots = append(shots, shot{v.head, mdEscape(m.Label) + " — " + v.head,
			filepath.Join(*modelImgDir, m.Key+"-"+v.suffix+".jpg")})
	}
	// The Parameters module as it stands for THIS model: the knobs differ per
	// system, and a reference that shows the figure without the controls that
	// shape it is only half the entry.
	shots = append(shots, shot{"parameters", mdEscape(m.Label) + " parameters",
		filepath.Join(*modelImgDir, m.Key+"-params.jpg")})

	var have []shot
	for _, s := range shots {
		if fileExists(s.path) {
			have = append(have, s)
		}
	}
	if len(have) > 0 {
		var heads, seps, cells []string
		for _, s := range have {
			heads = append(heads, s.head)
			seps = append(seps, "---")
			cells = append(cells, fmt.Sprintf("![%s](%s)", s.alt, s.path))
		}
		fmt.Fprintf(&b, "| %s |\n| %s |\n| %s |\n\n",
			strings.Join(heads, " | "), strings.Join(seps, " | "), strings.Join(cells, " | "))
	}

	prose, equations := splitDescription(m.Description)
	if prose != "" {
		b.WriteString(prose + "\n")
	}
	if equations != "" {
		fmt.Fprintf(&b, "\n```\n%s\n```\n", equations)
	}
	fmt.Fprintf(&b, "\n`#%s` · %s\n", m.Key, m.Class)
	return b.String()
}

// splitDescription separates the prose from the equations the descriptions
// carry after a blank line, so the equations can be set as a code block and
// keep their line breaks — in a paragraph they would run together.
func splitDescription(d string) (prose, equations string) {
	d = strings.TrimSpace(d)
	i := strings.Index(d, "\n\n")
	if i < 0 {
		return d, ""
	}
	head, tail := strings.TrimSpace(d[:i]), strings.TrimSpace(d[i+2:])
	// Only a trailing block that looks like equations is set as code; the
	// scope modes use the same separator for a second paragraph of prose.
	if looksLikeEquations(tail) {
		return head, tail
	}
	return strings.ReplaceAll(d, "\n\n", "\n\n"), ""
}

func looksLikeEquations(s string) bool {
	if s == "" {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "=") {
			return false
		}
	}
	return true
}

// panelModule is one entry of the manifest `uitool modules` writes.
type panelModule struct {
	ID    string `json:"id"`    // element id, e.g. "record-module"
	Label string `json:"label"` // header text, e.g. "Record"
	Title string `json:"title"` // the header's tooltip — the module's own description
	Image string `json:"image"` // path to its shot, relative to the repo root
	// Sel is the data-uitool stamp used to find the element again in the DOM.
	// It is cleared before the manifest is written — it means nothing outside
	// the pass that stamped it — but it cannot be `json:"-"`, because that
	// also stops it being READ back out of the browser, which left every
	// selector empty and photographed nothing at all.
	Sel  string `json:"sel,omitempty"`
	Mode string `json:"mode"` // the model it was photographed under
}

// modulesSection writes the module reference from the manifest. Absent a
// manifest it says so rather than writing an empty section, because an empty
// section reads like "there are no modules".
func modulesSection() string {
	path := filepath.Join(*moduleImgD, "modules.json")
	data, err := os.ReadFile(path) //nolint:gosec // a generated manifest at a path this command owns
	if err != nil {
		return "_No module manifest yet — run `go run ./cmd/uitool modules` against a\n" +
			"running tab to capture the rack._"
	}
	var mods []panelModule
	if err := json.Unmarshal(data, &mods); err != nil {
		return "_Module manifest could not be read: " + err.Error() + "._"
	}
	var b strings.Builder
	b.WriteString("The rack is built out of modules. Every one of them has a switch in the\n")
	b.WriteString("Console's Modules column — the Console itself has none, because that is\n")
	b.WriteString("where the switches live — and every one can be dragged by its header into\n")
	b.WriteString("another slot. This reference is captured from the running rack by\n")
	b.WriteString("`uitool modules`, and each description is the module header's own tooltip.\n")
	for _, m := range mods {
		fmt.Fprintf(&b, "\n### %s\n\n", mdEscape(m.Label))
		desc := tableText(m.Title)
		switch {
		case m.Image != "" && fileExists(m.Image):
			// The shot and what it is, side by side. Reading a panel and then
			// scrolling past it to find out what it does is the layout this
			// replaces. The width is capped so a 21 HP module does not take
			// the whole row and leave the text in a column two words wide.
			b.WriteString("| | |\n|---|---|\n")
			fmt.Fprintf(&b, "| <img src=\"%s\" alt=\"The %s module\" width=\"320\"> | %s |\n",
				m.Image, mdEscape(m.Label), desc)
		case desc != "":
			b.WriteString(desc + "\n")
		}
	}
	return b.String()
}

// tableOfContents lists every heading in the document at depth 2 and 3.
//
// Depth 4 is where the individual models are, and sixty of them would bury
// the sections they belong to; each category section carries its own inline
// index of them instead.
func tableOfContents(doc string) string {
	var b strings.Builder
	b.WriteString("<!-- generated by `uitool readme` — do not edit by hand -->\n")
	counts := map[string]int{}
	inFence := false
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		level, text := heading(line)
		// Every heading is counted, even the ones not listed: GitHub's anchor
		// numbering counts them all, so skipping them here would hand out the
		// wrong suffix to a later duplicate.
		if level == 0 {
			continue
		}
		slug := uniqueAnchor(text, counts)
		if level < 2 || level > 3 {
			continue
		}
		fmt.Fprintf(&b, "%s- [%s](#%s)\n", strings.Repeat("  ", level-2), mdEscape(text), slug)
	}
	return b.String()
}

// heading returns the level and text of an ATX heading line, or 0.
func heading(line string) (int, string) {
	if !strings.HasPrefix(line, "#") {
		return 0, ""
	}
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n > 6 || n >= len(line) || line[n] != ' ' {
		return 0, ""
	}
	return n, strings.TrimSpace(line[n+1:])
}

// uniqueAnchor is anchor() plus GitHub's duplicate suffixing: the second
// heading that slugs the same way gets -1, the third -2.
func uniqueAnchor(text string, counts map[string]int) string {
	base := anchor(text)
	n := counts[base]
	counts[base]++
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n)
}

// anchor slugs a heading the way GitHub does: strip the markdown, lowercase,
// drop everything that is not a letter, digit, underscore, hyphen or space,
// then turn spaces into hyphens.
//
// The punctuation rule is the one that matters here: an em dash is dropped but
// the spaces around it are not, so "FVF — Harmonic Wobbulator" becomes
// "fvf--harmonic-wobbulator", with the double hyphen.
func anchor(text string) string {
	text = stripInlineMarkdown(text)
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripInlineMarkdown removes the emphasis, code and link syntax a heading may
// carry, keeping the text GitHub slugs.
func stripInlineMarkdown(s string) string {
	s = strings.NewReplacer("**", "", "`", "", "*", "", "_", "").Replace(s)
	// [text](href) → text
	for {
		i := strings.Index(s, "](")
		if i < 0 {
			break
		}
		open := strings.LastIndex(s[:i], "[")
		close := strings.Index(s[i:], ")")
		if open < 0 || close < 0 {
			break
		}
		s = s[:open] + s[open+1:i] + s[i+close+1:]
	}
	return s
}

// mdEscape protects the few characters that would otherwise be read as
// markup in a link label or a table cell.
func mdEscape(s string) string {
	return strings.NewReplacer("|", "\\|", "[", "\\[", "]", "\\]").Replace(s)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// modelVariants are the extra faces a model has that one still cannot show
// alongside the first. The suffix names the image file; the head names the
// column.
//
// The turtle is why this exists: the same integer sequence walked in two
// dimensions and in three are not the same figure, and showing only the 3-D
// one hides half of what the mode does.
var modelVariants = map[string][]struct{ suffix, head string }{
	"turtle": {{"2d", "in 2-D (mod 30, closed)"}},
}

// tableText flattens a description into something that survives a markdown
// table cell: no raw newlines, no pipes, and equations kept on their own
// lines as code rather than run together into a sentence.
func tableText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	prose, equations := splitDescription(s)
	out := strings.ReplaceAll(mdEscape(prose), "\n", " ")
	if equations != "" {
		var lines []string
		for _, l := range strings.Split(equations, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, "<code>"+mdEscape(l)+"</code>")
			}
		}
		out += "<br><br>" + strings.Join(lines, "<br>")
	}
	return out
}
