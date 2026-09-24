//go:build !js

// Package server is the chaosrack web server: it serves the strange-attractor
// visualizer as a self-contained page (wasm inlined as base64), for both the
// standard Go and TinyGo builds, using the embedded assets package. Both the
// repo-root entrypoint and cmd/chaosrack call Execute.
package server

import (
	"encoding/json"
	"fmt"
	htmpl "html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0magnet/calvin/clihelp"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/assets/gowasm"
	"github.com/0magnet/chaosrack/assets/metersworker"
	"github.com/0magnet/chaosrack/assets/tinywasm"
)

var (
	webPort    int
	bindAddr   string
	debugMode  bool
	inlineWasm bool
)

// The URLs the pages fetch their binary from when it is not inlined. They are
// the paths the binaries already occupy in the repository, so the same page
// works served from here and saved into a GitHub Pages checkout — which is the
// property `make pages` depends on, since what it saves IS what this serves.
// Absolute, because the same page is served at /, /go/ and /tinygo/.
const (
	goWasmURL = "/assets/gowasm/chaosrack.wasm"

	// The analyzers' worker and what it loads. One directory, because a
	// worker resolves importScripts and fetch against its own URL.
	metersWorkerJS   = "/assets/metersworker/worker.js"
	metersWorkerWasm = "/assets/metersworker/meters.wasm"
	metersWorkerExec = "/assets/metersworker/wasm_exec.js"
	tinyWasmURL      = "/assets/tinywasm/chaosrack-tiny.wasm"
)

func init() {
	defaultport, err := strconv.Atoi(os.Getenv("WEBPORT"))
	if err != nil {
		defaultport = 8080
	}
	runCmd.Flags().IntVarP(&webPort, "port", "p", defaultport, "port to serve on - env WEBPORT="+os.Getenv("WEBPORT"))
	runCmd.Flags().StringVarP(&bindAddr, "bind", "b", "", "address to bind (default: every interface; 127.0.0.1 when --shell or --fs is on)")
	runCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "enable /debug/stats profiling endpoint")
	runCmd.Flags().BoolVar(&inlineWasm, "inline", false, "carry the wasm inside the HTML instead of fetching it (the single-file build; see `make onefile`)")
}

// Execute runs the root CLI command (the server).
func Execute() {
	clihelp.Init(runCmd, "chaosrack", true)
	if err := runCmd.Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}

var runCmd = &cobra.Command{
	Use:   "chaosrack",
	Short: "an analog computer in the browser",
	Long: `chaosrack is an analog computer in the browser: a rack of instruments for
dynamical systems, geometry and live signals, computed by a Go WebAssembly
core and played from a panel of knobs, switches and LED readouts.

What the rack can show:

  flows        Lorenz, Rössler, Chua, the Sprott cases, or a system you type in
  maps         Hénon, Ikeda, Clifford, de Jong, Tinkerbell, the standard map
  geometry     parametric figures, polyhedra, sequence walks and solids,
               with the rack's own panels exportable as printable STL
  audio        spectrogram, XY scope, delay embeddings, and an analyzer suite:
               distortion, RTA, transfer function, loudness, wow & flutter,
               waterfall
  ripple tank  a wave-equation water surface driven by drags or by sound
  terminal     a real shell in the page, and a desk of windows to work in

Audio can be routed into any parameter, any flow can be measured for chaos by its
Lyapunov exponent, and a model's output can be played as sound.

With no subcommand, chaosrack starts a web server for the rack. Open it at:

  /           the Go build, switchable to TinyGo with ?wasm=tinygo
  /go/        the Go build only
  /tinygo/    the TinyGo build only (when this binary includes it)

The page fetches its wasm by default. Use --inline to put the wasm in the HTML
instead, so a saved copy of the page runs by itself.

Optional flags give the page access to this machine:

  --audio      stream this machine's audio to the rack (PulseAudio/PipeWire)
  --wobbulate  route system audio through the FVF harmonic wobbulator
  --shell      run a real shell in the page's terminal and desk
  --fs         read and write local files, limited to --fs-root if given

--shell and --fs listen on 127.0.0.1 unless you choose another address with
--bind. Add --auth on a shared machine, so the page has to ask for the token.

The subcommands tui, ctl and rack control a rack that is already open in a
browser. render and models draw models to image files without a browser.`,
	Run: func(_ *cobra.Command, _ []string) {
		wg := new(sync.WaitGroup)
		r1 := gin.New()
		r1.Use(gin.Recovery())
		r1.Use(loggingMiddleware())

		hasTinygo := tinywasm.Has()

		// Default page at / and /index.html: DUAL — embeds both runtimes, boots
		// the Go build by default, switches to TinyGo via ?wasm=tinygo (the same
		// static HTML picks the runtime from the query, so it works on GitHub
		// Pages). Falls back to Go-only if this build has no TinyGo asset.
		dualPage := func(c *gin.Context) {
			d := htmlTemplateData{
				HostConfig:    hostConfigJS(),
				AudioFeed:     audioFeed(),
				WobbulateCtl:  wobbulateCtlOffered(),
				Title:         "Go",
				CanonicalPath: "",
				Debug:         debugMode,
				Dual:          hasTinygo,
				GoWasmExecJs:  htmpl.JS(gowasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
			}
			// Inlined, the dual page carries BOTH runtimes — 9.5 MB transferred
			// to run one of them. Fetched, only the one selected is asked for.
			if inlineWasm {
				d.GoWasmGzB64 = gzipBase64(gowasm.Wasm)
			} else {
				d.GoWasmURL, d.TinyWasmURL = goWasmURL, tinyWasmURL
			}
			if hasTinygo {
				d.TinyWasmExecJs = htmpl.JS(tinywasm.WasmExec) //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
				if inlineWasm {
					d.TinyWasmGzB64 = gzipBase64(tinywasm.Wasm)
				}
			} else {
				d.WasmExecJs = d.GoWasmExecJs
				d.WasmGzB64 = d.GoWasmGzB64
				d.WasmURL = d.GoWasmURL
			}
			serveInlineWasm(c, d)
		}
		r1.GET("/", dualPage)
		r1.GET("/index.html", dualPage)
		r1.GET("/wasm_exec.js", func(c *gin.Context) {
			serveAsset(c, "application/javascript", gowasm.WasmExec)
		})
		r1.GET("/chaosrack.wasm", func(c *gin.Context) {
			serveAsset(c, "application/wasm", gowasm.Wasm)
		})
		// The same binaries at the paths they occupy in the repository, which is
		// what the fetched pages ask for. Served here so a page saved by
		// `make pages` behaves identically here and on GitHub Pages, where these
		// are ordinary committed files — one page, two places, no rewriting.
		r1.GET(goWasmURL, func(c *gin.Context) {
			serveAsset(c, "application/wasm", gowasm.Wasm)
		})
		if hasTinygo {
			r1.GET(tinyWasmURL, func(c *gin.Context) {
				serveAsset(c, "application/wasm", tinywasm.Wasm)
			})
		}
		// The analyzers' worker: its own small wasm build, the script that
		// loads it, and a copy of wasm_exec.js beside them. Same directory as
		// the worker script, because importScripts and fetch inside a worker
		// resolve against the worker's own URL and not the page's.
		r1.GET(metersWorkerJS, func(c *gin.Context) {
			serveAsset(c, "application/javascript", metersworker.WorkerJS)
		})
		r1.GET(metersWorkerWasm, func(c *gin.Context) {
			serveAsset(c, "application/wasm", metersworker.Wasm)
		})
		r1.GET(metersWorkerExec, func(c *gin.Context) {
			serveAsset(c, "application/javascript", gowasm.WasmExec)
		})

		// Standalone Go-only page.
		goPage := func(c *gin.Context) {
			serveInlineWasm(c, htmlTemplateData{
				WasmExecJs:    htmpl.JS(gowasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
				WasmGzB64:     inlineOnly(gowasm.Wasm),
				WasmURL:       fetchedFrom(goWasmURL),
				Title:         "Go",
				OtherLink:     "../index.html",
				OtherLabel:    "dual",
				CanonicalPath: "",
				Debug:         debugMode,
				AudioFeed:     audioFeed(),
				WobbulateCtl:  wobbulateCtlOffered(),
			})
		}
		r1.GET("/go/", goPage)
		r1.GET("/go/index.html", goPage)

		if hasTinygo {
			tinyPage := func(c *gin.Context) {
				serveInlineWasm(c, htmlTemplateData{
					WasmExecJs:    htmpl.JS(tinywasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
					WasmGzB64:     inlineOnly(tinywasm.Wasm),
					WasmURL:       fetchedFrom(tinyWasmURL),
					Title:         "TinyGo",
					OtherLink:     "../index.html",
					OtherLabel:    "dual",
					CanonicalPath: "",
					Debug:         debugMode,
					AudioFeed:     audioFeed(),
					WobbulateCtl:  wobbulateCtlOffered(),
				})
			}
			r1.GET("/tinygo/", tinyPage)
			r1.GET("/tinygo/index.html", tinyPage)
			r1.GET("/tinygo/wasm_exec.js", func(c *gin.Context) {
				serveAsset(c, "application/javascript", tinywasm.WasmExec)
			})
			r1.GET("/tinygo/chaosrack-tiny.wasm", func(c *gin.Context) {
				serveAsset(c, "application/wasm", tinywasm.Wasm)
			})
		}

		if debugMode {
			var latestStats json.RawMessage
			var statsMu sync.RWMutex
			r1.POST("/debug/stats", func(c *gin.Context) {
				body, err := io.ReadAll(c.Request.Body)
				if err != nil {
					c.Status(http.StatusBadRequest)
					return
				}
				statsMu.Lock()
				latestStats = body
				statsMu.Unlock()
				c.Status(http.StatusOK)
			})
			r1.GET("/debug/stats", func(c *gin.Context) {
				statsMu.RLock()
				data := latestStats
				statsMu.RUnlock()
				if data == nil {
					c.JSON(http.StatusOK, gin.H{"status": "waiting for WASM to report stats..."})
					return
				}
				c.Data(http.StatusOK, "application/json", data)
			})
		}

		// The listener is made here rather than left to gin's Run, because the
		// host agent has to know what it is: whether it is loopback, and which
		// origins a page it served can legitimately come from.
		ln, err := net.Listen("tcp", listenAddress())
		if err != nil {
			log.Fatalf("chaosrack: %v", err)
		}
		mountHostAgent(r1, ln)
		mountAudio(r1)

		wg.Add(1)
		go func() {
			fmt.Printf("listening on http://127.0.0.1:%d using gin router\n", webPort)
			fmt.Printf("  Go WASM:     http://127.0.0.1:%d/index.html\n", webPort)
			if hasTinygo {
				fmt.Printf("  TinyGo WASM: http://127.0.0.1:%d/tinygo/index.html\n", webPort)
			}
			if err := r1.RunListener(ln); err != nil {
				panic(err)
			}
			wg.Done()
		}()
		wg.Wait()
	},
}

// inlineOnly and fetchedFrom are the two halves of the same switch, so a page
// cannot end up with both a payload and a URL (the template would emit the
// megabytes and then ignore them) or with neither (it would have nothing to
// run). gzipBase64 is the expensive half and is not called at all when the
// page is going to fetch instead.
func inlineOnly(wasm []byte) htmpl.HTML {
	if !inlineWasm {
		return ""
	}
	return gzipBase64(wasm)
}

func fetchedFrom(url string) string {
	if inlineWasm {
		return ""
	}
	return url
}

func serveInlineWasm(c *gin.Context, data htmlTemplateData) {
	html, err := renderTemplate(data)
	if err != nil {
		serveError(c, err.Error())
		return
	}
	c.Data(http.StatusOK, "text/html;charset=utf-8", html)
}

func serveError(c *gin.Context, msg string) {
	fmt.Println(msg)
	c.Data(http.StatusInternalServerError, "text/html;charset=utf-8",
		[]byte(fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`,
			strings.ReplaceAll(msg, "\n", "<br>"))))
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		if latency > time.Minute {
			latency = latency.Truncate(time.Second)
		}
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		fmt.Printf("[WASMSTUFF] | %s |%s %3d %s| %13v | %15s | %72s |%s %-7s %s %s\n",
			time.Now().Format("2006/01/02 - 15:04:05"), getBackgroundColor(statusCode), statusCode, resetColor(),
			latency, c.ClientIP(), c.Request.RemoteAddr, getMethodColor(method), method, resetColor(), path)
	}
}

func getBackgroundColor(statusCode int) string {
	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
		return green
	case statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest:
		return white
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return yellow
	default:
		return red
	}
}

func getMethodColor(method string) string {
	switch method {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return cyan
	case http.MethodPut:
		return yellow
	case http.MethodDelete:
		return red
	case http.MethodPatch:
		return green
	case http.MethodHead:
		return magenta
	case http.MethodOptions:
		return white
	default:
		return reset
	}
}
func resetColor() string { return reset }

const (
	green   = "\033[97;42m"
	white   = "\033[90;47m"
	yellow  = "\033[90;43m"
	red     = "\033[97;41m"
	blue    = "\033[97;44m"
	magenta = "\033[97;45m"
	cyan    = "\033[97;46m"
	reset   = "\033[0m"
)

type htmlTemplateData struct {
	WasmExecJs    htmpl.JS
	WasmGzB64     htmpl.HTML // gzipped, then base64 — see wasmgz.go
	WasmURL       string     // fetched instead, when not inlined; see inlineOnly/fetchedFrom
	Title         string
	OtherLink     string
	OtherLabel    string
	CanonicalPath string // see PageOptions.CanonicalPath — empty means the site root
	Debug         bool
	HostConfig    htmpl.JS
	AudioFeed     string // the transport the page should prefer ("wt", "ws" or none); see audio.go
	WobbulateCtl  bool   // the page may offer the FVF routing switch; see audio.go
	// Dual mode: embed BOTH runtimes, default to Go, switch via ?wasm=tinygo.
	Dual           bool
	GoWasmExecJs   htmpl.JS
	GoWasmGzB64    htmpl.HTML
	GoWasmURL      string
	TinyWasmExecJs htmpl.JS
	TinyWasmGzB64  htmpl.HTML
	TinyWasmURL    string
}
