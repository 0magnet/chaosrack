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
png, err := c.ScreenshotPNG(ctx)                     // canvases included
err = c.Do(ctx, "Emulation.setDeviceMetricsOverride", params, nil) // anything else
```

Start the browser with `--remote-debugging-port=9222`.

## What it is and is not

It is not chromedp. There is no browser launcher, no generated bindings and
no action DSL. A `Client` sends a method name and a params value and hands
back the result; the helpers cover what every tool ends up calling.

- **`Browser`** is the HTTP endpoint: `Targets`, `Find`, `NewTab`,
  `CloseTab`, `Version`. A tool that opens a tab for itself closes it again.
- **`Client`** is one target's websocket. A reader goroutine matches each
  reply to the command that asked for it, so the client is safe for
  concurrent use and a command that times out does not take the connection
  down. `Frozen` reports that one did: an unanswered command almost always
  means the page's main thread is stuck.
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
Go                               7             99            143           1058
YAML                             1              0              7             98
Makefile                         1             19             34             89
Markdown                         1             13              0             45
Bourne Shell                     1              8             16             30
-------------------------------------------------------------------------------
TOTAL                           11            139            200           1320
-------------------------------------------------------------------------------
```
