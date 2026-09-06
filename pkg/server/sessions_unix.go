//go:build !js && !windows && !plan9

package server

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/0magnet/desk/panes/hostagent"
)

// printSessionsOnSignal lists the detached host shells on SIGUSR1.
//
// A detached shell is invisible by design: the agent has no listing endpoint
// and a miss CREATES a session rather than reporting one, so that the wire
// cannot be used as an oracle for which session names are live. That is right
// for the page and leaves the operator — who owns the machine and started the
// process — with no way to answer "what did I leave running?"
//
// A signal answers it on a channel the browser cannot reach: the terminal the
// server was started in. Nothing is added to the served surface.
//
// The default disposition of SIGUSR1 is to KILL THE PROCESS, which is why
// installing this handler is a change in behavior rather than an addition, and
// why it is scoped to the reconnect path exactly as reapSessionsOnSignal is:
// without a registry there are no detached shells to list, and the flag being
// off should leave the signal meaning what it meant before.
//
// Buffered at one, looping forever. Both halves matter: signal.Notify drops
// rather than blocks when the buffer is full, so a burst of kill -USR1
// collapses into one listing instead of queueing a hundred, and a handler that
// returned after the first signal would answer once and then be a kill switch
// again for the rest of the run.
func printSessionsOnSignal(reg *hostagent.Registry) {
	if reg == nil {
		return
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for range c {
			// Listed before taking the print lock: List waits on each
			// session's own mutex, and one slow session must not hold up an
			// unrelated event line.
			list := reg.List()
			hostPrint.Lock()
			hostagent.PrintSessions(os.Stdout, "chaosrack: ", list)
			hostPrint.Unlock()
		}
	}()
}
