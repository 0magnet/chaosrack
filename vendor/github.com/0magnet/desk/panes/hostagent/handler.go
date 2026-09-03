//go:build !js

package hostagent

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

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
	if q := ws.Request().URL.Query(); q != nil {
		if n, err := strconv.Atoi(q.Get(hostproto.ColsParam)); err == nil && n > 0 && n <= maxGrid {
			sess.Cols = n
		}
		if n, err := strconv.Atoi(q.Get(hostproto.RowsParam)); err == nil && n > 0 && n <= maxGrid {
			sess.Rows = n
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
		// Closing here is what unblocks the Receive below. Without it a
		// session whose shell exited would sit with a live socket and a
		// dead pty until the tab closed.
		_ = ws.Close() //nolint:errcheck // closing to unblock the read; the error is the close itself
	}()

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
