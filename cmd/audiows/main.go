//go:build !js

// Command audiows is a development/test harness that streams live system
// audio to the chaosrack browser build over a WebSocket, and serves the
// wasm page on the same origin so the whole thing works from one process.
//
// It mirrors audioprism-go's coreweb/wasm server: PulseAudio captures the
// default source, and each chunk of float32 samples (little-endian, four
// bytes per sample) is pushed over /ws in a binary frame. The browser-side
// WebSocket audiosrc.Source (pkg/audiosrc/ws_js.go) decodes the identical
// wire format, so the spectrogram and xy-scope modes behave the same here
// as they do in audioprism-go's wasm build.
//
// The same audio is also offered over WebTransport (HTTP/3 over QUIC) on
// the same port number over UDP, as an OPTION reached with ?audio=wt. The
// WebSocket is unchanged and remains the default: over loopback, which is
// what this harness is for, WebTransport buys nothing — 96 KB/s of
// samples and TCP never drops a packet locally. It pays off on a remote
// or lossy link, where the audio rides unreliable datagrams so a lost
// packet costs one chunk instead of stalling the whole feed. See
// pkg/wtaudio and pkg/audiosrc/datagram.go.
//
// Usage:
//
//	go run github.com/0magnet/chaosrack/cmd/audiows   # serve on :8080
//	# open http://127.0.0.1:8080/  → redirects to /?audio=ws
//	# ?audio=wt instead for the WebTransport path (falls back to ws)
//
// The wasm build, wasm_exec.js and page template are embedded (via the
// assets package), so no build step or -dir is required. Requires a running
// PulseAudio (or PipeWire-pulse) server.
//
// WHAT THIS IS STILL FOR. It is no longer the way to hear system audio: the
// root server captures on --audio and installs the FVF routing on --wobbulate,
// in the process that is already serving the page, and it carries the routing
// switch the page can operate at runtime. What is left here that is not there
// is the WebTransport listener -- the ?audio=wt path, which is what this
// harness exists to exercise. Reach for `chaosrack --audio` first.
package main

import (
	"flag"
	"fmt"
	htmpl "html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0magnet/chaosrack/assets/gowasm"
	"github.com/0magnet/chaosrack/pkg/audiocap"
	"github.com/0magnet/chaosrack/pkg/audioroute"
	"github.com/0magnet/chaosrack/pkg/server"
	"github.com/0magnet/chaosrack/pkg/wtaudio"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"golang.org/x/net/websocket"
)

var (
	webPort       int
	sampleRate    int
	latency       float64
	captureSource string
	wobbulate     bool
	nullSinkName  string
	outApps       string
	enableWT      bool
	wtPort        int
	wtPath        string
)

func init() {
	flag.IntVar(&webPort, "port", 8080, "port to serve on")
	flag.IntVar(&webPort, "p", 8080, "port to serve on (shorthand)")
	flag.IntVar(&sampleRate, "rate", 24000, "PulseAudio record sample rate (Hz) — 24000 matches audioprism-go's spectrogram")
	flag.Float64Var(&latency, "latency", 0.1, "PulseAudio record latency (seconds)")
	flag.StringVar(&captureSource, "source", "monitor",
		"capture source: 'monitor' (the default sink's monitor — picks up ALL "+
			"system audio, like audioprism), 'default' (default input, usually the "+
			"mic), or an explicit source name (e.g. fvf_in.monitor)")
	flag.BoolVar(&wobbulate, "wobbulate", false,
		"route ALL system audio through a temporary null sink so any app can be "+
			"wobbulated with no per-app routing; the default sink is swapped and "+
			"restored on exit (Ctrl-C)")
	flag.StringVar(&nullSinkName, "sink-name", "fvf_in", "null-sink name used by -wobbulate")
	flag.BoolVar(&enableWT, "wt", true,
		"also offer the audio over WebTransport (HTTP/3 over QUIC, UDP) for ?audio=wt; "+
			"the WebSocket is unaffected and stays the default, and a WebTransport that "+
			"fails to start is logged rather than fatal")
	flag.IntVar(&wtPort, "wt-port", 0,
		"UDP port for WebTransport (0 = the same number as -port; QUIC is UDP so the "+
			"numbers can be shared, and sharing them keeps the browser's origin check happy)")
	flag.StringVar(&wtPath, "wt-path", "/wt", "WebTransport endpoint path")
	flag.StringVar(&outApps, "out-apps", "brave,chrome,chromium,firefox,edge,vivaldi,opera",
		"comma-separated app-name substrings whose playback (in -wobbulate) is "+
			"auto-moved off the null sink to the real speakers — i.e. the wasm "+
			"page's own Listen output — so only the source app stays captured and "+
			"there's no loop and no manual pavucontrol step")
}

func main() {
	flag.Parse()

	if wobbulate {
		sess, err := audioroute.Start(audioroute.Options{SinkName: nullSinkName, OutApps: outApps})
		if err != nil {
			log.Fatalf("audiows: -wobbulate setup failed: %v", err)
		}
		cleanup := sess.Stop
		// With the null sink as default, the thing to record is its monitor.
		captureSource = "monitor"
		// Revert the routing on Ctrl-C / SIGTERM so we never leave the
		// system's default sink pointed at a null sink after we exit.
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigc
			log.Printf("audiows: reverting audio routing…")
			cleanup()
			os.Exit(0)
		}()
		defer cleanup()
	}

	// The page is rendered by pkg/server, which owns the template and how the
	// wasm is encoded into it. This used to be a second copy of that struct,
	// and it fell out of step the moment the inlined binary started being
	// gzipped: every request answered "template error: can't evaluate field
	// WasmGzB64". One template, one definition of what feeds it.
	html, err := server.RenderPage(server.PageOptions{
		AudioFeed:     "ws",
		Wasm:          gowasm.Wasm,
		WasmExecJs:    htmpl.JS(gowasm.WasmExec), //nolint:gosec // embedded asset
		Title:         "Go",
		OtherLink:     "index.html",
		OtherLabel:    "go",
		CanonicalPath: "index.html",
	})
	if err != nil {
		log.Fatalf("rendering the page: %v", err)
	}

	serveIndex := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html;charset=utf-8", html)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// Land on the ws backend automatically, matching audioprism-go's
	// zero-config auto-connect. The wasm reads ?audio=ws to pick the
	// WebSocket source over the default microphone source.
	r.GET("/", func(c *gin.Context) {
		if c.Query("audio") == "" {
			c.Redirect(http.StatusFound, "/?audio=ws")
			return
		}
		serveIndex(c)
	})
	r.GET("/index.html", serveIndex)
	r.GET("/wasm_exec.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/javascript", gowasm.WasmExec)
	})
	r.GET("/chaosrack.wasm", func(c *gin.Context) {
		c.Render(http.StatusOK, render.Data{ContentType: "application/wasm", Data: gowasm.Wasm})
	})
	r.GET("/ws", func(c *gin.Context) {
		websocket.Handler(capOpts().Serve).ServeHTTP(c.Writer, c.Request)
	})

	// The optional WebTransport listener. Everything about it is
	// additive: it fails soft, it is on a different socket (UDP), and no
	// page reaches it without ?audio=wt.
	wt := startWebTransport()
	r.GET("/wt-info", func(c *gin.Context) {
		if wt == nil {
			// The page reads a 404 here as "this server has no
			// WebTransport" and falls back to the WebSocket, which is
			// exactly right — it is what -wt=false means.
			c.Status(http.StatusNotFound)
			return
		}
		// The hash is not a secret — it is a fingerprint of a public
		// certificate — but the page has to be able to read it from
		// wherever it is served, including another machine.
		c.JSON(http.StatusOK, wt.Info(c.Request.Host))
	})

	addr := fmt.Sprintf(":%d", webPort)
	log.Printf("audiows: serving http://127.0.0.1:%d/ (audio via PulseAudio @ %d Hz)", webPort, sampleRate)
	log.Printf("audiows: streaming binary float32 over ws://127.0.0.1:%d/ws", webPort)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// capOpts is this command.s capture configuration, from its flags. The capture
// itself lives in pkg/audiocap so the root server can serve the same feed from
// the same process that serves the page -- see its --audio flag.
func capOpts() audiocap.Options {
	return audiocap.Options{SampleRate: sampleRate, Latency: latency, Source: captureSource}
}

// startWebTransport brings up the optional QUIC listener, returning nil
// when it is switched off or cannot start.
//
// Failing soft is deliberate: this is an extra transport nothing reaches
// without ?audio=wt, and a machine where UDP cannot be bound must still
// get the WebSocket harness that has always worked. The page treats a
// missing /wt-info as "no WebTransport here" and falls back on its own.
func startWebTransport() *wtaudio.Server {
	if !enableWT {
		return nil
	}
	port := wtPort
	if port == 0 {
		port = webPort
	}
	srv, err := wtaudio.New(wtaudio.Config{
		Addr:       fmt.Sprintf(":%d", port),
		Path:       wtPath,
		Capture:    capOpts().Start,
		SampleRate: sampleRate,
	})
	if err != nil {
		log.Printf("audiows: WebTransport off: %v (the WebSocket path is unaffected)", err)
		return nil
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("audiows: WebTransport listener stopped: %v (the WebSocket path is unaffected)", err)
		}
	}()
	cert := srv.Cert()
	log.Printf("audiows: WebTransport on udp/%d%s — open /?audio=wt", port, wtPath)
	log.Printf("audiows:   certificate SHA-256 %s (generated this run, valid until %s)",
		cert.Base64(), cert.NotAfter.Format(time.RFC3339))
	log.Printf("audiows:   the page reads that from /wt-info; nothing has to be installed in a trust store")
	return srv
}
