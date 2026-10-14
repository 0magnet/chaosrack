//go:build js

package rackhost

import "errors"

// hostCapture has no machine to capture in a browser: the page's own audio
// source is the page's.
func hostCapture(string, func([]float32) error) (func(), error) {
	return nil, errors.New("no capture in a browser")
}
