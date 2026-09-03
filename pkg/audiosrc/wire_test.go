package audiosrc

import (
	"bytes"
	"testing"

	"github.com/0magnet/audioprism-go/pkg/wsaudio"
)

// The format's own behavior is tested where it is defined, in
// audioprism-go/pkg/wsaudio — including the literal-header-bytes test that
// exists to stop the two projects drifting. Duplicating those cases here
// would test the same functions twice through an alias.
//
// What is worth asserting on THIS side is that the alias still points at the
// upstream: a local re-definition that shadowed it would compile, pass every
// call site, and silently reintroduce the second implementation this
// consolidation removed.
func TestWireFunctionsAreTheUpstreamOnes(t *testing.T) {
	samples := []float32{0, 1, -1, 0.5}
	if !bytes.Equal(Float32ToBytes(samples), wsaudio.Float32ToBytes(samples)) {
		t.Error("Float32ToBytes is not audioprism-go's")
	}
	b := wsaudio.Float32ToBytes(samples)
	got, want := BytesToFloat32(b), wsaudio.BytesToFloat32(b)
	if len(got) != len(want) {
		t.Fatalf("BytesToFloat32 returned %d samples, upstream %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("sample %d is %v, upstream %v", i, got[i], want[i])
		}
	}
	if DatagramHeaderSize != wsaudio.DatagramHeaderSize ||
		DefaultMaxDatagramSize != wsaudio.DefaultMaxDatagramSize {
		t.Error("the datagram constants have drifted from upstream")
	}
	a := SplitDatagrams(0xBEEF, []byte{1, 2, 3, 4, 5}, 8)
	c := wsaudio.SplitDatagrams(0xBEEF, []byte{1, 2, 3, 4, 5}, 8)
	if len(a) != len(c) {
		t.Fatalf("SplitDatagrams gave %d datagrams, upstream %d", len(a), len(c))
	}
	for i := range a {
		if !bytes.Equal(a[i], c[i]) {
			t.Errorf("datagram %d differs from upstream", i)
		}
	}
}
