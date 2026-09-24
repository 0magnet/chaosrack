package cdp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"time"
)

// ExceptionError is a JavaScript exception thrown by an evaluated expression,
// or the rejection of the promise it returned.
type ExceptionError struct {
	Text string
}

func (e *ExceptionError) Error() string { return "cdp: exception: " + e.Text }

// Evaluate runs expr in the page, awaiting a returned promise, and returns its
// value as JSON. undefined comes back as nil. An exception is an
// *ExceptionError.
func (c *Client) Evaluate(ctx context.Context, expr string) (json.RawMessage, error) {
	var r struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	err := c.Do(ctx, "Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true, "awaitPromise": true,
	}, &r)
	if err != nil {
		return nil, err
	}
	if d := r.ExceptionDetails; d != nil {
		text := d.Text
		if d.Exception != nil && d.Exception.Description != "" {
			text = d.Exception.Description // carries the message and the stack
		}
		return nil, &ExceptionError{Text: text}
	}
	return r.Result.Value, nil
}

// Eval runs expr with the Client's Timeout and returns its value decoded into
// a Go value (float64, string, bool, map, slice), or nil on any failure. It is
// for test oracles that treat "no answer" as an answer; use Evaluate to see
// why.
func (c *Client) Eval(expr string) any {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	raw, err := c.Evaluate(ctx, expr)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

// EvalJSON runs JS that returns a JSON string and decodes it into a map. Keys
// are simply absent when anything goes wrong.
func (c *Client) EvalJSON(expr string) map[string]any {
	v, _ := c.Eval(expr).(string)
	var m map[string]any
	if v != "" {
		_ = json.Unmarshal([]byte(v), &m) //nolint:errcheck // best-effort: callers treat missing keys as absent
	}
	return m
}

// Navigate loads rawURL in the page. It returns once the navigation has been
// committed, not when the page has finished loading.
func (c *Client) Navigate(ctx context.Context, rawURL string) error {
	var r struct {
		ErrorText string `json:"errorText"`
	}
	if err := c.Do(ctx, "Page.navigate", map[string]any{"url": rawURL}, &r); err != nil {
		return err
	}
	if r.ErrorText != "" {
		return fmt.Errorf("cdp: navigate %s: %s", rawURL, r.ErrorText)
	}
	return nil
}

// Reload hard-reloads the tab, bypassing the cache, and waits d for it to
// settle. Whatever the caller injected into the page is gone afterwards.
func (c *Client) Reload(d time.Duration) {
	_, _ = c.Call("Page.reload", map[string]any{"ignoreCache": true}) //nolint:errcheck // a failure surfaces on the next call that needs the page
	time.Sleep(d)
}

// ScreenshotPNG captures the composited page, canvases included, as PNG bytes.
func (c *Client) ScreenshotPNG(ctx context.Context) ([]byte, error) {
	var r struct {
		Data string `json:"data"`
	}
	if err := c.Do(ctx, "Page.captureScreenshot", map[string]any{"format": "png"}, &r); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(r.Data)
}

// Screenshot captures the composited page with the Client's Timeout and
// decodes it.
func (c *Client) Screenshot() (image.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	b, err := c.ScreenshotPNG(ctx)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

// Mouse dispatches one trusted mouse event: kind is mouseMoved, mousePressed,
// mouseReleased or mouseWheel, and buttons is the bitmask of buttons held
// (1 = left). Press and release are of the left button.
func (c *Client) Mouse(kind string, x, y float64, buttons int) {
	p := map[string]any{"type": kind, "x": x, "y": y, "buttons": buttons}
	if kind == "mousePressed" || kind == "mouseReleased" {
		p["button"] = "left"
		p["clickCount"] = 1
	}
	_, _ = c.Call("Input.dispatchMouseEvent", p) //nolint:errcheck // input is fire-and-forget; the page's reaction is the result
}

// Click moves to (x, y) and clicks the left button there.
func (c *Client) Click(x, y float64) {
	c.Mouse("mouseMoved", x, y, 0)
	c.Mouse("mousePressed", x, y, 1)
	c.Mouse("mouseReleased", x, y, 0)
}

// Drag presses at (x1, y1), moves to (x2, y2) in n steps 12ms apart, and
// releases, which is slow enough for pointer handlers that sample movement.
func (c *Client) Drag(x1, y1, x2, y2 float64, n int) {
	c.Mouse("mouseMoved", x1, y1, 0)
	c.Mouse("mousePressed", x1, y1, 1)
	for k := 1; k <= n; k++ {
		f := float64(k) / float64(n)
		c.Mouse("mouseMoved", x1+(x2-x1)*f, y1+(y2-y1)*f, 1)
		time.Sleep(12 * time.Millisecond)
	}
	c.Mouse("mouseReleased", x2, y2, 0)
}

// Wheel scrolls by dy pixels with the pointer at (x, y).
func (c *Client) Wheel(x, y, dy float64) {
	_, _ = c.Call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": dy}) //nolint:errcheck // input is fire-and-forget; the page's reaction is the result
}
