//go:build !js

package server

import (
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
)

// The server's cleanups on Ctrl-C or SIGTERM, all run, in the order they were
// asked for, before it exits. Each used to catch the signal itself and exit
// when done, and with two of them (--shell's sessions, --wobbulate's routing)
// whichever finished first ended the process with the other's work undone: a
// detached shell still running as the user, or the null sink left the default.
var shutdown struct {
	sync.Mutex
	hooks []func()
	once  sync.Once
}

// onShutdown runs f when the server is stopped by a signal.
func onShutdown(f func()) {
	shutdown.Lock()
	shutdown.hooks = append(shutdown.hooks, f)
	shutdown.Unlock()
	shutdown.once.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			shutdown.Lock()
			hooks := slices.Clone(shutdown.hooks)
			shutdown.Unlock()
			for _, h := range hooks {
				h()
			}
			os.Exit(0)
		}()
	})
}
