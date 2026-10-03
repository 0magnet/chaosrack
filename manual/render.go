package manual

// The manual's markdown is rendered by miniRender (mini.go), not by a
// markdown library: goldmark, the one it used, compiles regexps at start-up
// that overflow a TinyGo goroutine's stack and brought the TinyGo build down
// before main, and it was most of the manual's weight in either build for a
// handful of constructs. The tests still hold the result to goldmark's.

// Render is markdown as HTML. Raw HTML in it is left out.
func Render(md string) string { return miniRender(md, false) }

// renderPage is a whole file as HTML, its headings given ids (Page).
func renderPage(src string) string { return miniRender(src, true) }
