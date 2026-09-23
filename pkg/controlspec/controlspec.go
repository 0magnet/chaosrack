// Package controlspec is what a control IS, as plain data.
//
// It exists because two things need it and neither should depend on the
// other: the rack, which records its controls as it wires them, and a front
// end, which renders them. When this type lived in the rack's own package a
// terminal panel could import the rack — but the rack could not then import
// the panel, which is exactly what putting the panel INSIDE the rack requires.
//
// So the description moved to the bottom, where a description belongs. The
// rack fills it in, pkg/racktui draws it, internal/rackcable carries it over
// a wire, and none of the three knows about the others.
package controlspec

// ControlInfo is a control stripped of everything that only means something
// inside the running rack: the closures, the element ids, the display
// mapping. What is left is what a description of the control surface would
// have to say, and it is all plain data — listable, printable, diffable,
// sendable, and drawable by a front end that has never heard of a DOM.
type ControlInfo struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Min       float64 `json:"min,omitempty"`
	Max       float64 `json:"max,omitempty"`
	Step      float64 `json:"step,omitempty"`
	Def       float64 `json:"def,omitempty"`
	IsSelect  bool    `json:"select,omitempty"`
	SelectDef string  `json:"selectDef,omitempty"`
	PermaKey  string  `json:"perma,omitempty"`
	// Module is the panel the control is mounted in — its header text, which
	// is what the rack names a module by. A front end that draws the RACK
	// rather than a list of settings needs it: a knob belongs to a panel, and
	// a panel belongs to a bay.
	Module    string `json:"module,omitempty"`
	ModTarget bool   `json:"mod,omitempty"`
}
