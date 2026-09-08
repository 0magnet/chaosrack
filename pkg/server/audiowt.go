//go:build !js

package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/0magnet/chaosrack/pkg/audiocap"
	"github.com/0magnet/chaosrack/pkg/wtaudio"
	"github.com/gin-gonic/gin"
)

// The WebTransport half of --audio, and why the page reaches for it FIRST.
//
// The audio is the same capture and the same bytes as /ws; what differs is what
// happens when a packet is lost. A WebSocket is TCP, so byte N+1 cannot be
// delivered until byte N has been retransmitted: one loss stalls the feed for a
// round trip, the spectrogram freezes, and then it fast-forwards through the
// backlog. WebTransport carries the audio in unreliable datagrams, so a loss
// costs exactly the ~12 ms that datagram held and nothing after it. A
// visualizer would far rather lose 12 ms than stutter.
//
// None of that matters over loopback and it was offered as an opt-in
// (?audio=wt) for that reason. It is the default now because the opt-in was the
// wrong shape for the case it exists for: the page that needs the lossy-link
// transport is the page being watched from another machine, and that is exactly
// the page whose operator is not going to append a query parameter. Preferring
// it costs nothing where it is not needed, because the page falls back to /ws by
// itself the moment WebTransport cannot be had — the browser has none, this
// server is not offering any, the handshake is refused — and says which
// transport it ended up on.
//
// WHAT THIS CANNOT DO. WebTransport requires a secure context in the browser.
// http://127.0.0.1 and http://localhost are secure contexts, so the local case
// works with nothing installed anywhere — the generated certificate is pinned
// by the SHA-256 the page reads from /wt-info. A page served over plain http to
// any OTHER address is not a secure context, and there the WebTransport
// constructor does not exist at all: the LAN case, which is the one the
// transport is for, gets the WebSocket unless this server is fronted by https.
// That is a property of the browser and there is nothing to wire up here that
// would change it; the page names the reason in its status overlay rather than
// falling back silently.

// wtSrv is the listener, nil when --audio-wt is off or could not bind. Written
// once during mountAudio, before the HTTP server starts accepting, and read
// from request handlers afterwards.
var wtSrv *wtaudio.Server

// mountWebTransport brings up the QUIC listener beside /ws and publishes what a
// page needs to reach it.
//
// FAILING SOFT is the whole error policy. Everything here is a second way to
// carry audio that is already being carried, so a machine that cannot bind UDP
// — a port already taken, a container without the capability — must still get
// the WebSocket feed that has always worked. Every failure below logs and
// returns, leaving wtSrv nil, which makes audioFeed say "ws" and the page never
// look for a WebTransport that is not there.
func mountWebTransport(r *gin.Engine) {
	if !audioWT {
		return
	}
	port := audioWTPort
	if port == 0 {
		port = webPort
	}
	// The socket is bound HERE rather than inside ListenAndServe, because the
	// answer decides what the page is told. audioFeed() is read when a page is
	// rendered, which is after this runs and before any listener goroutine has
	// had time to fail — so a bind error discovered asynchronously would be a
	// page told to prefer a transport that was never going to answer, and the
	// only symptom would be a fallback notice on every load.
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("chaosrack: WebTransport off: %v (the WebSocket feed at /ws is unaffected)", err)
		return
	}
	srv, err := wtaudio.New(wtaudio.Config{
		Addr:       pc.LocalAddr().String(),
		Path:       audioWTPath,
		SampleRate: audioRate,
		Capture:    wtCapture,
	})
	if err != nil {
		log.Printf("chaosrack: WebTransport off: %v (the WebSocket feed at /ws is unaffected)", err)
		_ = pc.Close() //nolint:errcheck // nothing has used it; the only thing to do with the error is what the line above already did
		return
	}
	go func() {
		if err := srv.Serve(pc); err != nil {
			log.Printf("chaosrack: WebTransport listener stopped: %v (pages fall back to /ws)", err)
		}
	}()
	wtSrv = srv

	r.GET("/wt-info", func(c *gin.Context) {
		// The hostname comes from the page's own Host header, so a browser on
		// another machine is sent to an address it can reach rather than to
		// itself. The hash is not a secret — it is the fingerprint of a public
		// certificate — and the page cannot connect without it.
		c.JSON(http.StatusOK, wtSrv.Info(c.Request.Host))
	})

	cert := srv.Cert()
	log.Printf("chaosrack: WebTransport on udp/%d%s — the page prefers it and falls back to /ws by itself", port, audioWTPath)
	log.Printf("chaosrack:   certificate SHA-256 %s (generated this run, valid until %s)",
		cert.Base64(), cert.NotAfter.Format(time.RFC3339))
	log.Printf("chaosrack:   the page reads that from /wt-info; nothing has to be installed in a trust store")
}

// wtCapture is the /ws handler's capture, for a WebTransport session: the same
// options and the same reading of ?ch=2, which asks for the source as recorded
// rather than folded to one channel.
//
// The channel count is per session and not per server for the reason it is on
// the WebSocket: one tab asking for stereo must not change the stream another
// tab is already decoding.
func wtCapture(r *http.Request, write func([]float32) error) (func(), error) {
	return wtCaptureOptions(r).Start(write)
}

// wtCaptureOptions is the decision wtCapture makes, split out so it can be
// tested: Start opens a PulseAudio client, which a test has no business doing
// and no way to do on a machine without a sound server.
func wtCaptureOptions(r *http.Request) audiocap.Options {
	opts := captureOptions()
	if r != nil && r.URL != nil && r.URL.Query().Get("ch") == "2" {
		opts.Channels = 2
	}
	return opts
}

// dropWTSessions is dropAudioConns for the WebTransport feed; see
// wtaudio.Server.CloseSessions for what it costs the pages that were on it.
func dropWTSessions() {
	if wtSrv != nil {
		wtSrv.CloseSessions()
	}
}
