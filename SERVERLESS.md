# Serverless builds

chaosrack runs without its server in two forms: the deployed pages, which fetch the wasm, and a single self-contained HTML file. See the [README](README.md#run) for the server.

There are two builds, and the difference is whether the binary is *in* the page.

**The deployed pages fetch it.** `make pages` writes
[index.html](index.html) · [go/index.html](go/index.html) ·
[tinygo/index.html](tinygo/index.html), which are 70 KB, 52 KB and 52 KB and
ask for the `.wasm` in `assets/` as an ordinary resource — the copy already in
this repository, so no binary is duplicated to publish it. That is better than
inlining on every axis that matters to a page someone loads: only the runtime
actually being run is fetched, where the inlined dual page carried both and
transferred 9.5 MB to start one of them; `instantiateStreaming` compiles the
module *while* it downloads instead of after; no base64 is parsed and nothing
is inflated in JavaScript; and the host serves `.wasm` gzipped, so the binary
costs the same on the wire either way — 5.7 MB for the Go build.

**`make onefile` builds the other one.**
[chaosrack-standalone.html](chaosrack-standalone.html)
([download](https://chaosrack.magnetosphere.net/chaosrack-standalone.html)) is 12.3 MB
with both runtimes gzipped and base64'd inside it, and it needs nothing else at
all: no server, no sibling files, no network after it is saved. That is the one
to keep, mail, or open off a disk, and it is why the inlining code is still
here rather than deleted with the payload the deployed pages stopped carrying.

Gzipping before base64 matters more than it sounds, and still does for that
build. A 6 MB wasm base64s to 8.4 MB of JavaScript source the browser must
receive, parse and `atob` before anything starts; gzipped it is 2.2 MB.
`DecompressionStream` does the inflating — no library, present since Chrome 80,
Firefox 113 and Safari 16.4 — and where it is missing the page falls back to
fetching the wasm as an ordinary resource, which works beside a server and not
inside the single file, where there is nothing to fetch.

In that build the payload sits at the **end of the document**, in a
`text/plain` script element the boot code reads with `textContent`, and the
boot code sits after it. The browser does not care; everything that reads the
page without running it does. A crawler fetches a bounded prefix and discards
the rest — Google documents 2 MB, measured on the uncompressed bytes — so with
the payload in `<head>` the page's own description sat at about byte
13,500,000 and was never read by anything. It is at 16,667 now, and the
deployed pages have no payload in front of it at all.

It is also written out **unescaped**, which is worth a line because the two
ways of getting that wrong look nothing alike. In a JavaScript string literal
`html/template` turns every `+` into a six-byte unicode escape and every `/`
into an escaped slash, which JavaScript decodes back — correct, and about
1.4 MB a page. In the element it
now lives in, the same escaper turns `+` into `&#43;`, which is not base64 any
more and fails only in the browser, at `atob`, with the page otherwise looking
perfectly well-formed. `pkg/server` types it so neither happens, and
`page_test.go` decodes what the template actually wrote and requires a wasm
back out of it.

