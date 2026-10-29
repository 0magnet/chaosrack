package attractor

import (
	"fmt"
	"math"
	"strings"
)

// The color key: what the colors on the trace MEAN, said beside them.
//
// A color source is a measurement and the map its scale, and nothing on the
// screen said which, or on what scale. That matters least for the fixed
// sources, whose ends always mean one thing, and most for the auto-ranged
// ones, whose ends move with the music: the same orange is 700 Hz one minute
// and 2 kHz the next. So the key names the source and labels the bottom,
// middle and top of the ramp with what they stand for NOW — a fixed scale's
// marks, or the live span an auto-ranged source is being mapped across.

// colorKey is what the key says: the source, and the ramp's bottom, middle
// and top. An empty label is an end with nothing worth saying.
type colorKey struct{ Title, Lo, Mid, Hi string }

// colorKeyFor is the key for source src on model (axes is the Stereo
// Embedding's AXES position), reading the live range from a. False when
// there is nothing to key: the source is OFF.
func colorKeyFor(src int, model string, axes int, a *AudioColor) (colorKey, bool) {
	switch src {
	case GradientSourceOff:
		return colorKey{}, false
	case 0, 1, 2:
		name := axisName(model, axes, src)
		if name == "time" {
			return colorKey{Title: "Z · time", Lo: "older", Hi: "newer"}, true
		}
		return colorKey{Title: name, Lo: "low", Hi: "high"}, true
	case 3:
		return colorKey{Title: "trail age", Lo: "older", Hi: "newer"}, true
	}
	if gradientSourceNeedsStereo(src) && model != "stereo" {
		return colorKey{Title: sourceTitle(src), Mid: "needs two channels"}, true
	}
	k := colorKey{Title: sourceTitle(src)}
	switch src {
	case gradientSourceCorr:
		k.Lo, k.Mid, k.Hi = "−1 inverted", "0", "+1 same"
	case gradientSourceBalance, gradientSourcePosition:
		k.Lo, k.Mid, k.Hi = "R", "center", "L"
	case gradientSourceDB:
		k.Lo, k.Mid, k.Hi = minus(fmt.Sprintf("%.0f dB", dbFloor)), minus(fmt.Sprintf("%.0f", dbFloor/2)), "0 dB"
	case gradientSourcePitch:
		k.Lo, k.Mid, k.Hi = "A", "D♯", "A"
	case gradientSourceAudio:
		k.Lo, k.Hi = "darker", "brighter"
		if a.ranged && a.sr > 0 {
			nyq := float32(a.sr) / 2
			k.Lo, k.Hi = fmtHz(a.rlo*nyq), fmtHz(a.rhi*nyq)
		}
	case gradientSourceLevel, gradientSourceSide:
		k.Lo, k.Hi = "quieter", "louder"
		if src == gradientSourceSide {
			k.Lo, k.Hi = "narrower", "wider"
		}
		if a.ranged {
			k.Lo, k.Hi = fmtDBFS(a.rlo), fmtDBFS(a.rhi)
		}
	case gradientSourceFlux:
		k.Lo, k.Hi = "steady", "changing"
	}
	return k, true
}

// sourceTitle is what an audio source measures, in a word or two.
func sourceTitle(src int) string {
	switch src {
	case gradientSourceAudio:
		return "spectral centroid"
	case gradientSourceLevel:
		return "level"
	case gradientSourceDB:
		return "level"
	case gradientSourceCorr:
		return "L/R correlation"
	case gradientSourceSide:
		return "side level"
	case gradientSourceBalance:
		return "balance"
	case gradientSourcePosition:
		return "stereo position"
	case gradientSourceFlux:
		return "spectral flux"
	case gradientSourcePitch:
		return "pitch class"
	}
	return "color"
}

// axisName is what coordinate axis (0 X, 1 Y, 2 Z) is in model: a delayed
// copy of the signal in the embeddings, time on the Stereo Embedding's
// goniometer positions, and just the coordinate elsewhere.
func axisName(model string, axes, axis int) string {
	letter := [3]string{"X", "Y", "Z"}[axis]
	var names [3]string
	switch model {
	case "takens", "polar":
		names = [3]string{"s(t)", "s(t−τ)", "s(t−2τ)"}
	case "stereo":
		names = [4][3]string{
			{"L", "R", "L(t−τ)"},
			{"L", "R", "time"},
			{"M", "S", "M(t−τ)"},
			{"M", "S", "time"},
		}[min(max(axes, 0), 3)]
	default:
		return letter
	}
	if names[axis] == "time" {
		return "time"
	}
	return letter + " · " + names[axis]
}

// fmtHz is a frequency as a key labels it.
func fmtHz(f float32) string {
	if f >= 1000 {
		return fmt.Sprintf("%.1f kHz", f/1000)
	}
	return fmt.Sprintf("%.0f Hz", f)
}

// fmtDBFS is an RMS of full scale, 0..1, in decibels.
func fmtDBFS(v float32) string {
	return minus(fmt.Sprintf("%.0f dB", 20*math.Log10(math.Max(float64(v), 1e-6))))
}

// minus sets a number's sign as the key's other labels do: −, not a hyphen.
func minus(s string) string { return strings.ReplaceAll(s, "-", "−") }
