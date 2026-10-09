# cdp

A small Chrome DevTools Protocol client for Go tools that drive an
already-running Chromium, Chrome or Brave: UI tests, page probes, screenshots.
One dependency, [coder/websocket](https://github.com/coder/websocket).

It started as `internal/cdp` in [chaosrack](https://github.com/0magnet/chaosrack),
and exists so that the half-dozen hand-rolled copies across 0magnet and
skywire tools can be one.

```go
c, err := cdp.Dial(9222, "localhost:8080") // first page whose URL contains it
if err != nil {
	log.Fatal(err)
}
defer c.Close()

v, err := c.Evaluate(ctx, `document.title`)        // JSON; exceptions are errors
c.Click(120, 40)                                     // trusted input
c.Focus(ctx, `input[name="q"]`); c.Type(ctx, "1N4001"); c.Press(ctx, "Enter")
png, err := c.ScreenshotPNG(ctx)                     // canvases included
err = c.Do(ctx, "Emulation.setDeviceMetricsOverride", params, nil) // anything else
```

Start the browser with `--remote-debugging-port=9222`. `Dial` enables Runtime and
Page and brings the tab to the front (`Page.bringToFront`) so its timers and
`requestAnimationFrame` are not throttled. To leave the person's window alone,
use `DialBackground`, which turns on focus emulation instead; pair it with
`Browser.NewWindow` for a page that must keep rendering.

## What it is and is not

It is not chromedp. There is no browser launcher, no generated bindings and
no action DSL. A `Client` sends a method name and a params value and hands
back the result; the helpers cover what every tool ends up calling.

- **`Browser`** is the HTTP endpoint (`Local(port)` makes one): `Targets`,
  `Find`, `NewTab`, `NewWindow`, `CloseTab`, `Version`. `NewWindow` opens a page
  in a window of its own, behind the others, so the person's window keeps the
  focus. A tool that opens a tab or window for itself closes it again.
- **`Client`** is one target's websocket. A reader goroutine matches each
  reply to the command that asked for it, so the client is safe for
  concurrent use and a command that times out does not take the connection
  down. `Frozen` reports that one did: an unanswered command almost always
  means the page's main thread is stuck.
  `Dial`, `DialBackground` and `DialWS` (a bare `webSocketDebuggerUrl`, no
  domains enabled) make one. Beyond `Evaluate`, `Do`, `Click`, `Focus`, `Type`,
  `Press` and `ScreenshotPNG` it has `Navigate`, `Reload`, `Eval` and
  `EvalJSON` (best-effort, nil on failure), `Call`, `Mouse`, `Drag`, `Wheel`,
  `Keys`, `Screenshot` (an `image.Image`), and `Done` and `Err` for the
  connection's end.
- **`Events`** subscribes to everything the target emits once its domain is
  enabled (`Runtime.consoleAPICalled`, `Log.entryAdded`, ...). A full
  subscriber drops events rather than stalling replies.
- **`BrightFrac`, `BrightBBox`, `DiffFrac`** are cheap image oracles for tests
  that cannot know exactly what a frame should contain: is it blank,
  collapsed to a dot, or changed.

Firefox no longer speaks CDP; for it there is
[wfdrive/bidi](https://github.com/0magnet/wfdrive), which speaks WebDriver BiDi.

## Testing

`make test` runs against a fake browser. To run against a real one, in a tab
of the test's own that it closes again:

```
CDP_LIVE=127.0.0.1:9222 go test -run Live -v
```

## Prototyping commands

Chrome's DevTools Protocol Monitor has a [command editor](https://developer.chrome.com/blog/cdp-command-editor) with autocomplete and validation, useful for trying a command against a live page before adding it to the client.

## Related projects

Another Go tool built on the Chrome DevTools Protocol:

- [Hubcap](https://tomyandell.dev/blog/introducing-hubcap) — a single Go binary that drives Chrome over CDP, emitting JSON

## Dependency Graph

Made with [goda](https://github.com/loov/goda):

```
go run github.com/loov/goda@latest graph github.com/0magnet/cdp/... | dot -Tsvg -o docs/cdp-goda-graph.svg
```

![Dependency Graph](docs/cdp-goda-graph.svg "github.com/0magnet/cdp Dependency Graph")

## Lines of Code

Made with [gocloc](https://github.com/hhatto/gocloc) (excludes `vendor/`, `node_modules/`, `.git/`):

```
gocloc --not-match-d='(vendor|node_modules|\.git)' .
```

```
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                               7            110            171           1265
YAML                             1              0              7             98
Makefile                         1             19             34             89
Markdown                         1             25              0             86
Bourne Shell                     1              8             16             30
-------------------------------------------------------------------------------
TOTAL                           11            162            228           1568
-------------------------------------------------------------------------------
```
