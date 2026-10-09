//go:build !js

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// What the page is told decides which transport it reaches for before it has
// asked the server anything, so this is the one place the preference is
// expressed and it has to say only what is true.
//
// The case that matters is the third: a server capturing with no WebTransport
// listener must say "ws". Saying "wt" there would send every page through a
// /wt-info fetch that 404s and a fallback notice, on every load, to arrive at
// the transport it could have been given directly.
func TestAudioFeedNamesTheTransportOnOffer(t *testing.T) {
	savedOn, savedWT := audioOn, wtAudio
	defer func() { audioOn, wtAudio = savedOn, savedWT }()

	for _, c := range []struct {
		name string
		on   bool
		wt   bool // the audio is on the WebTransport listener
		want string
	}{
		{"not capturing", false, false, ""},
		{"not capturing, listener somehow up", false, true, ""},
		{"capturing, WebTransport up", true, true, "wt"},
		// The listener can be up for the rack link with no audio on it
		// (--audio-wt=false): the page must not look for audio there.
		{"capturing, no audio over WebTransport", true, false, "ws"},
	} {
		audioOn, wtAudio = c.on, c.wt
		if got := audioFeed(); got != c.want {
			t.Errorf("%s: audioFeed() = %q, want %q", c.name, got, c.want)
		}
	}
}

// The session's channel count comes from its own query and from nowhere else.
// Per session rather than per server because one tab asking for stereo must not
// change the stream another tab is already decoding — the WebSocket handler has
// always read it this way and the two must not disagree, or the page would get
// two channels or one depending on which transport it landed on.
func TestWTCaptureReadsTheChannelQuery(t *testing.T) {
	for _, c := range []struct {
		query string
		want  int
	}{
		{"", 1},
		{"?ch=2", 2},
		{"?ch=1", 1},
		{"?ch=stereo", 1}, // not the value the WebSocket handler accepts either
	} {
		r := httptest.NewRequest(http.MethodConnect, "/wt"+c.query, http.NoBody)
		got := wtCaptureOptions(r).Channels
		if got == 0 {
			got = 1 // left at zero means one; see audiocap.Options.withDefaults
		}
		if got != c.want {
			t.Errorf("%q asked for %d channels, want %d", c.query, got, c.want)
		}
	}
	// A capture started from anywhere but a request — nothing does today, and
	// a nil dereference in the handler would take the listener with it.
	if got := wtCaptureOptions(nil).Channels; got == 2 {
		t.Error("no request at all asked for two channels")
	}
}
