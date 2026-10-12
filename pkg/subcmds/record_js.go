//go:build js

package subcmds

import (
	"errors"

	"github.com/spf13/cobra"
)

// What --audio does here, and how an audio model is given a signal: the page
// has only the test signals.
const (
	audioUsage = "for an audio model, generate this many seconds of the --signal (default 2)"
	audioHint  = "--signal NAME draws it from a test signal"
)

// recordFlags adds nothing: the page has no machine audio to record.
func recordFlags(*cobra.Command) {}

// record is not possible in the page; a test signal stands in.
func record(float64) (l, r []float32, err error) {
	return nil, nil, errors.New("there is no audio to record here: use --signal NAME")
}
