//go:build !js

package racklink

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/net/websocket"
)

// SessionConn is a WebTransport session's one stream, as a Conn. Closing it
// ends the session.
func SessionConn(sess *webtransport.Session, str *webtransport.Stream) Conn {
	return LineConn(sessionStream{str: str, sess: sess})
}

// clientSession is a terminal's session, whose transport goes with it.
func clientSession(tr *webtransport.Transport, sess *webtransport.Session, str *webtransport.Stream) Conn {
	return LineConn(sessionStream{str: str, sess: sess, tr: tr})
}

type sessionStream struct {
	str  *webtransport.Stream
	sess *webtransport.Session
	tr   *webtransport.Transport // a client's own, closed with it; nil on the server
}

func (s sessionStream) Read(p []byte) (int, error)  { return s.str.Read(p) }
func (s sessionStream) Write(p []byte) (int, error) { return s.str.Write(p) }
func (s sessionStream) Close() error {
	_ = s.str.Close() //nolint:errcheck // the session's close below is what ends it
	err := s.sess.CloseWithError(0, "")
	if s.tr != nil {
		_ = s.tr.Close() //nolint:errcheck // the session's error is the one to report
	}
	return err
}

// WSConn is a WebSocket as a Conn: one text message a line.
func WSConn(ws *websocket.Conn) Conn { return wsConn{ws} }

type wsConn struct{ ws *websocket.Conn }

func (c wsConn) Read() ([]byte, error) {
	var s string
	if err := websocket.Message.Receive(c.ws, &s); err != nil {
		return nil, err
	}
	return []byte(s), nil
}

func (c wsConn) Write(b []byte) error { return websocket.Message.Send(c.ws, string(b)) }
func (c wsConn) Close() error         { return c.ws.Close() }

// Transports a terminal can ask for.
const (
	ViaAuto = "auto" // WebTransport, and the WebSocket if it cannot be had
	ViaWT   = "wt"
	ViaWS   = "ws"
)

// Dial connects a terminal to the server at base (http://host:port) over
// via. Auto tries WebTransport and falls back to the WebSocket, saying why
// in the error only when both fail.
func Dial(ctx context.Context, base, via string) (*Client, error) {
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	info, err := fetchInfo(ctx, base)
	if err != nil {
		return nil, err
	}
	var wtErr error
	if via != ViaWS {
		if info.WT == "" {
			wtErr = errors.New("the server offers no WebTransport")
		} else {
			c, err := dialWT(ctx, info)
			if err == nil {
				return c, nil
			}
			wtErr = err
		}
		if via == ViaWT {
			return nil, fmt.Errorf("WebTransport: %w", wtErr)
		}
	}
	c, err := dialWS(base)
	if err != nil {
		if wtErr != nil {
			return nil, fmt.Errorf("WebTransport: %v; WebSocket: %w", wtErr, err)
		}
		return nil, fmt.Errorf("WebSocket: %w", err)
	}
	return c, nil
}

func fetchInfo(ctx context.Context, base string) (Info, error) {
	var info Info
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+PathInfo, http.NoBody)
	if err != nil {
		return info, err
	}
	rsp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return info, fmt.Errorf("reaching %s: %w (is chaosrack running there?)", base, err)
	}
	defer rsp.Body.Close() //nolint:errcheck // read-only
	if rsp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("%s%s answered %s: this server has no rack link (an older chaosrack, or --rack-link=false)", base, PathInfo, rsp.Status)
	}
	err = json.NewDecoder(rsp.Body).Decode(&info)
	return info, err
}

// dialWT opens a session to the terminal path and its one stream, trusting
// the certificate whose fingerprint the server published and nothing else —
// what a browser's serverCertificateHashes does.
func dialWT(ctx context.Context, info Info) (*Client, error) {
	want, err := base64.RawStdEncoding.DecodeString(info.CertHash)
	if err != nil || len(want) != sha256.Size {
		return nil, errors.New("the server published no usable certificate hash")
	}
	tr := &webtransport.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // replaced by the fingerprint check, which is stricter
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{http3.NextProtoH3},
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("no certificate presented")
			}
			if sum := sha256.Sum256(state.PeerCertificates[0].Raw); string(sum[:]) != string(want) {
				return errors.New("the certificate is not the one the server published")
			}
			return nil
		},
	}}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rsp, sess, err := tr.Dial(dctx, info.WT+PathTerminal, nil)
	if err != nil {
		_ = tr.Close() //nolint:errcheck // nothing was opened
		return nil, err
	}
	if rsp.StatusCode != http.StatusOK {
		_ = tr.Close() //nolint:errcheck // nothing was opened
		return nil, fmt.Errorf("the server refused the session: %s", rsp.Status)
	}
	str, err := sess.OpenStreamSync(dctx)
	if err != nil {
		_ = sess.CloseWithError(0, "") //nolint:errcheck // giving up on it
		_ = tr.Close()                 //nolint:errcheck // and on the transport
		return nil, err
	}
	return NewClient(clientSession(tr, sess, str), ViaWT), nil
}

func dialWS(base string) (*Client, error) {
	u := "ws" + strings.TrimPrefix(base, "http") + PathTerminal + WSSuffix
	// The Origin is the server's own, which is what the server lets in: a
	// page from anywhere else in the same browser is turned away.
	ws, err := websocket.Dial(u, "", base)
	if err != nil {
		return nil, err
	}
	return NewClient(WSConn(ws), ViaWS), nil
}
