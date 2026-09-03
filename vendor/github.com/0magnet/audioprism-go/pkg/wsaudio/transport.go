package wsaudio

import "strings"

// Which transport carries the audio, and when the WebTransport one gives
// up and hands the job back to the WebSocket one.
//
// This lives in a file with no build tag so the decision can be tested
// natively. The browser side (cmd/a/wasm/wasm/wt.go) is deliberately thin
// — it probes the environment, fills in a WTProbe and does what this says
// — because nothing in a browser can be unit tested here.
//
// A copy of chaosrack's pkg/audiosrc/transport.go, for the reason given at
// the top of datagram.go: the dependency between the repos already runs
// the other way, so the framing and the decision it feeds cannot be
// imported from there.

// TransportKind names the transport actually used, which is not always
// the one that was asked for.
type TransportKind string

const (
	// TransportWebSocket is the default and the fallback: TCP, ordered,
	// reliable, supported everywhere, and entirely adequate over
	// loopback. No query parameter selects it because it is what you get
	// when nothing else works out.
	TransportWebSocket TransportKind = "ws"

	// TransportWebTransport is the opt-in path (?audio=wt): HTTP/3 over
	// QUIC, carrying audio in unreliable datagrams so a lost packet
	// costs one chunk instead of stalling the feed.
	TransportWebTransport TransportKind = "wt"
)

// WTProbe is what the browser managed to find out about WebTransport, in
// the order it finds it out. A zero value means "not supported", which is
// the right answer for Safari and for anything older than Chrome 97.
type WTProbe struct {
	// Supported is whether the WebTransport constructor exists at all.
	Supported bool

	// InfoErr is a failure to fetch /wt-info from the page's own origin.
	// That endpoint carries the certificate hash, so without it the
	// connection cannot even be attempted — most likely the server was
	// started with -wt=false.
	InfoErr error

	// DialErr is a rejected WebTransport ready promise: the QUIC
	// handshake failed, the certificate hash was refused, UDP is blocked
	// on the path, or the session was closed under us mid-stream.
	DialErr error
}

// SelectTransport decides which transport to use from the ?audio= value
// and what the browser has told us so far, and returns a reason string to
// show the user when the answer is not what they asked for.
//
// It is called at each stage of the WebTransport bring-up with a probe
// that has learned one more thing, so a single function covers all four
// ways the optional path can be declined: unsupported browser, no
// /wt-info, failed handshake, and a session that dropped after working.
// An empty reason means nothing surprising happened.
func SelectTransport(param string, p WTProbe) (TransportKind, string) {
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "wt", "webtransport":
	default:
		// Not asked for. Anything else — "ws", "", a typo — is not this
		// function's business; the caller maps it.
		return TransportWebSocket, ""
	}
	switch {
	case !p.Supported:
		return TransportWebSocket, "WebTransport is not supported by this browser — using WebSocket"
	case p.InfoErr != nil:
		return TransportWebSocket, "WebTransport unavailable (" + p.InfoErr.Error() + ") — using WebSocket"
	case p.DialErr != nil:
		return TransportWebSocket, "WebTransport failed (" + p.DialErr.Error() + ") — using WebSocket"
	}
	return TransportWebTransport, ""
}
