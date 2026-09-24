// Package cdp is a small Chrome DevTools Protocol client for driving an
// already-running Chromium, Chrome or Brave from Go tools: UI tests, page
// probes, screenshots and the like.
//
// It is deliberately not chromedp. There is no browser launcher, no generated
// protocol bindings and no action DSL: a Client sends a method name with a
// params value and hands back the result, and a handful of helpers cover the
// calls every tool ends up making (evaluate, screenshot, navigate, mouse
// input). Anything else in the protocol is one Do away.
//
// A browser started with --remote-debugging-port=9222 is reached with
//
//	c, err := cdp.Dial(9222, "localhost:8080")
//
// which attaches to the first page whose URL contains the substring. Browser
// lists, opens and closes tabs over the HTTP endpoints; DialWS attaches to a
// webSocketDebuggerUrl directly.
//
// Firefox no longer speaks CDP. For it, see github.com/0magnet/wfdrive/bidi.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// DefaultTimeout bounds a Call, which takes no context. A command that has not
// been answered by then means the page's JS main thread is blocked (evaluation
// runs on it), and the Client records that as Frozen.
const DefaultTimeout = 10 * time.Second

// Error is an error the browser returned in answer to a command.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("cdp: %s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("cdp: %s (%d)", e.Message, e.Code)
}

// ErrClosed is returned by commands sent after the connection has ended.
var ErrClosed = errors.New("cdp: connection closed")

// Event is a message the browser sent unprompted, after the domain that emits
// it has been enabled (Runtime.enable, Log.enable, ...).
type Event struct {
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	SessionID string          `json:"sessionId,omitempty"`
}

type message struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type reply struct {
	msg message
	raw []byte
}

// Client is one CDP connection, normally to a single page target. It is safe
// for concurrent use: a reader goroutine matches each response to the command
// that asked for it and hands events to subscribers, so a command that times
// out does not take the connection down with it.
type Client struct {
	// URL is the page's URL at the time it was attached, when known.
	URL string
	// Timeout bounds Call and the helpers built on it. Zero means
	// DefaultTimeout.
	Timeout time.Duration

	ws      *websocket.Conn
	nextID  atomic.Int64
	frozen  atomic.Bool
	closing atomic.Bool

	mu      sync.Mutex
	pending map[int64]chan reply
	subs    map[chan Event]struct{}
	err     error
	done    chan struct{}
}

// DialWS attaches to a webSocketDebuggerUrl. No domain is enabled; Dial is the
// convenience that also enables Runtime and Page.
func DialWS(ctx context.Context, wsURL string) (*Client, error) {
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: dial %s: %w", wsURL, err)
	}
	ws.SetReadLimit(64 << 20) // a full-page PNG arrives as one message
	c := &Client{
		ws:      ws,
		pending: make(map[int64]chan reply),
		subs:    make(map[chan Event]struct{}),
		done:    make(chan struct{}),
	}
	go c.read() //nolint:gosec // the reader lives as long as the connection, not the dial's context
	return c, nil
}

// Dial attaches to the first page on a local browser's remote-debugging port
// whose URL contains urlSubstr (any page, when it is empty), enables Runtime
// and Page, and brings the tab to the front so that its timers and
// requestAnimationFrame are not throttled.
func Dial(port int, urlSubstr string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	t, err := Local(port).Find(ctx, urlSubstr)
	if err != nil {
		return nil, err
	}
	c, err := DialWS(ctx, t.WebSocketDebuggerURL)
	if err != nil {
		return nil, err
	}
	c.URL = t.URL
	for _, m := range []string{"Runtime.enable", "Page.enable", "Page.bringToFront"} {
		if err := c.Do(ctx, m, nil, nil); err != nil {
			_ = c.Close() //nolint:errcheck // the enable failure is the error worth reporting
			return nil, fmt.Errorf("cdp: %s: %w", m, err)
		}
	}
	return c, nil
}

// Close ends the connection. Commands waiting on it return ErrClosed.
func (c *Client) Close() error {
	c.closing.Store(true)
	return c.ws.Close(websocket.StatusNormalClosure, "")
}

// Done is closed when the connection ends, for whatever reason; Err says why.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err is why the connection ended, or nil while it is open.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Frozen reports whether a Call has timed out. A command that is never
// answered almost always means the page's main thread is stuck in a loop.
func (c *Client) Frozen() bool { return c.frozen.Load() }

// Events subscribes to every event the connection receives. Events are
// delivered in order; when the buffer of buf is full, further events are
// dropped rather than stalling responses to commands, so size it for the
// burst you expect and drain it. The channel is closed by cancel or when the
// connection ends.
func (c *Client) Events(buf int) (events <-chan Event, cancel func()) {
	ch := make(chan Event, buf)
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	c.subs[ch] = struct{}{}
	c.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if _, ok := c.subs[ch]; ok {
				delete(c.subs, ch)
				close(ch)
			}
		})
	}
}

// Do sends a command and waits for its answer, unmarshaling the result into
// out when out is non-nil. params may be nil. A protocol error comes back as
// an *Error.
func (c *Client) Do(ctx context.Context, method string, params, out any) error {
	r, err := c.send(ctx, method, params)
	if err != nil {
		return err
	}
	if r.msg.Error != nil {
		return r.msg.Error
	}
	if out != nil && len(r.msg.Result) > 0 {
		if err := json.Unmarshal(r.msg.Result, out); err != nil {
			return fmt.Errorf("cdp: %s result: %w", method, err)
		}
	}
	return nil
}

// Call sends a command with the Client's Timeout and returns the whole
// response message decoded as a map, so the result is at m["result"]. A
// timeout marks the Client Frozen. A protocol error is returned as an *Error
// alongside the message.
func (c *Client) Call(method string, params map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	r, err := c.send(ctx, method, params)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(r.raw, &m); err != nil {
		return nil, fmt.Errorf("cdp: %s: %w", method, err)
	}
	if r.msg.Error != nil {
		return m, r.msg.Error
	}
	return m, nil
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c *Client) send(ctx context.Context, method string, params any) (reply, error) {
	if params == nil {
		params = struct{}{}
	}
	id := c.nextID.Add(1)
	b, err := json.Marshal(struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{id, method, params})
	if err != nil {
		return reply{}, fmt.Errorf("cdp: %s: %w", method, err)
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return reply{}, ErrClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		return reply{}, fmt.Errorf("cdp: %s: %w", method, err)
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return reply{}, ErrClosed
		}
		return r, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			c.frozen.Store(true)
		}
		return reply{}, fmt.Errorf("cdp: %s: %w", method, ctx.Err())
	}
}

// read owns the socket's read side for the life of the connection.
func (c *Client) read() {
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.shutdown(err)
			return
		}
		var m message
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			c.mu.Unlock()
			if ch != nil {
				ch <- reply{msg: m, raw: data} // buffered, and each id is answered once
			}
			continue
		}
		if m.Method == "" {
			continue
		}
		ev := Event{Method: m.Method, Params: m.Params, SessionID: m.SessionID}
		c.mu.Lock()
		for ch := range c.subs {
			select {
			case ch <- ev:
			default:
			}
		}
		c.mu.Unlock()
	}
}

func (c *Client) shutdown(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing.Load() || websocket.CloseStatus(err) == websocket.StatusNormalClosure {
		err = ErrClosed
	}
	c.err = err
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	for ch := range c.subs {
		close(ch)
		delete(c.subs, ch)
	}
	close(c.done)
}
