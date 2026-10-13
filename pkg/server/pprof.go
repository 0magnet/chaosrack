//go:build !js

package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
)

// pprofAddr is where the panel serves Go's profiler, or "" for nowhere.
var pprofAddr string

// servePprof serves /debug/pprof on pprofAddr for as long as the process
// lives: the panel's cost is in drawing what it reads back, and the only way
// to see where is to profile it while it runs, at the terminal size it runs at.
func servePprof() {
	if pprofAddr == "" {
		return
	}
	l, err := net.Listen("tcp", pprofAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chaosrack: --pprof: %v\n", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	go http.Serve(l, mux) //nolint:errcheck,gosec // lives as long as the panel; a localhost debugging aid
}
