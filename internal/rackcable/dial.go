//go:build !js

package rackcable

import (
	"context"
	"fmt"

	"github.com/0magnet/cdp"
	"github.com/0magnet/chaosrack/pkg/memreport"
)

// Dial finds the rack on a browser's remote-debugging port and brings its tab
// to the front, so its frames are not throttled: what a panel driving the
// rack needs.
func Dial(port int, target string) (*Client, error) {
	c, err := cdp.Dial(port, target)
	if err != nil {
		return nil, err
	}
	return Over(c)
}

// DialBackground finds the rack the same way and leaves the browser as it is:
// for reading or setting a value, which needs no frames and has no business
// raising someone's window. A cable dialed this way can also ask the browser
// for the tab's memory (Client.Mem fills in Browser).
func DialBackground(port int, target string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cdp.DefaultTimeout)
	defer cancel()
	t, err := cdp.Local(port).Find(ctx, target)
	if err != nil {
		return nil, err
	}
	c, err := cdp.DialWS(ctx, t.WebSocketDebuggerURL)
	if err != nil {
		return nil, err
	}
	c.URL = t.URL
	if err := c.Do(ctx, "Runtime.enable", nil, nil); err != nil {
		_ = c.Close() //nolint:errcheck // the enable failure is the error worth reporting
		return nil, fmt.Errorf("cdp: Runtime.enable: %w", err)
	}
	cl, err := Over(c)
	if err != nil {
		return nil, err
	}
	cl.browser = func() (*memreport.Browser, error) { return browserMem(c, t.ID) }
	return cl, nil
}
