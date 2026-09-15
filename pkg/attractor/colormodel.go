package attractor

// The colour model, in the one form both sides of the build tag need.
//
// Every coloured thing in the rack makes its colour the same way: a VALUE, a
// MAPPING of that value to a colour, and a window onto the mapping. The panel
// says so with two knobs — SRC and MAP — and this is the one number out of that
// model that the native tools need too, so it lives in an untagged file rather
// than behind //go:build js.

// GradientSourceOff is the SRC ring's OFF position: the colour follows nothing,
// and the trace is flat in the start swatch.
//
// It is what used to be called the mono palette, moved to the knob it belongs
// to. "One colour, value ignored" is a statement about the SOURCE — there is no
// value — and not about how a value becomes a colour, which is the only
// question the map ring answers. On the map ring it was the one position that
// contradicted the knob's own stated function, and the reason the src ring had
// to be dimmed by a rule written specially for it.
//
// 5 rather than 0 so that X / Y / Z / trail / audio keep the option values a
// permalink already records. The ring binds a label to an option by INDEX and a
// permalink binds by VALUE, and those are free to disagree — which is what lets
// OFF be drawn first on the dial while numbering last in the markup.
//
// Exported because `uitool` sets this knob from outside the package: it shoots
// the colour-matrix contact sheets, and the flat row is now a source setting
// rather than a palette one. A tool asking for a value the select no longer has
// is answered by the browser leaving the select alone, so the knob would
// silently never move and the sheet would come out wrong with nothing to see.
const GradientSourceOff = 5
