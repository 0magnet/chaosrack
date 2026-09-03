package wtaudio

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/quic-go/quic-go"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// fakeConn stands in for a WebTransport session. limit is the largest
// datagram it will accept, and it rejects anything bigger the way quic-go
// does — with the size it WOULD accept, which is what the sender adapts
// to.
type fakeConn struct {
	sent  [][]byte
	limit int
	fail  error
}

func (f *fakeConn) SendDatagram(b []byte) error {
	if f.fail != nil {
		return f.fail
	}
	if f.limit > 0 && len(b) > f.limit {
		return &quic.DatagramTooLargeError{MaxDatagramPayloadSize: int64(f.limit)}
	}
	f.sent = append(f.sent, append([]byte(nil), b...))
	return nil
}

func samples(n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(math.Sin(float64(i) * 0.02))
	}
	return s
}

// End to end through the pieces a browser would run: fragment, "transmit"
// and reassemble with the receiver the browser uses.
func TestSenderChunksReassembleForTheBrowser(t *testing.T) {
	conn := &fakeConn{}
	s := NewSender(conn, 0)
	if s.MaxDatagramSize() != audiosrc.DefaultMaxDatagramSize {
		t.Fatalf("default datagram size is %d, want %d", s.MaxDatagramSize(), audiosrc.DefaultMaxDatagramSize)
	}

	in := samples(2400) // 0.1 s at 24 kHz, one PulseAudio chunk
	if err := s.Send(in); err != nil {
		t.Fatal(err)
	}
	if len(conn.sent) < 2 {
		t.Fatalf("a %d-byte chunk went out in %d datagrams; it cannot fit in one", len(in)*4, len(conn.sent))
	}

	var r audiosrc.Reassembler
	var out []byte
	for _, dg := range conn.sent {
		if p := r.Push(dg); p != nil {
			out = p
		}
	}
	got := audiosrc.BytesToFloat32(out)
	if len(got) != len(in) {
		t.Fatalf("reassembled %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if math.Float32bits(got[i]) != math.Float32bits(in[i]) {
			t.Fatalf("sample %d = %v, want %v", i, got[i], in[i])
		}
	}
}

// Consecutive chunks must carry increasing message IDs, or the receiver
// reads every one after the first as a straggler and the audio stops.
func TestSenderAdvancesTheMessageID(t *testing.T) {
	conn := &fakeConn{}
	s := NewSender(conn, 0)
	const chunks = 5
	for i := 0; i < chunks; i++ {
		if err := s.Send(samples(2400)); err != nil {
			t.Fatal(err)
		}
	}
	var r audiosrc.Reassembler
	delivered := 0
	for _, dg := range conn.sent {
		if p := r.Push(dg); p != nil {
			delivered++
		}
	}
	if delivered != chunks {
		t.Errorf("%d of %d chunks reassembled; the message ID is not advancing", delivered, chunks)
	}
	if r.Dropped != 0 {
		t.Errorf("a lossless fake link dropped %d messages", r.Dropped)
	}
}

// The path MTU is discovered, not known. When the stack refuses a
// datagram it says how big one may be; the sender must take that answer,
// keep it, and lose only the chunk it was in the middle of.
func TestSenderShrinksToWhatTheStackAllows(t *testing.T) {
	conn := &fakeConn{limit: 600}
	s := NewSender(conn, 0) // starts at 1200, twice what this link takes

	if err := s.Send(samples(2400)); err != nil {
		t.Fatalf("the first chunk after a shrink should still go out: %v", err)
	}
	if s.MaxDatagramSize() != 600 {
		t.Errorf("datagram size is %d after a rejection, want 600", s.MaxDatagramSize())
	}
	if s.Shrunk != 1 {
		t.Errorf("Shrunk = %d, want 1", s.Shrunk)
	}
	for _, dg := range conn.sent {
		if len(dg) > 600 {
			t.Fatalf("a %d-byte datagram went out over a 600-byte link", len(dg))
		}
	}

	// The discovery sticks: the next chunk must not have to be refused
	// again.
	before := s.Shrunk
	in := samples(2400)
	if err := s.Send(in); err != nil {
		t.Fatal(err)
	}
	if s.Shrunk != before {
		t.Errorf("Shrunk went from %d to %d; the smaller size did not stick", before, s.Shrunk)
	}
	// And the chunk sent at the smaller size still reassembles.
	var r audiosrc.Reassembler
	var out []byte
	for _, dg := range conn.sent {
		if p := r.Push(dg); p != nil {
			out = p
		}
	}
	if got := audiosrc.BytesToFloat32(out); len(got) != len(in) {
		t.Errorf("the chunk sent at the reduced size reassembled to %d samples, want %d", len(got), len(in))
	}
}

// A dead session must be reported so the capture unwinds, not retried
// forever.
func TestSenderReportsADeadSession(t *testing.T) {
	want := errors.New("session closed")
	s := NewSender(&fakeConn{fail: want}, 0)
	if err := s.Send(samples(64)); !errors.Is(err, want) {
		t.Errorf("Send returned %v, want %v", err, want)
	}
}

// A link so small that not even one sample fits has to be reported rather
// than silently sending nothing forever.
func TestSenderRefusesAnUnusableLink(t *testing.T) {
	s := NewSender(&fakeConn{}, audiosrc.DatagramHeaderSize+3)
	err := s.Send(samples(64))
	if err == nil {
		t.Fatal("a datagram size too small for one sample was accepted")
	}
	if !strings.Contains(err.Error(), "will not fit") {
		t.Errorf("error %q does not say what is wrong", err)
	}
}

func TestSenderIgnoresEmptyChunks(t *testing.T) {
	conn := &fakeConn{}
	s := NewSender(conn, 0)
	if err := s.Send(nil); err != nil {
		t.Fatal(err)
	}
	if len(conn.sent) != 0 {
		t.Errorf("an empty chunk produced %d datagrams", len(conn.sent))
	}
}
