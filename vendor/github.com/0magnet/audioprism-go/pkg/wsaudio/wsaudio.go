// Package wsaudio is the audio wire format the WebSocket servers here
// (cmd/a/coreweb, cmd/a/wasm) and every client that reads them agree on:
// little-endian float32 samples, four bytes each, and nothing else. No
// header, no channel count, no sample rate — the rate is agreed out of
// band, which is why a mismatch shows up as a pitch shift rather than as
// a decode error.
//
// The samples travel in a WebSocket BINARY frame. They used to be base64
// in a TEXT frame, which costs 4/3 of the bytes: a 24 kHz mono float32
// stream is 96 KB/s of samples and was 128 KB/s on the wire, plus an
// encode per chunk on the server and a decode per chunk in every client.
// Nothing about the payload needed to be text.
//
// Receivers still accept the old encoding (wscodec.Samples branches on
// the frame type; the browser client branches on the JS type of
// event.data), so no flag day: an upgraded client reads an old server and
// an old client reads an upgraded one. Both encodings decode through
// BytesToFloat32 here, so the one definition of the layout is the one
// under test.
package wsaudio

import (
	"encoding/binary"
	"math"
)

// Float32ToBytes encodes samples as little-endian float32. NaN and the
// infinities pass through as their exact bit patterns — no clamping, no
// normalization — because a consumer is better served by whatever was
// captured than by a silently altered signal.
func Float32ToBytes(samples []float32) []byte {
	b := make([]byte, len(samples)*4)
	for i, f := range samples {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// BytesToFloat32 decodes the format Float32ToBytes produces.
//
// A trailing partial sample (a length that is not a multiple of four) is
// dropped rather than reported: a WebSocket frame is all-or-nothing, so a
// short tail can only come from a peer writing a different format, and
// dropping three bytes of it beats discarding the audio that did arrive.
func BytesToFloat32(b []byte) []float32 {
	n := len(b) / 4
	if n == 0 {
		return nil
	}
	samples := make([]float32, n)
	for i := range samples {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return samples
}
