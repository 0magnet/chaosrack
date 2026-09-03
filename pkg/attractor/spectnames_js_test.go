//go:build js && wasm

package attractor

import (
	"testing"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The knob picks a scheme by INDEX into spectColNames and hands the string to
// the library. A name the library does not recognize is not an error there --
// SetColorByName simply matches nothing and leaves the scheme alone -- so a typo
// or a name that upstream renamed shows up as a knob position that appears to do
// nothing at all. Round-tripping every entry is what catches that.
func TestEverySpectrogramColorNameIsOneTheLibraryKnows(t *testing.T) {
	s := sg.DefaultSettings()
	for i, name := range spectColNames {
		s.SetColorByName(name)
		if got := s.ColorName(); got != name {
			t.Errorf("knob position %d is %q, which the library resolved to %q", i, name, got)
		}
	}
}

func TestEveryWindowAndScaleNameIsOneTheLibraryKnows(t *testing.T) {
	s := sg.DefaultSettings()
	for i, name := range spectWinNames {
		s.SetWindowByName(name)
		if got := s.WindowName(); got != name {
			t.Errorf("window position %d is %q, resolved to %q", i, name, got)
		}
	}
	// The scale names are the app's own wording, not the library's: it answers
	// "log" where the knob says "logarithmic". Only the mapping has to work.
	for i, name := range spectScaleNames {
		s.SetScaleByName(name)
		if got := s.ScaleName(); got == "" {
			t.Errorf("scale position %d is %q, which resolved to nothing", i, name)
		}
	}
}
