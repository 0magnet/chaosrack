//go:build !js

package rackcable

import "github.com/0magnet/cdp"

// Dial finds the rack on a browser's remote-debugging port.
func Dial(port int, target string) (*Client, error) {
	c, err := cdp.Dial(port, target)
	if err != nil {
		return nil, err
	}
	return Over(c)
}
