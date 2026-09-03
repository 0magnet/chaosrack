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

	"github.com/0magnet/calvin"
	"github.com/0magnet/chaosrack/assets/gowasm"
	"github.com/0magnet/chaosrack/assets/tinywasm"
	cc "github.com/0magnet/coloredcobra"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/spf13/cobra"
)

var (
	webPort   int
	bindAddr  string
	debugMode bool
)

func init() {
	defaultport, err := strconv.Atoi(os.Getenv("WEBPORT"))
	if err != nil {
		defaultport = 8080
	}
	runCmd.Flags().IntVarP(&webPort, "port", "p", defaultport, "port to serve on - env WEBPORT="+os.Getenv("WEBPORT"))
	runCmd.Flags().StringVarP(&bindAddr, "bind", "b", "", "address to bind (default: every interface; 127.0.0.1 when --shell or --fs is on)")
	runCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "enable /debug/stats profiling endpoint")
}

// Execute runs the root CLI command (the server).
func Execute() {
	cc.Init(&cc.Config{
		RootCmd:         runCmd,
		Headings:        cc.HiBlue + cc.Bold,
		Commands:        cc.HiBlue + cc.Bold,
		CmdShortDescr:   cc.HiBlue,
		Example:         cc.HiBlue + cc.Italic,
		ExecName:        cc.HiBlue + cc.Bold,
		Flags:           cc.HiBlue + cc.Bold,
		FlagsDescr:      cc.HiBlue,
		NoExtraNewlines: true,
		NoBottomNewline: true,
	})
	if err := runCmd.Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}

var runCmd = &cobra.Command{
	Use:   "chaosrack",
	Short: "wasm attractors",
	Long:  calvin.AsciiFont("chaosrack") + "\nwasm attractors",
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
				Title:         "Go",
				CanonicalPath: "index.html",
				Debug:         debugMode,
				Dual:          hasTinygo,
				GoWasmExecJs:  htmpl.JS(gowasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
				GoWasmGzB64:   gzipBase64(gowasm.Wasm),
			}
			if hasTinygo {
				d.TinyWasmExecJs = htmpl.JS(tinywasm.WasmExec) //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
				d.TinyWasmGzB64 = gzipBase64(tinywasm.Wasm)
			} else {
				d.WasmExecJs = d.GoWasmExecJs
				d.WasmGzB64 = d.GoWasmGzB64
			}
			serveInlineWasm(c, d)
		}
		r1.GET("/", dualPage)
		r1.GET("/index.html", dualPage)
		r1.GET("/wasm_exec.js", func(c *gin.Context) {
			c.Data(http.StatusOK, "application/javascript", gowasm.WasmExec)
		})
		r1.GET("/chaosrack.wasm", func(c *gin.Context) {
			c.Render(http.StatusOK, render.Data{ContentType: "application/wasm", Data: gowasm.Wasm})
		})

		// Standalone Go-only page.
		goPage := func(c *gin.Context) {
			serveInlineWasm(c, htmlTemplateData{
				WasmExecJs:    htmpl.JS(gowasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
				WasmGzB64:     gzipBase64(gowasm.Wasm),
				Title:         "Go",
				OtherLink:     "../index.html",
				OtherLabel:    "dual",
				CanonicalPath: "go/index.html",
				Debug:         debugMode,
				AudioFeed:     audioFeed(),
			})
		}
		r1.GET("/go/", goPage)
		r1.GET("/go/index.html", goPage)

		if hasTinygo {
			tinyPage := func(c *gin.Context) {
				serveInlineWasm(c, htmlTemplateData{
					WasmExecJs:    htmpl.JS(tinywasm.WasmExec), //nolint:gosec // wasm_exec.js, compiled into this binary by go:embed — not request data
					WasmGzB64:     gzipBase64(tinywasm.Wasm),
					Title:         "TinyGo",
					OtherLink:     "../index.html",
					OtherLabel:    "dual",
					CanonicalPath: "tinygo/index.html",
					Debug:         debugMode,
				})
			}
			r1.GET("/tinygo/", tinyPage)
			r1.GET("/tinygo/index.html", tinyPage)
			r1.GET("/tinygo/wasm_exec.js", func(c *gin.Context) {
				c.Data(http.StatusOK, "application/javascript", tinywasm.WasmExec)
			})
			r1.GET("/tinygo/chaosrack-tiny.wasm", func(c *gin.Context) {
				c.Render(http.StatusOK, render.Data{ContentType: "application/wasm", Data: tinywasm.Wasm})
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
	WasmGzB64     string // gzipped, then base64 — see wasmgz.go
	Title         string
	OtherLink     string
	OtherLabel    string
	CanonicalPath string
	Debug         bool
	HostConfig    htmpl.JS
	AudioFeed     string // "ws" when this server is capturing; see audio.go
	// Dual mode: embed BOTH runtimes, default to Go, switch via ?wasm=tinygo.
	Dual           bool
	GoWasmExecJs   htmpl.JS
	GoWasmGzB64    string
	TinyWasmExecJs htmpl.JS
	TinyWasmGzB64  string
}
