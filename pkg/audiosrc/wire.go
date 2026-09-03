package audiosrc

import "github.com/0magnet/audioprism-go/pkg/wsaudio"

// The audio wire format, and the datagram framing that carries it over
// WebTransport, are DEFINED UPSTREAM in audioprism-go and re-exported here.
//
// They were defined in both places for a while, because the import can only
// run one way: chaosrack already depends on audioprism-go for the spectrogram,
// so audioprism-go importing chaosrack would be a cycle. Duplication was the
// only way to have the framing in both, and it was guarded by a test in each
// repo that pinned the literal header bytes — a round trip is no guard at all,
// since it passes perfectly against a framing the other repo disagrees with.
//
// Guarding is weaker than not being able to differ. Both projects stream the
// same samples and a client of either can talk to a server of the other, so
// the format is one thing and there should be one definition of it. It lives
// where the dependency already points.
//
// wsaudio was the right package to put it in and not merely the convenient
// one: it has no dependencies outside the standard library, which matters
// because this is compiled into a wasm binary. That is also why audioprism-go
// keeps its WebSocket codec in a separate package — pulling x/net/websocket in
// here would drag net/http and crypto/tls along with it, measured at 3.1 MB to
// 5.2 MB on their browser build.
//
// The aliases exist so the call sites read the same as they did. There is no
// second implementation behind them.

// Float32ToBytes encodes samples as little-endian float32.
var Float32ToBytes = wsaudio.Float32ToBytes

// BytesToFloat32 decodes what Float32ToBytes produced.
var BytesToFloat32 = wsaudio.BytesToFloat32

// SplitDatagrams fragments one audio chunk into datagrams.
var SplitDatagrams = wsaudio.SplitDatagrams

// DatagramPayloadSize is how many audio bytes fit in one datagram.
var DatagramPayloadSize = wsaudio.DatagramPayloadSize

// SelectTransport decides between the WebSocket and WebTransport.
var SelectTransport = wsaudio.SelectTransport

const (
	// DatagramHeaderSize is the per-datagram overhead: a little-endian
	// uint16 message id, a fragment index and a fragment count.
	DatagramHeaderSize = wsaudio.DatagramHeaderSize

	// DefaultMaxDatagramSize is the datagram size assumed before the QUIC
	// stack says otherwise.
	DefaultMaxDatagramSize = wsaudio.DefaultMaxDatagramSize
)

// Reassembler puts the fragments of one audio chunk back together.
type Reassembler = wsaudio.Reassembler

// TransportKind is which transport a source ended up on.
type TransportKind = wsaudio.TransportKind

// WTProbe is what is known about WebTransport's availability so far.
type WTProbe = wsaudio.WTProbe

// The transport kinds, re-exported so a caller need not import wsaudio to
// name one.
const (
	TransportWebSocket    = wsaudio.TransportWebSocket
	TransportWebTransport = wsaudio.TransportWebTransport
)
