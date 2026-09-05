//go:build !js

package server

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/0magnet/chaosrack/pkg/audiocap"
	"github.com/0magnet/chaosrack/pkg/audioroute"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

// The audio feed, served by the same process that serves the page.
//
// The capture used to live only in cmd/audiows, so hearing anything meant
// running two servers and telling the page where the second one was
// (?wsurl=ws://…). That is a workaround for where the code lived, not a
// property of audio capture: one listener can serve the page, the host agent
// and the samples.
//
// OPT-IN, because the reason for keeping it out of the root server was partly
// right. The capture opens a PulseAudio client, and a server nobody asked to
// record anything should not touch a sound device on start-up. --audio is the
// asking; without it this registers nothing at all and the route 404s exactly
// as it did before.
var (
	audioOn     bool
	audioRate   int
	audioSource string
	audioLat    float64

	// The FVF routing (pkg/audioroute). Same argument as --audio, one step
	// further: this one changes the machine's default sink, so it is off unless
	// asked for -- but ASKED FOR can now mean a switch in the page as well as a
	// flag, because the two are the same operation and only the flag could
	// reach it before.
	wobbulateOn   bool
	wobbulateSink string
	wobbulateApps string
	wobbulateCtl  bool
)

func init() {
	runCmd.Flags().BoolVar(&audioOn, "audio", false, "capture this machine's audio and serve it at /ws (needs PulseAudio/PipeWire)")
	runCmd.Flags().IntVar(&audioRate, "audio-rate", 24000, "sample rate for --audio; must match the page's ?wsrate=")
	runCmd.Flags().StringVar(&audioSource, "audio-source", "monitor", "what --audio records: monitor (what is playing), default (the input), or a source ID")
	runCmd.Flags().Float64Var(&audioLat, "audio-latency", 0.05, "requested capture latency in seconds for --audio")
	runCmd.Flags().BoolVar(&wobbulateOn, "wobbulate", false, "start with the FVF routing on: all system audio through a temporary null sink so the wobbulated result can be heard out the speakers (restored on exit)")
	runCmd.Flags().StringVar(&wobbulateSink, "wobbulate-sink", audioroute.DefaultSinkName, "name of the null sink --wobbulate creates")
	runCmd.Flags().StringVar(&wobbulateApps, "wobbulate-apps", audioroute.DefaultOutApps, "app-name substrings whose playback --wobbulate moves back to the real speakers (the page's own Listen output)")
	runCmd.Flags().BoolVar(&wobbulateCtl, "wobbulate-ctl", true, "let the page turn the FVF routing on and off (requests from this machine only)")
}

// Live audio state. The routing can be switched while the server runs, so what
// was a start-up decision is now guarded state.
var (
	audioMu    sync.Mutex
	wobbSess   *audioroute.Session
	audioConns = map[*websocket.Conn]struct{}{}
	sigOnce    sync.Once
)

// mountAudio serves the capture at /ws when --audio was passed, and the routing
// switch beside it.
//
// "monitor" is the default source because it is what people mean: capture what
// this machine is PLAYING, not what its microphone hears. The mic is already
// reachable without any server at all — it is what the page uses when no
// ?audio= is given — so a flag that duplicated it would be the less useful of
// the two defaults.
func mountAudio(r *gin.Engine) {
	if wobbulateOn && !audioOn {
		// Routing every app into a null sink so that a capture can pick it up,
		// without starting the capture, leaves the machine silent and nothing
		// listening. Reading it as asking for both is the only reading under
		// which the flag does anything at all.
		log.Printf("chaosrack: --wobbulate implies --audio; capturing as well")
		audioOn = true
	}
	if !audioOn {
		return
	}
	// Every live capture is tracked, because toggling the routing has to end
	// them. audiocap resolves "monitor" to the default sink ONCE, when the
	// stream opens, so a capture that was already running when the default sink
	// changed goes on recording the sink the audio no longer goes to -- silence,
	// with nothing in the logs. Closing the socket is the whole fix: the page's
	// WebSocket source reconnects two seconds after any drop, and the new
	// capture resolves the new default. No reload, no second server.
	r.GET("/ws", gin.WrapH(websocket.Handler(func(ws *websocket.Conn) {
		addAudioConn(ws)
		defer removeAudioConn(ws)
		captureOptions().Serve(ws)
	})))
	log.Printf("chaosrack: --audio on; the page it serves connects to /ws by itself")

	if wobbulateOn {
		if err := setWobbulate(true); err != nil {
			log.Fatalf("chaosrack: --wobbulate: %v", err)
		}
	}
	mountWobbulateCtl(r)
}

// captureOptions is the capture configuration for one connection.
//
// The source is forced to "monitor" while the routing is on, whatever --audio-source
// said. It is not an override of the operator's choice so much as the same
// choice followed: the null sink IS the default sink now, and recording anything
// else would capture the audio the routing just moved away from.
func captureOptions() audiocap.Options {
	audioMu.Lock()
	defer audioMu.Unlock()
	src := audioSource
	if wobbSess != nil {
		src = "monitor"
	}
	return audiocap.Options{SampleRate: audioRate, Latency: audioLat, Source: src}
}

func addAudioConn(ws *websocket.Conn) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioConns[ws] = struct{}{}
}

func removeAudioConn(ws *websocket.Conn) {
	audioMu.Lock()
	defer audioMu.Unlock()
	delete(audioConns, ws)
}

// dropAudioConns closes every live capture so each one is reopened against the
// current default sink. Callers hold audioMu.
//
// Closing does not wait for the handler goroutines: each one is blocked in a
// receive that the close unblocks, and their teardown takes audioMu, which this
// function's caller still holds. They get it when the toggle is done.
func dropAudioConns() {
	for ws := range audioConns {
		_ = ws.Close() //nolint:errcheck // the point is to end the connection; a socket that is already gone is the outcome asked for
	}
	audioConns = map[*websocket.Conn]struct{}{}
}

// setWobbulate installs or removes the FVF routing. Idempotent: asking for the
// state it is already in is not an error, so a page whose switch is out of step
// with the server cannot make it flap.
func setWobbulate(on bool) error {
	audioMu.Lock()
	defer audioMu.Unlock()
	if on == (wobbSess != nil) {
		return nil
	}
	if on {
		s, err := audioroute.Start(audioroute.Options{SinkName: wobbulateSink, OutApps: wobbulateApps})
		if err != nil {
			return err
		}
		wobbSess = s
		restoreRoutingOnSignal()
	} else {
		wobbSess.Stop()
		wobbSess = nil
	}
	dropAudioConns()
	return nil
}

// restoreRoutingOnSignal makes Ctrl-C put the default sink back.
//
// Installed the first time the routing goes on rather than at start-up, so a
// server that never touches the routing keeps the process's ordinary signal
// behavior. It matters here more than most places: the deferred cleanup in a Run
// function does not execute for a process that is signaled, and what that
// leaves behind is a machine whose default sink is a null sink -- every app
// silent, with nothing on screen to say why.
func restoreRoutingOnSignal() {
	sigOnce.Do(func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigc
			audioMu.Lock()
			if wobbSess != nil {
				log.Printf("chaosrack: reverting the FVF audio routing…")
				wobbSess.Stop()
				wobbSess = nil
			}
			audioMu.Unlock()
			os.Exit(0)
		}()
	})
}

// wobbulateStatus is what the page is told, and what a POST answers with.
type wobbulateStatus struct {
	On        bool   `json:"on"`
	Available bool   `json:"available"`
	Sink      string `json:"sink"`
	PrevSink  string `json:"prevSink,omitempty"`
	Error     string `json:"error,omitempty"`
}

func currentWobbulateStatus() wobbulateStatus {
	audioMu.Lock()
	defer audioMu.Unlock()
	st := wobbulateStatus{Available: audioroute.Available(), Sink: wobbulateSink}
	if wobbSess != nil {
		st.On = true
		st.PrevSink = wobbSess.PrevSink()
	}
	return st
}

// wobbulateCtlOffered reports whether the page should show the switch at all.
// Three things have to be true: this server captures, the operator did not
// refuse the control, and the machine has the pactl the routing is made of. A
// switch that cannot work is worse than no switch.
func wobbulateCtlOffered() bool {
	return audioOn && wobbulateCtl && audioroute.Available()
}

// mountWobbulateCtl adds the routing switch's endpoints.
//
// GET is readable by anything that can reach the server; it says what the
// routing is doing and nothing else. POST changes the machine's default sink,
// so it is guarded three ways: the request must come from this machine, it must
// not be a cross-site one, and it must be JSON.
//
// The last two are the CSRF guard, and the JSON requirement is the load-bearing
// half. A form post or a text/plain fetch from any page in the browser is a
// "simple request" that needs no preflight -- the attacker cannot read the
// answer, but the sink is swapped all the same, which is the entire damage. An
// application/json body is not simple, so the browser asks permission first, and
// this server never answers a preflight.
func mountWobbulateCtl(r *gin.Engine) {
	if !wobbulateCtl {
		return
	}
	r.GET("/audio/wobbulate", func(c *gin.Context) {
		c.JSON(http.StatusOK, currentWobbulateStatus())
	})
	r.POST("/audio/wobbulate", func(c *gin.Context) {
		if !fromThisMachine(c.Request) {
			c.JSON(http.StatusForbidden, wobbulateStatus{Error: "the audio routing can only be switched from this machine"})
			return
		}
		if !sameOriginRequest(c.Request) {
			c.JSON(http.StatusForbidden, wobbulateStatus{Error: "cross-site request refused"})
			return
		}
		if ct := c.GetHeader("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			c.JSON(http.StatusUnsupportedMediaType, wobbulateStatus{Error: "send application/json"})
			return
		}
		var body struct {
			On bool `json:"on"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, wobbulateStatus{Error: "unreadable request: " + err.Error()})
			return
		}
		if err := setWobbulate(body.On); err != nil {
			// Not a 500: the request was fine and the machine said no. The page
			// puts the switch back where it was and shows this.
			st := currentWobbulateStatus()
			st.Error = err.Error()
			log.Printf("chaosrack: the page asked for wobbulate=%v: %v", body.On, err)
			c.JSON(http.StatusConflict, st)
			return
		}
		c.JSON(http.StatusOK, currentWobbulateStatus())
	})
	log.Printf("chaosrack: the FVF routing switch is on the page (POST /audio/wobbulate, from this machine only)")
}

// fromThisMachine reports whether a request arrived over the loopback interface.
// The server binds every interface by default, so this is what keeps a routing
// change local even when the page is being watched from across the LAN.
func fromThisMachine(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOriginRequest reports whether the request did not come from another
// site's page. No Origin at all is allowed: that is curl, which the loopback
// check has already vouched for. A browser sends one for anything cross-site.
func sameOriginRequest(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

// audioFeed is what the page is told about audio: "ws" when this server is
// capturing, empty when it is not. Empty means the page never dials a socket
// that nothing is listening on.
func audioFeed() string {
	if audioOn {
		return "ws"
	}
	return ""
}
