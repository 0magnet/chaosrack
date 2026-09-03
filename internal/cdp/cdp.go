// Package cdp is a tiny Chrome DevTools Protocol client for the project's UI
// test tools (cmd/monkey, cmd/uigolden). It attaches to an already-open tab in
// a Chromium/Brave started with --remote-debugging-port, drives real input,
// evaluates JS, and captures screenshots — plus a couple of image helpers for
// "is the render blank / sane" oracles. Reuses the vendored
// golang.org/x/net/websocket (no extra deps).
package cdp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// Client is a single-tab CDP connection. Not safe for concurrent use.
type Client struct {
	ws     *websocket.Conn
	id     int
	Frozen bool // set when a CDP eval timed out → the page's JS main thread is blocked
	URL    string
}

// Dial connects to the tab whose URL contains urlSubstr on the given
// remote-debugging port.
func Dial(port int, urlSubstr string) (*Client, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return nil, fmt.Errorf("CDP list (is the browser running with --remote-debugging-port=%d?): %w", port, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("CDP list: %w", err)
	}
	var tabs []struct{ Type, URL, WebSocketDebuggerURL string }
	if err := json.Unmarshal(body, &tabs); err != nil {
		return nil, err
	}
	for _, t := range tabs {
		if t.Type == "page" && strings.Contains(t.URL, urlSubstr) && t.WebSocketDebuggerURL != "" {
			ws, err := websocket.Dial(t.WebSocketDebuggerURL, "", fmt.Sprintf("http://127.0.0.1:%d", port))
			if err != nil {
				return nil, err
			}
			ws.MaxPayloadBytes = 64 << 20 // screenshots are big
			c := &Client{ws: ws, URL: t.URL}
			_, _ = c.Call("Runtime.enable", nil)    //nolint:errcheck // a CDP domain command; a failure surfaces on the next call that needs it
			_, _ = c.Call("Page.enable", nil)       //nolint:errcheck // a CDP domain command; a failure surfaces on the next call that needs it
			_, _ = c.Call("Page.bringToFront", nil) //nolint:errcheck // avoid rAF/timer throttling while occluded
			return c, nil
		}
	}
	return nil, fmt.Errorf("no open tab whose URL contains %q", urlSubstr)
}

// Call sends a CDP command and returns its result, skipping events. If a
// response doesn't arrive within the deadline the page's JS main thread is
// blocked (a real freeze — CDP evals run on it); Frozen is set.
func (c *Client) Call(method string, params map[string]any) (map[string]any, error) {
	c.id++
	myID := c.id
	req, err := json.Marshal(map[string]any{"id": myID, "method": method, "params": params})
	if err != nil {
		return nil, fmt.Errorf("CDP %s: %w", method, err)
	}
	_ = c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck // only fails on a closed connection, which the Send/Receive below reports
	if err := websocket.Message.Send(c.ws, string(req)); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = c.ws.SetReadDeadline(deadline) //nolint:errcheck // only fails on a closed connection, which the Send/Receive below reports
		var s string
		if err := websocket.Message.Receive(c.ws, &s); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				c.Frozen = true
			}
			return nil, err
		}
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			continue
		}
		if idf, ok := m["id"].(float64); ok && int(idf) == myID {
			return m, nil
		}
	}
}

// Eval runs JS (awaiting a returned promise) and returns the by-value result.
func (c *Client) Eval(expr string) any {
	r, err := c.Call("Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true, "awaitPromise": true,
	})
	if err != nil {
		return nil
	}
	if res, ok := r["result"].(map[string]any); ok {
		if inner, ok := res["result"].(map[string]any); ok {
			return inner["value"]
		}
	}
	return nil
}

// EvalJSON runs JS that returns a JSON string and unmarshals it into a map.
func (c *Client) EvalJSON(expr string) map[string]any {
	v, _ := c.Eval(expr).(string)
	var m map[string]any
	if v != "" {
		_ = json.Unmarshal([]byte(v), &m) //nolint:errcheck // best-effort: callers treat missing keys as absent
	}
	return m
}

func (c *Client) Mouse(kind string, x, y float64, buttons int) {
	p := map[string]any{"type": kind, "x": x, "y": y, "buttons": buttons}
	if kind == "mousePressed" || kind == "mouseReleased" {
		p["button"] = "left"
		p["clickCount"] = 1
	}
	_, _ = c.Call("Input.dispatchMouseEvent", p) //nolint:errcheck // a CDP domain command; a failure surfaces on the next call that needs it
}

func (c *Client) Click(x, y float64) {
	c.Mouse("mouseMoved", x, y, 0)
	c.Mouse("mousePressed", x, y, 1)
	c.Mouse("mouseReleased", x, y, 0)
}

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

func (c *Client) Wheel(x, y, dy float64) {
	_, _ = c.Call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": dy}) //nolint:errcheck // a CDP domain command; a failure surfaces on the next call that needs it
}

// Screenshot captures the composited page (canvas included) as an image.
func (c *Client) Screenshot() (image.Image, error) {
	r, err := c.Call("Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return nil, err
	}
	res, _ := r["result"].(map[string]any)
	data, _ := res["data"].(string)
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

// Reload hard-reloads the tab and waits d for it to settle. Any injected page
// state (error hooks) is lost and must be reinstalled by the caller.
func (c *Client) Reload(d time.Duration) {
	_, _ = c.Call("Page.reload", map[string]any{"ignoreCache": true}) //nolint:errcheck // a CDP domain command; a failure surfaces on the next call that needs it
	time.Sleep(d)
}

// ── image oracles ─────────────────────────────────────────────────────────────

// BrightFrac returns the fraction of pixels in the region that are brighter
// than a near-black background (luminance > 24/255), i.e. how much is "drawn".
// A visual model render is thin bright lines on black, so it's small but > 0; a
// blank/failed render is ~0.
func BrightFrac(img image.Image, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return 0
	}
	var bright, total int
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			lum := (299*cr + 587*cg + 114*cb) / 1000 >> 8 // 0..255
			if lum > 24 {
				bright++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(bright) / float64(total)
}

// BrightBBox returns the bounding box of the drawn (bright) pixels within r —
// useful to spot a model collapsed to a dot (tiny box) or blown up off-screen
// (box hugging the edges). Empty if nothing is drawn.
func BrightBBox(img image.Image, r image.Rectangle) image.Rectangle {
	r = r.Intersect(img.Bounds())
	minX, minY, maxX, maxY := r.Max.X, r.Max.Y, r.Min.X, r.Min.Y
	found := false
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if (299*cr+587*cg+114*cb)/1000>>8 > 24 {
				found = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

// DiffFrac returns the fraction of sampled pixels that differ between a and b
// by more than tol per channel (0..255). Both images must be the same size.
func DiffFrac(a, b image.Image, tol int) float64 {
	ra, rb := a.Bounds(), b.Bounds()
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		return 1
	}
	var diff, total int
	t := uint32(tol) //nolint:gosec // a pixel dimension from the page; never negative
	for y := 0; y < ra.Dy(); y += 2 {
		for x := 0; x < ra.Dx(); x += 2 {
			ar, ag, ab, _ := a.At(ra.Min.X+x, ra.Min.Y+y).RGBA()
			br, bg, bb, _ := b.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
			if absu(ar>>8, br>>8) > t || absu(ag>>8, bg>>8) > t || absu(ab>>8, bb>>8) > t {
				diff++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(diff) / float64(total)
}

func absu(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
