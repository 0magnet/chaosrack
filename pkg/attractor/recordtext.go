package attractor

import (
	"strconv"
	"strings"
)

// What the Record module's displays say, shared by the page
// (recordmodule_js.go) and a native rack's recorder.

// RecTimecode is secs as the record counter shows it: hh:mm:ss.
func RecTimecode(secs int) string {
	secs = max(secs, 0)
	return pad2(secs/3600) + ":" + pad2((secs/60)%60) + ":" + pad2(secs%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// RecLogLine is the log line a finished take leaves: what it was written as,
// how long it ran, and how big it came out — for a GIF, encoded as the take
// ends, the only moment its size exists.
func RecLogLine(ext string, secs, bytes int) string {
	return strings.ToUpper(ext) + "  " + RecTimecode(secs) + "  " + humanBytes(bytes)
}

// humanBytes is a size a person reads, not a byte count.
func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return strconv.Itoa(n/(1<<20)) + "." + strconv.Itoa((n%(1<<20))*10/(1<<20)) + " MB"
	case n >= 1<<10:
		return strconv.Itoa(n/(1<<10)) + " KB"
	default:
		return strconv.Itoa(n) + " B"
	}
}
