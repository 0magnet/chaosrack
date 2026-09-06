//go:build !js

package hostagent

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/net/websocket"

	"github.com/0magnet/desk/panes/hostproto"
)

// maxGrid bounds a resize.
//
// Not because a bigger terminal would be wrong but because the number arrives
// over the wire and ends up in a uint16 in a TIOCSWINSZ: 65536 columns is zero
// columns after the conversion, and a shell told it has no columns behaves
// strangely rather than loudly. Anything past this is a mistake or a probe.
const maxGrid = 1000

// Config is how the agent is allowed to be reached, and what it starts.
type Config struct {
	// Token must be presented as a query parameter. An empty token means the
	// agent refuses every request — starting wide open because a field was
	// left unset is exactly the failure this must not have.
	Token string

	// Origins are the exact Origin header values that may connect. Empty
	// refuses every browser request, for the same reason.
	//
	// The Origin check is what actually stops a hostile page: a browser sets
	// this header itself on the WebSocket handshake and script cannot change
	// it, so a page at evil.example cannot pretend to be the served desk no
	// matter what it knows.
	Origins []string

	// Session is what to start for each connection.
	Session SessionConfig

	// Sessions, when non-nil, lets a client name a session that outlives the
	// socket carrying it: closing the window and opening it again gets the
	// same shell back, with what it printed in between replayed into it.
	//
	// OPT-IN, and nil is not merely the default but a different code path.
	// The shell-dies-with-the-window model is the one with no timers, no
	// registry and nothing to reason about — closing the tab revokes the
	// shell — and an agent that did not ask for anything else keeps exactly
	// that behavior, unchanged, with none of session.go reachable from it.
	//
	// Even with a registry, a client that sends no session id gets the old
	// behavior for the same reason: naming a session is how a client says it
	// intends to come back, and a client that says nothing is asking for a
	// shell for this window only.
	Sessions *Registry
}

// NewToken returns a fresh 128-bit token.
//
// Per run and never written to disk: the window in which one is worth stealing
// is the lifetime of the process that printed it, and a token in a file is one
// that outlives the decision to allow access.
func NewToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("hostagent: generating a token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Handler serves the pty endpoint.
//
// The two checks are deliberately in different places. The token is checked in
// plain HTTP, before any upgrade, so a caller with the wrong one gets a 403 and
// not a WebSocket that closes for reasons it cannot see. The origin is checked
// in the handshake, which is where the WebSocket library already looks and
// where returning an error refuses the upgrade.
func (c Config) Handler() http.Handler {
	srv := &websocket.Server{
		Handshake: func(_ *websocket.Config, r *http.Request) error {
			return c.checkOrigin(r)
		},
		Handler: c.serve,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.checkToken(r) {
			// No detail. A response that distinguishes "wrong token" from
			// "no token" from "agent not enabled" is a response that helps
			// enumerate, and there is nothing a legitimate caller learns
			// from it that it did not already have.
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		srv.ServeHTTP(w, r)
	})
}

func (c Config) checkToken(r *http.Request) bool {
	if c.Token == "" {
		return false
	}
	got := r.URL.Query().Get(hostproto.TokenParam)
	// Constant time because the comparison is against a secret and the
	// endpoint can be hit in a loop. Length is compared first by the
	// function itself, which leaks only the length.
	return subtle.ConstantTimeCompare([]byte(got), []byte(c.Token)) == 1
}

// checkBrowserOrigin is the guard for ordinary HTTP requests, as opposed to the
// WebSocket handshake.
//
// THE ORIGIN HEADER IS NOT ENOUGH HERE, and finding that out cost a confusing
// 403: a browser does NOT send Origin on a same-origin GET. It is a CORS
// header, so the request that most needs to be allowed — the page fetching from
// the very server that served it — arrives with no Origin at all, and a check
// that requires one refuses exactly the traffic it exists to permit.
//
// Sec-Fetch-Site is the header that actually answers the question being asked.
// The browser sets it on every request and page script cannot change it, so
// "same-origin" is a statement by the browser that this came from the served
// page. That makes it a STRONGER guard than Origin, not a weaker fallback.
//
// A request without it is not from a modern browser — it is curl, or a test, or
// something else local. Those fall back to the Origin check, and the token is
// what stands between them and the filesystem either way.
func (c Config) checkBrowserOrigin(r *http.Request) error {
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "same-origin":
		return nil
	case "":
		return c.checkOrigin(r)
	default:
		// cross-site, same-site and none all mean this did not come from the
		// page the agent served.
		return fmt.Errorf("hostagent: refusing a %s request", site)
	}
}

func (c Config) checkOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	for _, ok := range c.Origins {
		if origin == ok {
			return nil
		}
	}
	return fmt.Errorf("hostagent: refusing origin %q", origin)
}

// serve runs one session for the life of one socket.
func (c Config) serve(ws *websocket.Conn) {
	defer ws.Close() //nolint:errcheck // nothing useful to do with it

	// The grid is taken from the handshake rather than left to an immediate
	// resize, because a shell reads the window size once, when it draws its
	// first prompt. Connecting at 80x24 and correcting a frame later leaves a
	// prompt wrapped at the wrong width that nothing will redraw.
	sess := c.Session
	q := ws.Request().URL.Query()
	if n, err := strconv.Atoi(q.Get(hostproto.ColsParam)); err == nil && n > 0 && n <= maxGrid {
		sess.Cols = n
	}
	if n, err := strconv.Atoi(q.Get(hostproto.RowsParam)); err == nil && n > 0 && n <= maxGrid {
		sess.Rows = n
	}

	// A named session is the client asking for one that survives this socket,
	// and it is the only thing that diverts from the path below. Everything
	// after this point is what the agent has always done, untouched: start a
	// shell, pump it at the socket, and close it when the socket ends.
	if c.Sessions != nil {
		if id := q.Get(hostproto.SessionParam); id != "" {
			c.serveDurable(ws, id, sess)
			return
		}
	}

	s, err := Start(sess)
	if err != nil {
		log.Printf("desk: %v", err)
		// Reported into the terminal rather than only to the log, because
		// the person who needs it is looking at the window, not at the
		// process they started three hours ago.
		//nolint:errcheck // the socket is about to close; there is nowhere else to report this
		_ = websocket.Message.Send(ws, []byte("\r\n\x1b[31mcould not start a shell: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer s.Close() //nolint:errcheck

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				if err := websocket.Message.Send(ws, buf[:n]); err != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		// Closing here is what unblocks readClient below. Without it a
		// session whose shell exited would sit with a live socket and a
		// dead pty until the tab closed.
		_ = ws.Close() //nolint:errcheck // closing to unblock the read; the error is the close itself
	}()

	readClient(ws, s)
}

// shell is the half of a session the client-to-server loop needs.
//
// It exists so that the loop below can be written once and used by both a
// Session, which dies with its socket, and an Attachment, which does not. The
// alternative was a second copy of the switch, and a second copy is where the
// two paths quietly stop agreeing about what a resize means.
type shell interface {
	io.Writer
	Resize(cols, rows int) error
}

// readClient runs the client-to-server loop until the socket ends.
func readClient(ws *websocket.Conn, s shell) {
	for {
		var raw string
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			return
		}
		var m hostproto.Msg
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue // a frame we cannot read is not a reason to drop a shell
		}
		switch m.T {
		case hostproto.TypeInput:
			if _, err := s.Write([]byte(m.D)); err != nil {
				return
			}
		case hostproto.TypeBinary:
			raw, err := base64.StdEncoding.DecodeString(m.D)
			if err != nil {
				continue // as with unparseable JSON: not worth a shell
			}
			if _, err := s.Write(raw); err != nil {
				return
			}
		case hostproto.TypeResize:
			if m.C > 0 && m.R > 0 && m.C <= maxGrid && m.R <= maxGrid {
				_ = s.Resize(m.C, m.R) //nolint:errcheck // a refused resize is not worth dropping a shell
			}
		}
	}
}

// serveDurable attaches this socket to a named session instead of starting one
// that dies with it.
//
// The difference from serve, in full: nothing here reads the pty. The session's
// own pump does that for its whole life, which is what lets it keep running
// while nobody is watching — see session.go, where that is the point rather
// than an optimization. This function replays what was missed, applies the new
// window's size, and then runs the same client-to-server loop as the ordinary
// path.
func (c Config) serveDurable(ws *websocket.Conn, id string, sess SessionConfig) {
	sink := &wsSink{ws: ws, timeout: c.Sessions.cfg.StallTimeout}
	at, err := c.Sessions.Attach(c.Token, id, sess, sink, func() {
		_ = ws.Close() //nolint:errcheck // closing to unblock the loop; the error is the close itself
	})
	if err != nil {
		log.Printf("desk: %v", err)
		// Into the terminal, for the same reason serve does it: the person
		// who needs to know is looking at the window. The two errors a
		// client can provoke here — the cap and an absurd id — are both
		// things it can fix, and neither says anything about what sessions
		// exist.
		//nolint:errcheck // the socket is about to close; there is nowhere else to report this
		_ = websocket.Message.Send(ws, []byte("\r\n\x1b[31m"+err.Error()+"\x1b[0m\r\n"))
		return
	}
	// Detach and NOT Close: the socket ending is the whole event this
	// feature exists to survive. What ends the session is the idle timeout,
	// the shell exiting, or the agent stopping.
	defer at.Detach()

	// Attach has already written the transcript through the sink. It has to
	// be Attach that does it — writing it from here would race the session's
	// own delivery of live output onto this same socket, and a WebSocket
	// tolerates exactly one writer at a time.

	// REPLAY FIRST, THEN RESIZE, and the order is not arbitrary. A resize is
	// a SIGWINCH, and a full-screen program answering one redraws itself
	// immediately; doing that before the transcript went out puts the redraw
	// underneath it, so the window ends up showing history below the live
	// screen. This way the redraw is the last thing written, which is where
	// the cursor should be. The replayed text keeps the wrapping it had at
	// the old width, which is wrong and unavoidable — nothing can reflow
	// bytes that were already wrapped by the program that emitted them.
	if !at.Created && sess.Cols > 0 && sess.Rows > 0 {
		_ = at.Resize(sess.Cols, sess.Rows) //nolint:errcheck // a refused resize is not worth dropping a shell
	}

	readClient(ws, at)
}

// wsSink is how a session writes to the client that is currently attached.
//
// THE DEADLINE IS THE REASON THIS TYPE EXISTS. A socket whose far end has gone
// away without saying so — a suspended laptop, a cable pulled — does not fail
// on write, it blocks, for as long as the kernel is willing to retransmit.
// Because the session's pump is what drains the pty, a block here is a stalled
// shell: the build that reconnection was supposed to protect stops making
// progress because a browser that no longer exists has not been noticed. With
// a deadline the write fails, the client is detached, and the output goes to
// the ring buffer instead — which is exactly where it would have gone if the
// socket had closed politely.
//
// The non-reconnecting path deliberately does not use this. There, a wedged
// socket wedges only its own shell, which dies with it anyway, and adding a
// deadline would change behavior that nobody asked to have changed.
type wsSink struct {
	ws      *websocket.Conn
	timeout time.Duration
}

func (s *wsSink) Write(p []byte) (int, error) {
	if s.timeout > 0 {
		// Ignored deliberately: the only way this fails is a closed
		// connection, which the Send below is about to report properly.
		_ = s.ws.SetWriteDeadline(time.Now().Add(s.timeout)) //nolint:errcheck // reported by the Send
	}
	if err := websocket.Message.Send(s.ws, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
