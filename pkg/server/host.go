//go:build !js

package server

import (
	"encoding/json"
	"fmt"
	htmpl "html/template"
	"log"
	"net"
	"strconv"

	"github.com/0magnet/desk/panes/hostagent"
	"github.com/0magnet/desk/panes/hostproto"
	"github.com/gin-gonic/gin"
)

// Reaching the machine from chaosrack.
//
// The same agent desk-serve mounts, mounted here, so the desk inside chaosrack
// can have a real shell and real files rather than only websh's in-memory
// ones. It is one import and a route: the agent is an http.Handler and knows
// nothing about who is serving it, which is why it could move here without
// being changed.
//
// The DEFAULTS are the ones desk-serve uses, for the same reasons and with the
// same honesty about what they are worth:
//
//   - nothing is served unless --shell or --fs is passed;
//   - either flag with a non-loopback listener is refused outright rather than
//     warned about, because "a shell, reachable from the network, because a
//     flag defaulted" is not a sentence this should be able to produce;
//   - the Origin must match a page this listener served, and ordinary requests
//     are checked with Sec-Fetch-Site as well, because a browser does not send
//     Origin on a same-origin GET;
//   - a token from crypto/rand, per run, never written to disk.
//
// The Origin check is the load-bearing one. The token is honestly weaker: a
// browser sets Origin itself and script cannot forge it, whereas a local
// process running as you can read the token out of the served page — and such
// a process already has your shell without asking this one.

var (
	hostShell  bool
	hostShcmd  string
	hostFS     bool
	hostFSRoot string

	// hostToken is generated once, when the agent is mounted, and handed to
	// the page through the template. Empty means no agent, which is what a
	// static host and every default run produce.
	hostToken string
	hostHasFS bool
	hostAuth  bool
)

func init() {
	runCmd.Flags().BoolVar(&hostShell, "shell", false, "let the page run a real shell on this machine")
	runCmd.Flags().StringVar(&hostShcmd, "shell-cmd", "", "what --shell starts (default $SHELL, then /bin/sh)")
	runCmd.Flags().BoolVar(&hostFS, "fs", false, "let the page read and write this machine's files")
	runCmd.Flags().StringVar(&hostFSRoot, "fs-root", "", "confine --fs to this subtree (default: the whole filesystem)")
	runCmd.Flags().BoolVar(&hostAuth, "auth", false, "print the token instead of putting it in the page, and ask for it (for shared machines)")
}

// hostWanted reports whether either half of the agent was asked for.
func hostWanted() bool { return hostShell || hostFS }

// listenAddress is what the server binds.
//
// Every interface by default, because serving the visualizer on a LAN is a
// reasonable thing to want. Asking for a shell or the filesystem changes the
// DEFAULT to loopback rather than forcing it — an explicit --bind is still
// obeyed, and then refused by mountHostAgent if it is not loopback. The
// difference matters: a default that quietly overrode what someone typed would
// be worse than a refusal that says why.
func listenAddress() string {
	if bindAddr != "" {
		if _, _, err := net.SplitHostPort(bindAddr); err == nil {
			return bindAddr // already host:port
		}
		return net.JoinHostPort(bindAddr, strconv.Itoa(webPort))
	}
	if hostWanted() {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(webPort))
	}
	return fmt.Sprintf(":%d", webPort)
}

// mountHostAgent adds the agent's routes to the router, and returns without
// doing anything if it was not asked for.
//
// gin is handed the agent with WrapH: the agent is a plain http.Handler, and
// the fewer frameworks that know about it the better. Its own guards run
// before anything it serves.
func mountHostAgent(r *gin.Engine, ln net.Listener) {
	if !hostWanted() {
		return
	}
	// Refusing rather than warning. The Origin check makes a non-loopback
	// listener less bad than it sounds, but this is not a decision to make by
	// omission.
	if a, ok := ln.Addr().(*net.TCPAddr); !ok || !a.IP.IsLoopback() {
		log.Fatalf("chaosrack: --shell/--fs need a loopback address; %s is reachable from elsewhere\n"+
			"           (drop --bind, or bind 127.0.0.1)", ln.Addr())
	}

	token, err := hostagent.NewToken()
	if err != nil {
		log.Fatalf("chaosrack: %v", err)
	}
	hostToken = token
	hostHasFS = hostFS
	if hostAuth {
		fmt.Printf("\nchaosrack: --auth is ON. The host shell will ask for this token:\n\n    %s\n\n", token)
	}

	cfg := hostagent.Config{
		Token:   token,
		Origins: servedOrigins(ln),
		Session: hostagent.SessionConfig{Shell: hostShcmd},
	}
	if hostShell {
		r.GET(hostproto.Path, gin.WrapH(cfg.Handler()))
	}
	if hostFS {
		// One route per verb rather than a wildcard: gin's wildcards and the
		// agent's own mux would both want to own the path, and the agent is
		// the one that knows what its endpoints are.
		h := gin.WrapH(cfg.FSHandler(hostagent.FSConfig{Root: hostFSRoot}))
		for _, p := range []string{
			hostproto.FSStat, hostproto.FSList, hostproto.FSRead, hostproto.FSWrite,
			hostproto.FSMkdir, hostproto.FSRemove, hostproto.FSRename,
			hostproto.FSChmod, hostproto.FSChtimes, hostproto.FSTruncate,
		} {
			r.GET(p, h)
			r.POST(p, h)
		}
	}
	warnAboutHostAccess()
}

// servedOrigins is the set of Origin values a browser might send for a page
// this listener served.
//
// Both spellings, because they are different origins to the browser and only
// one of them is what the address bar ends up saying: the server may print
// 127.0.0.1 while the person types localhost, or the reverse, and a mismatch
// would refuse the connection for a reason that looks like a network fault.
func servedOrigins(ln net.Listener) []string {
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return []string{"http://" + ln.Addr().String()}
	}
	return []string{
		fmt.Sprintf("http://127.0.0.1:%d", addr.Port),
		fmt.Sprintf("http://localhost:%d", addr.Port),
		fmt.Sprintf("http://[::1]:%d", addr.Port),
	}
}

// hostConfigJS is what the page is told, as a JSON object for the template.
//
// Injected into the page rather than fetched, because a page that has to fetch
// its token can open a window before the answer arrives, and then the first
// shell of every session fails once.
func hostConfigJS() htmpl.JS {
	if hostToken == "" {
		return ""
	}
	cfg := struct {
		Token string `json:"token,omitempty"`
		Auth  bool   `json:"auth,omitempty"`
		Path  string `json:"path,omitempty"`
		FS    bool   `json:"fs,omitempty"`
	}{Token: hostToken, FS: hostHasFS}
	if hostShell {
		cfg.Path = hostproto.Path
	}
	if hostAuth {
		// The page is told there IS a token to supply, and not what it is.
		// See desk's panes/hostauth: the injected token is only as private as
		// the page, and the Origin check does not stop a local process.
		cfg.Token, cfg.Auth = "", true
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return htmpl.JS(b) //nolint:gosec // hex from crypto/rand and constants, marshaled by encoding/json
}

// warnAboutHostAccess says what was just turned on, loudly and every time,
// because the whole risk of this is someone leaving it running after they
// stopped thinking about it.
func warnAboutHostAccess() {
	if hostShell {
		fmt.Printf("chaosrack: --shell is ON: this page can run commands on this machine as you.\n")
	}
	if hostFS {
		scope := "your whole filesystem"
		if hostFSRoot != "" {
			scope = hostFSRoot
		}
		fmt.Printf("chaosrack: --fs is ON: this page can read and write %s.\n", scope)
	}
	fmt.Printf("chaosrack:   guarded by a per-run token and an Origin check; stop the server to revoke both.\n")
	if !hostAuth {
		fmt.Printf("chaosrack:   the token is in the served page. On a machine with other users on it,\n")
		fmt.Printf("chaosrack:   add --auth so it is printed here instead — they can read the page.\n")
	}
}
