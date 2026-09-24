package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Browser is a browser's remote-debugging HTTP endpoint, which lists, opens
// and closes targets. Each target then has a websocket of its own.
type Browser struct {
	// Addr is host:port, e.g. "127.0.0.1:9222".
	Addr string
}

// Local is the browser listening on port on the loopback interface.
func Local(port int) Browser {
	return Browser{Addr: "127.0.0.1:" + strconv.Itoa(port)}
}

// Target is one entry of /json/list: a page, worker, iframe or extension.
type Target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// Version is /json/version.
type Version struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// Targets lists everything the browser will let a debugger attach to.
func (b Browser) Targets(ctx context.Context) ([]Target, error) {
	var ts []Target
	if err := b.get(ctx, http.MethodGet, "/json/list", &ts); err != nil {
		return nil, err
	}
	return ts, nil
}

// Find returns the first page whose URL contains urlSubstr; an empty
// substring matches any page.
func (b Browser) Find(ctx context.Context, urlSubstr string) (Target, error) {
	ts, err := b.Targets(ctx)
	if err != nil {
		return Target{}, err
	}
	for _, t := range ts {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" && strings.Contains(t.URL, urlSubstr) {
			return t, nil
		}
	}
	if urlSubstr == "" {
		return Target{}, fmt.Errorf("cdp: no page target on %s", b.Addr)
	}
	return Target{}, fmt.Errorf("cdp: no open page on %s whose URL contains %q", b.Addr, urlSubstr)
}

// NewTab opens a tab at rawURL (about:blank when empty) and returns it. A
// tool that opens a tab for itself should CloseTab it when done.
func (b Browser) NewTab(ctx context.Context, rawURL string) (Target, error) {
	path := "/json/new"
	if rawURL != "" {
		path += "?" + url.QueryEscape(rawURL)
	}
	var t Target
	// Chrome 111+ refuses a GET here; PUT works on every version still around.
	if err := b.get(ctx, http.MethodPut, path, &t); err != nil {
		return Target{}, err
	}
	if t.WebSocketDebuggerURL == "" {
		return Target{}, fmt.Errorf("cdp: the new tab on %s has no debugger URL", b.Addr)
	}
	return t, nil
}

// CloseTab closes the target with the given ID.
func (b Browser) CloseTab(ctx context.Context, id string) error {
	return b.get(ctx, http.MethodGet, "/json/close/"+url.PathEscape(id), nil)
}

// Version reports the browser's product and protocol version, and the
// browser-level websocket that Target.* commands are sent on.
func (b Browser) Version(ctx context.Context) (Version, error) {
	var v Version
	err := b.get(ctx, http.MethodGet, "/json/version", &v)
	return v, err
}

func (b Browser) get(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+b.Addr+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cdp: %s (is the browser running with --remote-debugging-port?): %w", b.Addr, err)
	}
	defer resp.Body.Close() //nolint:errcheck // a read-only body
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cdp: %s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("cdp: %s: %w", path, err)
	}
	return nil
}
