package wsaudio

import "encoding/binary"

// Datagram framing for the optional WebTransport audio path.
//
// WHY THIS IS A COPY. The identical framing lives in chaosrack's
// pkg/audiosrc/datagram.go, and the two must stay byte-for-byte the same:
// both projects put the SAME payload on the wire (little-endian float32,
// no header — the format this package defines), so a chaosrack browser
// build can point at an audioprism-go server and get audio. Two different
// datagram framings would end that quietly, with a page that connects and
// then shows nothing.
//
// It is a copy rather than an import because chaosrack already imports
// audioprism-go (pkg/spectrogram), so importing back would be a cycle. The
// long-term fix is the other direction: chaosrack should import THIS
// package, since the dependency already runs that way. Until then the
// literal-byte test in datagram_test.go is what stops the two drifting —
// a round-trip test would pass happily against a framing the other repo
// disagrees with.
//
// WHY DATAGRAMS AND NOT A STREAM. WebTransport offers both, and a stream
// would need no framing at all — the bytes would arrive in order, exactly
// as they do over the WebSocket. The reason to take on this framing is
// what happens when a packet is lost. A stream (WebSocket over TCP, or a
// WebTransport stream over QUIC) cannot deliver byte N+1 until byte N has
// been retransmitted, so one lost packet stalls the whole audio feed for
// at least a round trip; the spectrogram freezes and then fast-forwards
// through the backlog. A datagram is not retransmitted, so a loss costs
// exactly the audio it carried and nothing after it — at 24 kHz mono
// float32 a full 1200-byte datagram is 1196/4 = 299 samples, about 12 ms.
// A spectrogram would far rather lose 12 ms than stutter.
//
// None of that matters over loopback, which is what the WebSocket path
// serves: ~96 KB/s of samples and a TCP connection that never drops a
// packet locally. WebTransport buys nothing there and is offered as an
// option, not a replacement. The payoff is a remote or lossy link.
//
// WHY THERE IS A HEADER AT ALL. Datagrams may be reordered and dropped,
// and a PulseAudio chunk (0.1 s of latency at 24 kHz is 9600 bytes) does
// not fit in one. So a chunk is split, and the pieces carry just enough
// to be put back together: which chunk, which piece, how many pieces.
// Four bytes, and the payload itself stays the same little-endian float32
// this package already defines — both transports carry identical bytes so
// BytesToFloat32 is the only decoder, on the server and in the browser.

const (
	// DatagramHeaderSize is the per-datagram overhead: a uint16 message
	// ID (little-endian, wrapping), a fragment index and a fragment
	// count. Deliberately tiny — at 1200 bytes per datagram it is 0.3%.
	DatagramHeaderSize = 4

	// DefaultMaxDatagramSize is the datagram size assumed before the
	// QUIC stack says otherwise. QUIC datagrams cannot be fragmented at
	// the IP layer, so the limit is the path MTU minus QUIC and
	// WebTransport overhead; ~1200 bytes is the conservative figure that
	// survives the internet's smallest common MTU. The server lowers it
	// on the spot if the stack rejects a datagram as too large
	// (wtaudio.Sender), because guessing high once is cheaper than
	// guessing low forever.
	DefaultMaxDatagramSize = 1200

	// maxFragments is what a uint8 fragment count holds. At the default
	// size that is 305 KB per message — three seconds of this audio, so
	// nowhere near a real chunk. A message that would need more is
	// refused rather than silently truncated.
	maxFragments = 255
)

// DatagramPayloadSize is how many audio bytes fit in one datagram of
// maxSize bytes.
//
// It is rounded DOWN to a multiple of four so a fragment boundary never
// falls inside a float32. That matters because of what the receiver does
// with a partial message: the samples that did arrive are whole samples,
// and a future decision to play the surviving fragments instead of
// dropping the message needs no re-alignment. It also costs at most three
// bytes per datagram.
func DatagramPayloadSize(maxSize int) int {
	usable := maxSize - DatagramHeaderSize
	if usable < 4 {
		return 0
	}
	return usable &^ 3
}

// SplitDatagrams fragments one audio chunk into datagrams, all but the
// last exactly DatagramPayloadSize(maxSize) bytes of payload.
//
// Returns nil for an empty payload, for a maxSize too small to carry even
// one sample, and for a payload needing more than 255 fragments — in the
// last case the caller is sending chunks far larger than this path is
// designed for and should chunk them itself rather than have the wire
// format quietly lose the tail.
func SplitDatagrams(msgID uint16, payload []byte, maxSize int) [][]byte {
	per := DatagramPayloadSize(maxSize)
	if per == 0 || len(payload) == 0 {
		return nil
	}
	count := (len(payload) + per - 1) / per
	if count > maxFragments {
		return nil
	}
	out := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		end := (i + 1) * per
		if end > len(payload) {
			end = len(payload)
		}
		part := payload[i*per : end]
		dg := make([]byte, DatagramHeaderSize+len(part))
		binary.LittleEndian.PutUint16(dg[0:2], msgID)
		dg[2] = uint8(i)     //nolint:gosec // count is <= maxFragments (255), checked above
		dg[3] = uint8(count) //nolint:gosec // same
		copy(dg[DatagramHeaderSize:], part)
		out = append(out, dg)
	}
	return out
}

// Reassembler puts the fragments of one audio chunk back together.
//
// It tracks exactly one message at a time. When a fragment of a newer
// message arrives while an older one is still incomplete, the older one
// is abandoned — that is the whole point of the unreliable path, and the
// alternative (holding it open in case the straggler shows up) is the
// stall this transport exists to avoid. Dropped counts those abandonments
// so a caller can tell "the link is lossy" from "the link is down".
//
// Not safe for concurrent use. In the browser it is only ever touched by
// the single JS thread, and on the server one lives per session.
type Reassembler struct {
	// Dropped is the number of messages abandoned incomplete. Each one
	// is roughly one PulseAudio chunk of audio — tens of milliseconds.
	Dropped int

	seen    bool
	id      uint16
	frags   [][]byte
	missing int
}

// Push feeds one received datagram in and returns the completed payload
// when this datagram was the last piece missing, otherwise nil.
//
// The datagram's bytes are copied, so the caller may reuse its buffer —
// the browser side reads every datagram into one scratch slice.
func (r *Reassembler) Push(dg []byte) []byte {
	if len(dg) <= DatagramHeaderSize {
		return nil // header-only or truncated: nothing to reassemble
	}
	id := binary.LittleEndian.Uint16(dg[0:2])
	idx, count := int(dg[2]), int(dg[3])
	if count == 0 || idx >= count {
		return nil // not our framing, or corrupted past the point of use
	}
	payload := dg[DatagramHeaderSize:]

	switch {
	case !r.seen || newerMsgID(id, r.id):
		if r.missing > 0 {
			r.Dropped++
		}
		r.seen = true
		r.id = id
		r.frags = make([][]byte, count)
		r.missing = count
	case id != r.id:
		// A straggler from a message we already completed or gave up
		// on. Restarting it would deliver audio out of order behind
		// what has already been played, so it is discarded.
		return nil
	case r.missing == 0:
		return nil // duplicate arriving after this message completed
	case count != len(r.frags):
		return nil // same ID, different fragment count: not the same message
	}

	if r.frags[idx] != nil {
		return nil // duplicate fragment; QUIC does not promise not to
	}
	r.frags[idx] = append([]byte(nil), payload...)
	r.missing--
	if r.missing > 0 {
		return nil
	}

	n := 0
	for _, f := range r.frags {
		n += len(f)
	}
	out := make([]byte, 0, n)
	for _, f := range r.frags {
		out = append(out, f...)
	}
	return out
}

// Reset forgets any partially received message, for a caller that has
// just reconnected and knows the fragments in flight are worthless.
func (r *Reassembler) Reset() {
	r.seen = false
	r.frags = nil
	r.missing = 0
}

// newerMsgID compares two wrapping 16-bit message IDs. The subtraction is
// done in the unsigned domain and read as signed, which is the standard
// sequence-number trick: it stays correct across the wrap at 65535 as
// long as the two IDs are less than 32768 apart, and at ~10 messages a
// second they are seconds apart at most.
func newerMsgID(a, b uint16) bool { return int16(a-b) > 0 } //nolint:gosec // the wrap is the point
