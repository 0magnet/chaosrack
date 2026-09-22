// Package metersproto is what the panel and the analyzers' worker say to each
// other. It exists because they used to say it in two places.
//
// The masks below were declared once on each side of the boundary, and the
// panel's copy shared a const block with a string — so iota started at one and
// every mask was double the worker's. The panel asked for loudness and the
// worker heard distortion. Nothing errored: the wrong analyzers simply ran,
// and the only symptom was one readout staying on dashes.
//
// A wire protocol with two definitions has a bug in it that the compiler
// cannot see. This package is the one definition.
package metersproto

import "github.com/0magnet/chaosrack/pkg/meters"

// Which analyzers the panel wants running. A module scrolled out of view is
// not asked for, so the worker does nothing for it either.
const (
	WantLufs = 1 << iota
	WantThd
	WantWf
)

// Message types.
const (
	TypeReady  = "ready" // worker → panel, once, when its Go side is up
	TypeConfig = "cfg"   // panel → worker, when a switch moves
	TypeAudio  = "audio" // panel → worker, per block
	TypeResult = "res"   // worker → panel, when an analysis has a new answer
)

// Config is what the panel's switches say. Sent when one moves, so the worker
// never asks and never polls.
type Config struct {
	SampleRate int     `json:"sr"`
	ThdPeriod  float64 `json:"tp"`
	ThdHarm    int     `json:"th"`
	WfPeriod   float64 `json:"wp"`
	WfWindow   int     `json:"ww"`
	WfNominal  float64 `json:"wn"`
	LufsPeriod float64 `json:"lp"`
	Want       uint8   `json:"w"`
}

// Result carries whichever analyses have a new answer, so a quiet second
// costs one small message rather than three.
type Result struct {
	Lufs *meters.LoudnessResult   `json:"l,omitempty"`
	Thd  *meters.DistortionResult `json:"d,omitempty"`
	Wf   *meters.WowFlutterResult `json:"w,omitempty"`
}

// Envelope is the shape every message on the wire has.
type Envelope struct {
	T string  `json:"t"`
	V *Result `json:"v,omitempty"`
}
