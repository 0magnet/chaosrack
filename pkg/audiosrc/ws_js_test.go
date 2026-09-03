//go:build js && wasm

package audiosrc

import (
	"encoding/base64"
	"math"
	"syscall/js"
	"testing"
)

// Both wire encodings must decode to the same samples in the browser,
// which is what lets a new page talk to an old server and the reverse.
// Node has ArrayBuffer and Uint8Array without a DOM, so the branch is
// checked under `make test-wasm` — the real WebSocket never appears.
func TestDecodeWSMessageBothEncodings(t *testing.T) {
	want := []float32{0.5, -0.25, 0, float32(math.Inf(1))}
	raw := Float32ToBytes(want)

	buf := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(buf, raw)

	cases := []struct {
		name string
		data js.Value
	}{
		{"binary frame (ArrayBuffer)", buf.Get("buffer")},
		{"legacy text frame (base64)", js.ValueOf(base64.StdEncoding.EncodeToString(raw))},
	}
	for _, c := range cases {
		got := decodeWSMessage(c.data)
		if len(got) != len(want) {
			t.Errorf("%s: decoded %d samples, want %d", c.name, len(got), len(want))
			continue
		}
		for i := range want {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Errorf("%s: sample %d = %v, want %v", c.name, i, got[i], want[i])
			}
		}
	}
}

// Anything that is neither of the two encodings is dropped rather than
// decoded into noise or a panic: a Blob (if binaryType were ignored), a
// string that is not base64, undefined.
func TestDecodeWSMessageRejects(t *testing.T) {
	cases := []struct {
		name string
		data js.Value
	}{
		{"undefined", js.Undefined()},
		{"null", js.Null()},
		{"not base64", js.ValueOf("!!!not base64!!!")},
		{"object that is not an ArrayBuffer", js.Global().Get("Object").New()},
	}
	for _, c := range cases {
		if got := decodeWSMessage(c.data); got != nil {
			t.Errorf("%s decoded to %v, want nil", c.name, got)
		}
	}
}
