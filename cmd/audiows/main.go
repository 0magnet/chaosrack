// Command audiows is a development/test harness that streams live system
// audio to the wasm-stuff browser build over a WebSocket, and serves the
// wasm page on the same origin so the whole thing works from one process.
//
// It mirrors audioprism-go's coreweb/wasm server: PulseAudio captures the
// default source, and each chunk of float32 samples is base64-encoded
// (little-endian, four bytes per sample) and pushed over /ws. The
// browser-side WebSocket audiosrc.Source (pkg/audiosrc/ws_js.go) decodes
// the identical wire format, so the spectrogram and xy-scope modes behave
// the same here as they do in audioprism-go's wasm build.
//
// Usage:
//
//	go run github.com/0magnet/wasm-stuff/cmd/audiows   # serve on :8080
//	# open http://127.0.0.1:8080/  → redirects to /?audio=ws
//
// The wasm build, wasm_exec.js and page template are embedded (via the
// assets package), so no build step or -dir is required. Requires a running
// PulseAudio (or PipeWire-pulse) server. This binary is Linux-oriented and
// intentionally kept out of the portable root server so that `wasm-stuff`
// stays free of the audio-capture dependency.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	htmpl "html/template"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/0magnet/wasm-stuff/assets"
	"github.com/0magnet/wasm-stuff/assets/gowasm"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/jfreymuth/pulse"
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
	flag.StringVar(&outApps, "out-apps", "brave,chrome,chromium,firefox,edge,vivaldi,opera",
		"comma-separated app-name substrings whose playback (in -wobbulate) is "+
			"auto-moved off the null sink to the real speakers — i.e. the wasm "+
			"page's own Listen output — so only the source app stays captured and "+
			"there's no loop and no manual pavucontrol step")
}

type htmlTemplateData struct {
	WasmExecJs    htmpl.JS
	WasmBase64    string
	Title         string
	OtherLink     string
	OtherLabel    string
	CanonicalPath string
	Debug         bool
	// Dual-runtime fields (unused here — audiows serves the single Go build —
	// but the shared template references them, so they must exist).
	Dual           bool
	GoWasmExecJs   htmpl.JS
	GoWasmBase64   string
	TinyWasmExecJs htmpl.JS
	TinyWasmBase64 string
}

func main() {
	flag.Parse()

	if wobbulate {
		cleanup, err := setupWobbulate()
		if err != nil {
			log.Fatalf("audiows: -wobbulate setup failed: %v", err)
		}
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

	tmpl, err := htmpl.New("index").Parse(assets.IndexTemplate)
	if err != nil {
		log.Fatalf("parse index template: %v", err)
	}
	wasmExecJS := gowasm.WasmExec
	wasmData := gowasm.Wasm

	page := htmlTemplateData{
		WasmExecJs:    htmpl.JS(wasmExecJS), //nolint:gosec // embedded asset
		WasmBase64:    base64.StdEncoding.EncodeToString(wasmData),
		Title:         "Go",
		OtherLink:     "index.html",
		OtherLabel:    "go",
		CanonicalPath: "index.html",
		Debug:         false,
	}

	serveIndex := func(c *gin.Context) {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]interface{}{"Page": page}); err != nil {
			c.String(http.StatusInternalServerError, "template error: %v", err)
			return
		}
		c.Data(http.StatusOK, "text/html;charset=utf-8", buf.Bytes())
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
		c.Data(http.StatusOK, "application/javascript", wasmExecJS)
	})
	r.GET("/b.wasm", func(c *gin.Context) {
		c.Render(http.StatusOK, render.Data{ContentType: "application/wasm", Data: wasmData})
	})
	r.GET("/ws", func(c *gin.Context) {
		websocket.Handler(wsHandler).ServeHTTP(c.Writer, c.Request)
	})

	addr := fmt.Sprintf(":%d", webPort)
	log.Printf("audiows: serving http://127.0.0.1:%d/ (audio via PulseAudio @ %d Hz)", webPort, sampleRate)
	log.Printf("audiows: streaming base64 float32 over ws://127.0.0.1:%d/ws", webPort)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// wsHandler opens a PulseAudio record stream and forwards every chunk to
// the browser as a base64 string, for the lifetime of the WebSocket. One
// PulseAudio client per connection keeps the failure of one browser tab
// from taking down the others (unlike a shared client). Errors are logged
// and end this connection only — never the whole server.
func wsHandler(ws *websocket.Conn) {
	defer func() { _ = ws.Close() }()

	client, err := pulse.NewClient()
	if err != nil {
		log.Printf("audiows: pulse.NewClient: %v (is PulseAudio/PipeWire running?)", err)
		return
	}
	defer client.Close()

	opts := []pulse.RecordOption{pulse.RecordSampleRate(sampleRate), pulse.RecordLatency(latency)}
	if opt, desc, err := recordSourceOption(client); err != nil {
		log.Printf("audiows: source %q: %v — falling back to default source", captureSource, err)
	} else if opt != nil {
		opts = append(opts, opt)
		log.Printf("audiows: capturing %s", desc)
	}

	stream, err := client.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		if len(p) == 0 {
			return 0, nil
		}
		if err := websocket.Message.Send(ws, float32SliceToBase64(p)); err != nil {
			return 0, err // closed connection — unwinds the record stream
		}
		return len(p), nil
	}), opts...)
	if err != nil {
		log.Printf("audiows: NewRecord: %v", err)
		return
	}

	stream.Start()
	defer stream.Stop()

	// Block until the client goes away. The browser never sends data; a
	// receive error means the socket closed, so we can tear down.
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
	}
}

// recordSourceOption resolves the -source flag into a pulse RecordOption for
// this connection. "default" (or "") returns nil so the library uses the
// server's default source. "monitor" captures the default sink's monitor — the
// "record everything playing" tap that makes the visualizer work with ANY app
// (VLC, a browser tab, anything) with no per-app routing, exactly like
// audioprism. Any other value is treated as an explicit source name (e.g.
// "fvf_in.monitor").
func recordSourceOption(client *pulse.Client) (pulse.RecordOption, string, error) {
	switch captureSource {
	case "", "default":
		return nil, "", nil
	case "monitor":
		sink, err := client.DefaultSink()
		if err != nil {
			return nil, "", err
		}
		return pulse.RecordMonitor(sink), "monitor of default sink " + sink.ID(), nil
	default:
		src, err := client.SourceByID(captureSource)
		if err != nil {
			return nil, "", err
		}
		return pulse.RecordSource(src), "source " + src.ID(), nil
	}
}

// setupWobbulate inserts a temporary null sink and makes it the default, so
// every application's audio flows into it and can be captured (and wobbulated)
// with zero per-app routing. It returns a cleanup func that restores the
// previous default sink and unloads the module. The wasm page's OWN output
// (the Listen switch) must be pointed back at the real speakers once in
// pavucontrol — PulseAudio remembers it per-app thereafter — otherwise the
// wobbulated result would loop back into the null sink instead of being heard.
func setupWobbulate() (func(), error) {
	prevSink, err := pactl("get-default-sink")
	if err != nil {
		return nil, fmt.Errorf("get-default-sink: %w", err)
	}
	prevSink = strings.TrimSpace(prevSink)

	moduleID, err := pactl("load-module", "module-null-sink",
		"sink_name="+nullSinkName,
		"sink_properties=device.description=FVF_in")
	if err != nil {
		return nil, fmt.Errorf("load null sink: %w", err)
	}
	moduleID = strings.TrimSpace(moduleID)

	if _, err := pactl("set-default-sink", nullSinkName); err != nil {
		_, _ = pactl("unload-module", moduleID)
		return nil, fmt.Errorf("set-default-sink %s: %w", nullSinkName, err)
	}
	// With the null sink as default, capture its monitor via -source monitor.
	captureSource = "monitor"

	// Keep the browser's own Listen output on the real speakers (not the null
	// sink), automatically, so there's no manual pavucontrol step and no loop.
	go watchWobbulateOutput(nullSinkName, prevSink)

	log.Printf("audiows: -wobbulate on — all system audio now routes through %q (was %q).", nullSinkName, prevSink)
	log.Printf("audiows:   Play audio from ANY app, open the page (?audio=ws), pick FVF, turn on Listen.")
	log.Printf("audiows:   The wasm page's own output is auto-routed to your speakers (apps: %s).", outApps)
	log.Printf("audiows:   Ctrl-C restores the default sink (%q) and removes the null sink.", prevSink)

	return func() {
		if prevSink != "" {
			_, _ = pactl("set-default-sink", prevSink)
		}
		_, _ = pactl("unload-module", moduleID)
	}, nil
}

// watchWobbulateOutput continuously moves any playback stream from a browser
// (the wasm page's Listen output) off the null sink and onto the real
// speakers, so the wobbulated result is heard rather than fed back into the
// capture. The source app being wobbulated (e.g. VLC) is a different app, so
// it stays on the null sink and keeps being captured. Runs until the process
// exits (the -wobbulate cleanup handler tears everything down).
func watchWobbulateOutput(nullSink, target string) {
	var apps []string
	for _, a := range strings.Split(outApps, ",") {
		if a = strings.TrimSpace(strings.ToLower(a)); a != "" {
			apps = append(apps, a)
		}
	}
	if len(apps) == 0 {
		return
	}
	logged := map[string]bool{}
	for {
		time.Sleep(time.Second)
		nullIdx := sinkIndexByName(nullSink)
		if nullIdx == "" {
			continue
		}
		for _, si := range listSinkInputs() {
			if si.sink != nullIdx {
				continue
			}
			al := strings.ToLower(si.app)
			matched := false
			for _, a := range apps {
				if strings.Contains(al, a) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			if _, err := pactl("move-sink-input", si.index, target); err == nil && !logged[si.index] {
				log.Printf("audiows: -wobbulate routed %q output (stream #%s) to %s (heard, not re-captured)", si.app, si.index, target)
				logged[si.index] = true
			}
		}
	}
}

type sinkInput struct{ index, sink, app string }

// listSinkInputs parses `pactl list sink-inputs` into (index, sink index, app
// name) triples. Text parsing (rather than the pulse client) because the
// library exposes playback/record streams, not full sink-input introspection.
func listSinkInputs() []sinkInput {
	out, err := pactl("list", "sink-inputs")
	if err != nil {
		return nil
	}
	var res []sinkInput
	var cur *sinkInput
	flush := func() {
		if cur != nil {
			res = append(res, *cur)
			cur = nil
		}
	}
	for _, ln := range strings.Split(out, "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "Sink Input #"):
			flush()
			cur = &sinkInput{index: strings.TrimPrefix(t, "Sink Input #")}
		case cur == nil:
			// between/ before blocks
		case strings.HasPrefix(t, "Sink:"):
			cur.sink = strings.TrimSpace(strings.TrimPrefix(t, "Sink:"))
		case strings.HasPrefix(t, "application.name = "):
			cur.app = strings.Trim(strings.TrimPrefix(t, "application.name = "), "\"")
		}
	}
	flush()
	return res
}

// sinkIndexByName returns the numeric index of a sink given its name.
func sinkIndexByName(name string) string {
	out, err := pactl("list", "short", "sinks")
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 && f[1] == name {
			return f[0]
		}
	}
	return ""
}

// pactl runs a pactl subcommand and returns its stdout. -wobbulate is a
// Linux/PulseAudio-only convenience, matching the rest of this dev harness.
func pactl(args ...string) (string, error) {
	out, err := exec.Command("pactl", args...).Output() //nolint:gosec // fixed binary name; args are dev-harness constants
	return string(out), err
}

// float32SliceToBase64 encodes samples as little-endian float32 bytes then
// base64 — byte-for-byte identical to audioprism-go's server, so the
// browser decoder in pkg/audiosrc/ws_js.go reads it unchanged.
func float32SliceToBase64(floats []float32) string {
	b := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := math.Float32bits(f)
		b[i*4+0] = byte(bits >> 0)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return base64.StdEncoding.EncodeToString(b)
}
