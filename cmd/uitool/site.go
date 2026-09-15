// Subcommand site: write a page per model, for the things that read this site
// without running it.
//
//	uitool site              # write models/ and refresh sitemap.xml
//	uitool site -check       # exit 1 if they are out of date, changing nothing
//
// The app is one URL that draws sixty-odd systems on a canvas. To a search
// engine that is one page about nothing in particular, and none of the words
// someone would actually type — "Chua double scroll", "Chirikov standard map",
// "Takens delay embedding of live audio" — appear anywhere a crawler can read
// them, because they are drawn, not written.
//
// So each model gets a real page with its prose, its equations, its captured
// stills, and a link into the app at the hash that selects it. The text is the
// same text the Info overlay shows, from the same attractor.Catalog() the
// README and the mode-selector knobs come from, so this cannot drift from what
// the app actually has: adding a mode adds a page.
package main

import (
	"bytes"
	"flag"
	"fmt"
	htmpl "html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

var (
	siteDir   = flag.String("site", "models", "directory to write the per-model pages into")
	siteBase  = flag.String("site-base", "https://chaosrack.magnetosphere.net", "site root, for canonical URLs")
	siteMap   = flag.String("sitemap", "sitemap.xml", "the sitemap to rewrite")
	siteCheck = flag.Bool("site-check", false, "report whether the pages are up to date; write nothing")
)

// modelPage is one model, flattened for the template.
type modelPage struct {
	Key, Label, Class string
	CSS               htmpl.CSS
	Paras             []string
	Equations         string
	Still, Loop, Prms string // image URLs, empty when not captured
	Group             string
	PrevKey, PrevLbl  string
	NextKey, NextLbl  string
	Title, Desc       string
	Canonical, Image  string
	AppURL            string
}

// indexPage is the hub the model pages hang off.
type indexPage struct {
	Groups          []indexGroup
	Title, Desc     string
	Canonical, Base string
	CSS             htmpl.CSS
}

type indexGroup struct {
	Label  string
	Models []indexEntry
}

type indexEntry struct {
	Key, Label, Class, Still, Blurb string
	Repeat                          bool // listed again under a second category
}

func runSite() {
	pages, index := buildSite()

	files := map[string][]byte{}
	for _, p := range pages {
		b, err := render(modelTmpl, p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "site:", err)
			os.Exit(1)
		}
		files[filepath.Join(*siteDir, p.Key+".html")] = b
	}
	b, err := render(indexTmpl, index)
	if err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
	files[filepath.Join(*siteDir, "index.html")] = b
	files[*siteMap] = sitemap(pages)

	if *siteCheck {
		stale := 0
		for path, want := range files {
			got, err := os.ReadFile(path) //nolint:gosec // paths this command just composed from its own flags
			if err != nil || !bytes.Equal(got, want) {
				fmt.Fprintln(os.Stderr, "site: out of date:", path)
				stale++
			}
		}
		if stale > 0 {
			fmt.Fprintf(os.Stderr, "site: %d file(s) out of date — run `go run ./cmd/uitool site`\n", stale)
			os.Exit(1)
		}
		fmt.Println("site: up to date")
		return
	}

	if err := os.MkdirAll(*siteDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
	written := 0
	for path, body := range files {
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, body) { //nolint:gosec // as above
			continue
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "site:", err)
			os.Exit(1)
		}
		written++
	}
	fmt.Printf("site: %d pages in %s/ (%d written, %d unchanged)\n",
		len(files)-1, *siteDir, written, len(files)-written)
}

// buildSite flattens the catalog into pages. A model in two categories gets
// ONE page — the selector repeats it, a URL cannot — and the index says so
// in the second place rather than linking somewhere different.
func buildSite() ([]modelPage, indexPage) {
	var pages []modelPage
	var index indexPage
	seen := map[string]bool{}

	for _, g := range attractor.Catalog() {
		ig := indexGroup{Label: g.Label}
		for _, m := range g.Models {
			ig.Models = append(ig.Models, indexEntry{
				Key: m.Key, Label: m.Label, Class: m.Class.String(),
				Still: imgURL(m.Key, ".jpg"), Blurb: blurb(m.Description),
				Repeat: seen[m.Key],
			})
			if seen[m.Key] {
				continue
			}
			seen[m.Key] = true

			prose, equations := splitDescription(m.Description)
			p := modelPage{
				Key: m.Key, Label: m.Label, Class: m.Class.String(),
				Paras:     paragraphs(prose),
				Equations: equations,
				Still:     imgURL(m.Key, ".jpg"),
				Loop:      imgURL(m.Key, ".gif"),
				Prms:      imgURL(m.Key, "-params.jpg"),
				Group:     g.Label,
				// sharedCSS is this tool's own stylesheet, not input. htmpl.CSS is
				// the type that says so, which is exactly its purpose.
				CSS:       htmpl.CSS(sharedCSS), //nolint:gosec // our own stylesheet, not user input
				Canonical: *siteBase + "/" + *siteDir + "/" + m.Key + ".html",
				AppURL:    *siteBase + "/#" + m.Key,
			}
			// og:image must be absolute — an unfurler has no page to resolve a
			// relative path against.
			p.Image = imgAbs(m.Key, ".jpg")
			if p.Image == "" {
				p.Image = *siteBase + "/docs/img/hero.jpg"
			}
			// "Hénon — discrete map, drawn live in the browser" says what it
			// is and that you can turn it, which is the part that separates
			// this from the encyclopedia entry someone has already read.
			p.Title = fmt.Sprintf("%s — %s, drawn live in the browser · chaosrack", m.Label, p.Class)
			p.Desc = blurb(m.Description)
			pages = append(pages, p)
		}
		index.Groups = append(index.Groups, ig)
	}

	for i := range pages {
		if i > 0 {
			pages[i].PrevKey, pages[i].PrevLbl = pages[i-1].Key, pages[i-1].Label
		}
		if i < len(pages)-1 {
			pages[i].NextKey, pages[i].NextLbl = pages[i+1].Key, pages[i+1].Label
		}
	}

	index.Title = fmt.Sprintf("All %d models — chaosrack", len(pages))
	index.Desc = fmt.Sprintf("Every one of the %d systems chaosrack draws: continuous flows, "+
		"discrete maps, parametric figures, geometry and live-audio embeddings — "+
		"each with its equations and a link that opens it running in the browser.", len(pages))
	index.Canonical = *siteBase + "/" + *siteDir + "/"
	index.Base = *siteBase
	index.CSS = htmpl.CSS(sharedCSS) //nolint:gosec // our own stylesheet, not user input
	return pages, index
}

// imgURL returns the path to a captured image RELATIVE to a page in the model
// directory, or "" when that capture does not exist — a model with no
// parameters of its own has no parameters shot, and an <img> pointing at a 404
// is worse than no <img>. Relative so the pages can be opened from disk and
// from a local server without rewriting; og:image has to be absolute and uses
// imgAbs instead.
func imgURL(key, suffix string) string {
	if imgAbs(key, suffix) == "" {
		return ""
	}
	return "../" + filepath.ToSlash(filepath.Join(*modelImgDir, key+suffix))
}

func imgAbs(key, suffix string) string {
	path := filepath.Join(*modelImgDir, key+suffix)
	if !fileExists(path) {
		return ""
	}
	return *siteBase + "/" + filepath.ToSlash(path)
}

// blurb is the meta description: the first sentence or two of the prose,
// trimmed to something a result page will show rather than cut mid-word.
func blurb(desc string) string {
	prose, _ := splitDescription(desc)
	prose = strings.Join(strings.Fields(strings.ReplaceAll(prose, "\n", " ")), " ")
	// Counted and cut in runes: these descriptions are full of σ, ρ, β, é and
	// em dashes, and slicing a byte index through one of them writes a broken
	// rune into the meta description.
	const limit = 180
	r := []rune(prose)
	if len(r) <= limit {
		return prose
	}
	cut := string(r[:limit])
	if i := strings.LastIndexAny(cut, ".!?"); i > limit/2 {
		return cut[:i+1]
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		return cut[:i] + "…"
	}
	return cut
}

func paragraphs(prose string) []string {
	var out []string
	for _, p := range strings.Split(prose, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func render(t *htmpl.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sitemap(pages []modelPage) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	fmt.Fprintf(&b, "  <url><loc>%s/</loc><changefreq>monthly</changefreq><priority>1.0</priority></url>\n", *siteBase)
	fmt.Fprintf(&b, "  <url><loc>%s/%s/</loc><changefreq>monthly</changefreq><priority>0.9</priority></url>\n", *siteBase, *siteDir)
	for _, p := range pages {
		fmt.Fprintf(&b, "  <url><loc>%s</loc><changefreq>monthly</changefreq><priority>0.7</priority></url>\n", p.Canonical)
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}

const sharedCSS = `
:root { color-scheme: light dark;
        --bg:#ffffff; --fg:#15171c; --muted:#5c6470; --line:#e4e7ec; --accent:#7b4cd8; --card:#f7f8fa; }
@media (prefers-color-scheme: dark) {
  :root { --bg:#0d0f14; --fg:#e6e9ee; --muted:#8b94a3; --line:#232833; --accent:#b08cff; --card:#141821; }
}
* { box-sizing: border-box; }
body { margin:0; background:var(--bg); color:var(--fg);
       font:16px/1.65 ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif; }
.wrap { max-width: 900px; margin:0 auto; padding: 2rem 1.1rem 5rem; }
a { color:var(--accent); }
header nav { font-size:.86rem; color:var(--muted); margin-bottom:2rem; }
header nav a { color:var(--muted); text-decoration:none; }
header nav a:hover { color:var(--accent); text-decoration:underline; }
h1 { font-size:1.9rem; line-height:1.2; margin:0 0 .3rem; letter-spacing:-.01em; }
.class { color:var(--muted); font-size:.85rem; text-transform:uppercase; letter-spacing:.09em; margin-bottom:1.6rem; }
.shots { display:grid; gap:.9rem; grid-template-columns:repeat(auto-fit,minmax(210px,1fr)); margin:0 0 2rem; }
figure { margin:0; }
figure img { width:100%; height:auto; display:block; border:1px solid var(--line); border-radius:9px; background:#000; }
figcaption { color:var(--muted); font-size:.78rem; margin-top:.4rem; text-transform:uppercase; letter-spacing:.07em; }
pre { background:var(--card); border:1px solid var(--line); border-radius:9px;
      padding:1rem; overflow-x:auto; font:14px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace; }
.open { display:inline-block; margin:1.6rem 0; padding:.75rem 1.3rem; border-radius:8px;
        background:var(--accent); color:#fff; font-weight:600; text-decoration:none; }
.open:hover { filter:brightness(1.1); }
.pager { display:flex; justify-content:space-between; gap:1rem; flex-wrap:wrap;
         border-top:1px solid var(--line); margin-top:3rem; padding-top:1.2rem; font-size:.9rem; }
.grid { display:grid; gap:1.1rem; grid-template-columns:repeat(auto-fill,minmax(215px,1fr)); margin:.6rem 0 2.6rem; }
.card { border:1px solid var(--line); border-radius:10px; overflow:hidden; background:var(--card);
        text-decoration:none; color:inherit; display:flex; flex-direction:column; }
.card:hover { border-color:var(--accent); }
.card img { width:100%; aspect-ratio:1/1; object-fit:cover; display:block; background:#000; }
.card .body { padding:.6rem .75rem .8rem; }
.card b { display:block; font-size:.95rem; }
.card span { color:var(--muted); font-size:.76rem; }
h2 { font-size:1.15rem; margin:2.4rem 0 .2rem; padding-bottom:.45rem; border-bottom:1px solid var(--line); }
.lead { color:var(--muted); max-width:68ch; }
`

var modelTmpl = htmpl.Must(htmpl.New("model").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="description" content="{{.Desc}}">
<meta name="author" content="0magnet">
<link rel="canonical" href="{{.Canonical}}">
<meta property="og:type" content="article">
<meta property="og:site_name" content="magnetosphere">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Desc}}">
<meta property="og:url" content="{{.Canonical}}">
<meta property="og:image" content="{{.Image}}">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Desc}}">
<meta name="twitter:image" content="{{.Image}}">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"WebPage","name":{{.Title}},"url":{{.Canonical}},
"description":{{.Desc}},"primaryImageOfPage":{{.Image}},
"isPartOf":{"@type":"WebSite","name":"chaosrack","url":"https://chaosrack.magnetosphere.net/"}}
</script>
<style>{{.CSS}}</style>
</head>
<body>
<div class="wrap">
<header><nav><a href="../">chaosrack</a> › <a href="./">models</a> › {{.Group}}</nav></header>
<h1>{{.Label}}</h1>
<div class="class">{{.Class}} · <code>#{{.Key}}</code></div>
{{if or .Still .Loop .Prms}}<div class="shots">
{{if .Still}}<figure><img src="{{.Still}}" alt="{{.Label}} drawn by chaosrack" loading="lazy"><figcaption>{{.Label}}</figcaption></figure>{{end}}
{{if .Loop}}<figure><img src="{{.Loop}}" alt="{{.Label}} turning through the palette" loading="lazy"><figcaption>turning</figcaption></figure>{{end}}
{{if .Prms}}<figure><img src="{{.Prms}}" alt="The parameter knobs for {{.Label}}" loading="lazy"><figcaption>parameters</figcaption></figure>{{end}}
</div>{{end}}
{{range .Paras}}<p>{{.}}</p>
{{end}}
{{if .Equations}}<pre>{{.Equations}}</pre>{{end}}
<a class="open" href="{{.AppURL}}">Open {{.Label}} in chaosrack →</a>
<p class="lead">It opens running, on the model selector, with its own knobs. Rotate it by dragging,
zoom with the wheel, and turn the parameters to see what the figure does when the system changes.</p>
<div class="pager">
<span>{{if .PrevKey}}← <a href="{{.PrevKey}}.html">{{.PrevLbl}}</a>{{end}}</span>
<span><a href="./">all models</a></span>
<span>{{if .NextKey}}<a href="{{.NextKey}}.html">{{.NextLbl}}</a> →{{end}}</span>
</div>
</div>
</body>
</html>
`))

var indexTmpl = htmpl.Must(htmpl.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="description" content="{{.Desc}}">
<meta name="author" content="0magnet">
<link rel="canonical" href="{{.Canonical}}">
<meta property="og:type" content="website">
<meta property="og:site_name" content="magnetosphere">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Desc}}">
<meta property="og:url" content="{{.Canonical}}">
<meta property="og:image" content="{{.Base}}/docs/img/hero.jpg">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Desc}}">
<meta name="twitter:image" content="{{.Base}}/docs/img/hero.jpg">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"CollectionPage","name":{{.Title}},"url":{{.Canonical}},
"description":{{.Desc}},
"isPartOf":{"@type":"WebSite","name":"chaosrack","url":"https://chaosrack.magnetosphere.net/"}}
</script>
<style>{{.CSS}}</style>
</head>
<body>
<div class="wrap">
<header><nav><a href="../">chaosrack</a> › models</nav></header>
<h1>Every model</h1>
<p class="lead">{{.Desc}} They are listed in the order the model-selector knob turns
through them, which is the order they are in the app. A model that appears under two
categories is shown under both, because that is what the selector does.</p>
{{range .Groups}}
<h2>{{.Label}}</h2>
<div class="grid">
{{range .Models}}<a class="card" href="{{.Key}}.html">
{{if .Still}}<img src="{{.Still}}" alt="{{.Label}}" loading="lazy">{{end}}
<div class="body"><b>{{.Label}}</b><span>{{.Class}}{{if .Repeat}} · also above{{end}}</span></div>
</a>
{{end}}</div>
{{end}}
</div>
</body>
</html>
`))
