package attractor

// The Stereo Embedding's dials as the page shows them: the name of each
// position and what fits around it, and the WIN knob's top.

// stereoAxisNames are the dial's positions by name — the tooltip on each
// detent. paramLabels turns a numeric knob into a labeled rotary switch given
// these; paramdefs_js.go is where both tables are indexed.
// The two TIME positions are goniometers in the instrument sense — a two
// channel vector display — with the third axis sweeping like a scope
// timebase. The two DELAY positions are not: a delayed copy on the third
// axis makes them a delay embedding of the pair, which is a different
// figure answering a different question, so they are not called one.
var stereoAxisNames = []string{
	doc("p.stereo-axes=0"),
	doc("p.stereo-axes=1"),
	doc("p.stereo-axes=2"),
	doc("p.stereo-axes=3"),
}

// stereoAxisRing is what fits AROUND the dial: five runes per position, and for
// a named setting the ring IS the readout — the LED is hidden deliberately,
// because seven segments cannot spell a word — so these have to be told apart
// at a glance and not merely be short. Basis in caps, since L, R, M and S are
// what the channels are called everywhere else here; third axis lower case.
//
// It said "LRτ"/"LRt"/"MSτ"/"MSt" first, which is the notation the rest of this
// file uses, and it was UNREADABLE: captured off the real panel, τ and t at ring
// size are the same few pixels, so the dial showed what looked like two "LRt"
// and two "MSt" and there was no way to tell which of a pair was selected.
// "d" for delay is not the notation, and it is legible, which beats it.
// "g" for goniometer on the two TIME positions, "d" for delay on the other
// two. It said "t" for time, which described the axis correctly and said
// nothing about what the figure IS — and the pair a reader wants to tell
// apart at a glance is goniometer against embedding, not time against delay.
var stereoAxisRing = []string{"LRd", "LRg", "MSd", "MSg"}

// stereoWinMax is the WIN knob's top, in milliseconds, and it is lower than
// the Takens mode's 500 for two unrelated reasons that happen to agree.
//
// The hard one: this mode SNAPSHOTS its window (audiosrc.DefaultRingSize and
// the comment on it), and a snapshot longer than the source's ring comes back
// silently wrapped. 250 ms is 12000 samples at 48 kHz, which leaves room for
// the largest τ under the 16384-sample ring. stereoWindow clamps as well, so
// an unusual sample rate cannot walk past the bound — the knob's max is what
// keeps the clamp from ever being the thing the user is fighting.
//
// The soft one: a goniometer is a PHASE display, and phase displays are read
// over a few tens of milliseconds. Past a couple of hundred the figure is a
// filled blob whatever the source is doing, so the range the knob does not
// cover is range nobody would turn it to. The Takens mode wants the long
// window because a reconstructed manifold fills in as it accumulates; a
// Lissajous just gets darker.
const stereoWinMax = 250

// stereoTrigNames are the dial's positions.
var stereoTrigNames = []string{
	doc("p.stereo-trig=0"),
	doc("p.stereo-trig=1"),
	doc("p.stereo-trig=2"),
	doc("p.stereo-trig=3"),
}

// stereoTrigRing is what fits around the dial.
var stereoTrigRing = []string{"off", "rise", "fall", "lock"}

var trigSrcNames = []string{
	doc("p.stereo-tsrc=0"),
	doc("p.stereo-tsrc=1"),
	doc("p.stereo-tsrc=2"),
}

var trigSrcRing = []string{"mid", "L", "R"}

var trigCplNames = []string{
	doc("p.stereo-tcpl=0"),
	doc("p.stereo-tcpl=1"),
	doc("p.stereo-tcpl=2"),
}

var trigCplRing = []string{"DC", "LFr", "HFr"}

var trigRunNames = []string{
	doc("p.stereo-trun=0"),
	doc("p.stereo-trun=1"),
	doc("p.stereo-trun=2"),
}

var trigRunRing = []string{"auto", "norm", "sgl"}

var gratNames = []string{
	doc("p.stereo-grat=0"),
	doc("p.stereo-grat=1"),
	doc("p.stereo-grat=2"),
}

var gratRing = []string{"off", "ax", "full"}
