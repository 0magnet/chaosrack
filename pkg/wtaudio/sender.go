package wtaudio

import (
	"errors"
	"fmt"

	"github.com/quic-go/quic-go"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// DatagramSender is the part of a WebTransport session this package
// writes to. Narrowed to one method so the fragmentation and the
// size-adaptation below can be tested without a QUIC connection, a
// certificate or a browser — none of which a unit test can produce.
type DatagramSender interface {
	SendDatagram([]byte) error
}

// Sender fragments audio chunks into datagrams for one session.
//
// One per session, and not safe for concurrent use: the message ID has to
// increase monotonically for the receiver's reassembler to tell a new
// chunk from a straggler, so two goroutines interleaving here would look
// exactly like a lossy link.
type Sender struct {
	// Shrunk counts the times the QUIC stack refused a datagram as too
	// large and the size was lowered. Expected to be 0 or 1 for the life
	// of a session — it is a discovery, not a recurring cost.
	Shrunk int

	conn DatagramSender
	max  int
	id   uint16
}

// NewSender starts at maxSize bytes per datagram, lowering it if the
// stack says so. Pass 0 for audiosrc.DefaultMaxDatagramSize.
func NewSender(conn DatagramSender, maxSize int) *Sender {
	if maxSize <= 0 {
		maxSize = audiosrc.DefaultMaxDatagramSize
	}
	return &Sender{conn: conn, max: maxSize}
}

// MaxDatagramSize is the size currently in use, after any shrinking.
func (s *Sender) MaxDatagramSize() int { return s.max }

// Send fragments one chunk of samples and sends every piece.
//
// The samples are encoded by audiosrc.Float32ToBytes — the same function
// the WebSocket handler calls — so both transports put identical bytes on
// the wire and the browser has one decoder for both.
//
// If the stack rejects a datagram as too large it says how large it may
// be; the size is lowered and the WHOLE chunk is resent under a new
// message ID. The fragments already sent are then an incomplete message
// that the receiver abandons, costing the ~100 ms of audio in this one
// chunk. That is the correct trade for a path that exists to drop rather
// than to stall, and it happens at most once per session because the
// answer sticks.
func (s *Sender) Send(samples []float32) error {
	if len(samples) == 0 {
		return nil
	}
	payload := audiosrc.Float32ToBytes(samples)

	// Two attempts: the original size, then whatever the stack said. A
	// third would mean the stack changed its answer, which is a bug
	// worth surfacing rather than looping over.
	for attempt := 0; attempt < 2; attempt++ {
		dgs := audiosrc.SplitDatagrams(s.id, payload, s.max)
		s.id++
		if dgs == nil {
			return fmt.Errorf("wtaudio: %d bytes of audio will not fit in %d-byte datagrams", len(payload), s.max)
		}
		var err error
		for _, dg := range dgs {
			if err = s.conn.SendDatagram(dg); err != nil {
				break
			}
		}
		if err == nil {
			return nil
		}
		var tooLarge *quic.DatagramTooLargeError
		if !errors.As(err, &tooLarge) {
			return err // the session is going away; the caller unwinds the capture
		}
		if int(tooLarge.MaxDatagramPayloadSize) >= s.max {
			// Refused for being too large, at a size we are already
			// under. Retrying cannot help.
			return err
		}
		s.max = int(tooLarge.MaxDatagramPayloadSize)
		s.Shrunk++
	}
	return nil
}
