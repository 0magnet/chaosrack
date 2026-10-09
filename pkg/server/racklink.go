//go:build !js

package server

import (
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/net/websocket"

	"github.com/0magnet/chaosrack/pkg/racklink"
)

// The rack's link (pkg/racklink): a page this server served offers its rack
// here, and `chaosrack tui` drives it from here, with no browser debugging
// port anywhere.
//
// Who may use it:
//   - a page, from this server's own origin only;
//   - a terminal, from this machine only, unless --rack-link-remote;
//   - nothing from another site's origin, ever: a page elsewhere in the same
//     browser could otherwise open a socket to 127.0.0.1 and play the rack.

var (
	rackLinkOn     bool
	rackLinkRemote bool
)

func init() {
	runCmd.Flags().BoolVar(&rackLinkOn, "rack-link", true, "let `chaosrack tui` reach the pages this server serves, over WebTransport or a WebSocket")
	runCmd.Flags().BoolVar(&rackLinkRemote, "rack-link-remote", false, "let terminals on other machines use the rack link (default: this machine only)")
}

// mountRackLink serves both ends of the link and where to find them.
func mountRackLink(r *gin.Engine) {
	if !rackLinkOn {
		return
	}
	hub := racklink.NewHub()
	wt := mountWebTransport(r, false)

	r.GET(racklink.PathInfo, func(c *gin.Context) {
		var info racklink.Info
		if wt != nil {
			u, _ := url.Parse(wt.URL(c.Request.Host, "")) //nolint:errcheck // made by URL, it parses
			info.WT = u.Scheme + "://" + u.Host
			info.CertHash = wt.Cert().Base64()
		}
		c.JSON(http.StatusOK, info)
	})

	page := func(c racklink.Conn) { hub.ServePage(c) }
	term := func(c racklink.Conn) { hub.ServeTerminal(c) }

	r.GET(racklink.PathPage+racklink.WSSuffix, wsLink(page, false))
	r.GET(racklink.PathTerminal+racklink.WSSuffix, wsLink(term, true))
	if wt != nil {
		wt.Handle(racklink.PathPage, wtLink(page, false))
		wt.Handle(racklink.PathTerminal, wtLink(term, true))
	}
	log.Printf("chaosrack: rack link on %s (WebTransport %v, WebSocket); `chaosrack tui` uses it", racklink.PathInfo, wt != nil)
}

// wsLink is one end of the link on a WebSocket.
func wsLink(serve func(racklink.Conn), terminal bool) gin.HandlerFunc {
	s := websocket.Server{
		Handshake: func(_ *websocket.Config, req *http.Request) error {
			if !linkAllowed(req, terminal) {
				return errLinkRefused
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			// Text frames: the lines are JSON.
			ws.PayloadType = websocket.TextFrame
			serve(racklink.WSConn(ws))
		},
	}
	return gin.WrapH(s)
}

// wtLink is one end of the link on WebTransport: the session's first stream,
// opened by whoever dialed.
func wtLink(serve func(racklink.Conn), terminal bool) func(*webtransport.Session, *http.Request) {
	return func(sess *webtransport.Session, req *http.Request) {
		if !linkAllowed(req, terminal) {
			return
		}
		str, err := sess.AcceptStream(sess.Context())
		if err != nil {
			return
		}
		serve(racklink.SessionConn(sess, str))
	}
}

type linkError string

func (e linkError) Error() string { return string(e) }

const errLinkRefused = linkError("the rack link refuses this origin or address")

// linkAllowed is the rule at the top of this file.
func linkAllowed(req *http.Request, terminal bool) bool {
	if o := req.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || u.Hostname() != hostOf(req.Host) {
			return false
		}
	}
	if terminal && !rackLinkRemote {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return false
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	return true
}

func hostOf(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}
