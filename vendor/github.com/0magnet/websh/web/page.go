//go:build js && wasm

package web

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"syscall/js"
)

// The page a program runs in (PROTOCOL.md, The page; Files): its title and
// address while the program runs, files it offers, and the clipboard. A
// remote program gets the title and address of no one's page, and asks the
// person before it saves a file or fills the clipboard.

// pageState is the page as it was before the running program changed it.
type pageState struct {
	saved bool
	title string
	url   string
	// The link the page was opened by: a command line, and where in it.
	linkLine string
	linkPath string
}

func (s *Session) pageSave() {
	if !s.page.saved {
		s.page.saved = true
		s.page.title = js.Global().Get("document").Get("title").String()
		s.page.url = js.Global().Get("location").Get("href").String()
	}
}

// pageRestore puts the title and address back, as the program exits.
func (s *Session) pageRestore() {
	if !s.page.saved {
		return
	}
	js.Global().Get("document").Set("title", s.page.title)
	js.Global().Get("history").Call("replaceState", js.Null(), "", s.page.url)
	s.page.saved = false
}

// setTitle is OSC 0 and OSC 2: the page's title, while the program runs.
func (s *Session) setTitle(title string) {
	if !s.running || s.Shell.Source() == "remote" {
		return
	}
	s.pageSave()
	js.Global().Get("document").Set("title", title)
}

// setPage is OSC 7337 page: where the program is, put in the page's address
// so it can be linked to and opened again. The address carries the command
// that is running and the program's path in it, in the fragment, which
// every host — a static one too — hands back untouched.
func (s *Session) setPage(enc string) {
	if !s.running || s.Shell.Source() == "remote" {
		return
	}
	var m struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || json.Unmarshal(b, &m) != nil || len(m.Path) > 2048 || strings.ContainsAny(m.Path, "\x00\r\n") {
		return
	}
	s.pageSave()
	loc := js.Global().Get("location")
	frag := "run=" + url.QueryEscape(s.line)
	if m.Path != "" {
		frag += "&at=" + url.QueryEscape(m.Path)
	}
	js.Global().Get("history").Call("replaceState", js.Null(), "", loc.Get("pathname").String()+loc.Get("search").String()+"#"+frag)
	if m.Title != "" {
		js.Global().Get("document").Set("title", m.Title)
	}
}

// OpenLink reads the address the page was opened by. A link made by a
// program (setPage) carries a command; it is typed at the prompt for the
// person to run, never run for them — a link that ran commands would let
// anyone's page reach this shell's files — and the program it starts is
// told, through Discovery, where in it the link pointed.
func (s *Session) OpenLink() {
	frag := strings.TrimPrefix(js.Global().Get("location").Get("hash").String(), "#")
	v, err := url.ParseQuery(frag)
	if err != nil || v.Get("run") == "" {
		return
	}
	line := strings.Map(func(r rune) rune {
		if r < ' ' {
			return -1 // one line, and no control characters
		}
		return r
	}, v.Get("run"))
	s.page.linkLine, s.page.linkPath = line, v.Get("at")
	s.Editor.Input(line)
}

// linkPath is where in the running command the page's link pointed, if the
// command is the one the link carried.
func (s *Session) linkPath() string {
	if s.page.linkLine != "" && s.line == s.page.linkLine {
		return s.page.linkPath
	}
	return ""
}

// download is iTerm2's OSC 1337 File= with inline=0: a file the program
// offers, saved by the browser.
func (s *Session) download(data string) {
	head, body, ok := strings.Cut(data, ":")
	if !ok || !strings.HasPrefix(head, "File=") {
		return
	}
	args := map[string]string{}
	for _, kv := range strings.Split(strings.TrimPrefix(head, "File="), ";") {
		k, v, _ := strings.Cut(kv, "=")
		args[k] = v
	}
	if args["inline"] == "1" {
		return // a picture to show in place; not yet (PROTOCOL.md)
	}
	nb, err := base64.StdEncoding.DecodeString(args["name"])
	name := strings.Map(func(r rune) rune {
		if r < ' ' || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, string(nb))
	if err != nil || name == "" {
		name = "download"
	}
	b, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return
	}
	if s.Shell.Source() == "remote" && !js.Global().Call("confirm", "A program on another machine offers a file: "+name+" ("+strconv.Itoa(len(b))+" bytes). Save it?").Bool() {
		return
	}
	buf := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(buf, b)
	blob := js.Global().Get("Blob").New(js.ValueOf([]any{buf}), map[string]any{"type": "application/octet-stream"})
	u := js.Global().Get("URL").Call("createObjectURL", blob)
	a := js.Global().Get("document").Call("createElement", "a")
	a.Set("href", u)
	a.Set("download", name)
	a.Call("click")
	once(func() { js.Global().Get("URL").Call("revokeObjectURL", u) }, func(f js.Value) {
		js.Global().Call("setTimeout", f, 1000)
	})
}

// clipboard is OSC 52: the program fills the clipboard. Reading it is
// refused: what the person copied elsewhere is not a program's to see.
func (s *Session) clipboard(data string) {
	_, enc, ok := strings.Cut(data, ";")
	if !ok || enc == "?" {
		return
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return
	}
	if s.Shell.Source() == "remote" && !js.Global().Call("confirm", "A program on another machine wants to put "+strconv.Itoa(len(b))+" bytes on the clipboard. Allow it?").Bool() {
		return
	}
	if cb := js.Global().Get("navigator").Get("clipboard"); cb.Truthy() {
		p := cb.Call("writeText", string(b))
		once(func() {}, func(f js.Value) { p.Call("catch", f) }) // a page without focus may refuse; nothing to tell
	}
}

// once hands use a JS function that runs f the first time it is called and
// then lets itself go.
func once(f func(), use func(js.Value)) {
	var fn js.Func
	fn = js.FuncOf(func(js.Value, []js.Value) any {
		fn.Release()
		f()
		return nil
	})
	use(fn.Value)
}
