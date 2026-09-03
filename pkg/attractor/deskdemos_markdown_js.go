//go:build js && wasm && tuimarkdown

package attractor

// The markdown demo, behind a tag, because it costs more than the rest of the
// collection put together and it is the only one that cannot be built at all by
// the toolchain this program also ships with.
//
// SIZE. It renders markdown through glamour, which brings goldmark and
// bluemonday with it. Measured against the wasm this program builds to:
//
//	baseline                       16.3 MB raw   3.90 MB gzipped
//	+ every demo but this one      20.3 MB raw   4.88 MB gzipped
//	+ this one as well             33.5 MB raw   7.38 MB gzipped
//
// So one demo is thirteen of the seventeen megabytes, and two and a half of the
// three and a half a visitor actually downloads. The twenty-one animations that
// were the point of taking tuiwasm at all cost less than one.
//
// TINYGO. It also breaks that build outright:
//
//	# github.com/microcosm-cc/bluemonday/css
//	regexp/syntax/parse.go:293: interp: running for more than 3m0s, timing out
//
// bluemonday compiles regexps at package level, TinyGo's compile-time
// interpreter tries to run them, and the default three minutes is not enough --
// tuiwasm's own build passes -interp-timeout 10m for exactly this. That would
// work here too, at half an hour of build time and 1.6GB of memory, but this
// program ships a TinyGo build as a first-class artifact and paying that for one
// demo is the wrong trade.
//
// So: `-tags tuimarkdown` to have it, with -interp-timeout raised for TinyGo.
import _ "github.com/0magnet/tuiwasm/demos/markdown"
