//go:build js

package subcmds

import (
	"errors"
	"image"

	"github.com/spf13/cobra"
)

// In the page there is no ffmpeg to write a video with and no rack to draw
// one off the clock: render draws stills, GIFs and SVGs there.

var renderAudioFile string

func videoFlags(*cobra.Command) {}

func isVideo(string) bool { return false }

func useRack() (bool, error) { return false, nil }

var errNoVideo = errors.New("a video is written by chaosrack on the host, not in the page")

func decodeAudio(string) (l, r []float32, err error) { return nil, nil, errNoVideo }

func rgbaVideo(string, int, func(int) *image.RGBA, []float32, []float32) error { return errNoVideo }

func renderRack(*cobra.Command) error { return errNoVideo }

func listRackParams() error { return nil }
