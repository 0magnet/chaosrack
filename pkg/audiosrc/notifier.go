package audiosrc

// Notifier is an optional extension a Source may implement to report
// something worth saying that is not an error: notably "the transport you
// asked for was not available, so this is the other one". Without it a
// successful fallback would be invisible — the Source is Ready and Err is
// nil, so the status overlay, which only ever speaks up about failures,
// would say nothing at all and the page would look like it was doing what
// was asked.
//
// This stays here rather than moving upstream with the wire format: it is
// about how THIS app surfaces a status to a user, not about what goes on
// the wire. audioprism-go reports its own fallback through its own console
// and overlay.
type Notifier interface {
	// Notice returns a one-line status, or "" when there is nothing to
	// say. It is polled every frame, so it must be cheap and stable:
	// the caller suppresses repeats by comparing the string.
	Notice() string
}
