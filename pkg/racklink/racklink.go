// Package racklink is the rack's own line to a terminal: a page connects to
// the server that served it, a terminal panel connects to the same server,
// and the server passes the panel's calls to the page and the answers back.
//
// It is what `chaosrack tui` uses instead of a browser's debugging port. The
// cable (internal/rackcable) reached into the page over the Chrome DevTools
// Protocol, which needs the browser started with --remote-debugging-port —
// a flag nobody runs their everyday browser with, and a door into every tab
// they have open. Here the page itself offers the rack, to its own server,
// and nothing else is opened.
//
// THE CALLS are the panel's Source, one request and one answer each:
// modules, controls, set, and the picture's three (picture, changes, act).
// Each message is one JSON object on a line of its own: a request
// {"id":1,"op":"set","args":{...}} and its answer {"id":1,"ok":...} or
// {"id":1,"err":"..."}. The ids are the asker's; the hub renumbers what it
// passes on, so two terminals on one page cannot answer each other.
//
// THE TRANSPORTS are WebTransport, preferred, and a WebSocket. The page tries
// WebTransport first and falls back by itself; a terminal does the same, and
// --via chooses one. Both carry the same lines: on WebTransport one
// bidirectional stream, on a WebSocket one text message a line.
package racklink

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Request is one call.
type Request struct {
	ID   int64           `json:"id"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Response answers the request with the same ID.
type Response struct {
	ID  int64           `json:"id"`
	OK  json.RawMessage `json:"ok,omitempty"`
	Err string          `json:"err,omitempty"`
}

// The operations.
const (
	OpModules  = "modules"
	OpControls = "controls"
	OpSet      = "set"
	OpPicture  = "picture"
	OpChanges  = "changes"
	OpAct      = "act"
	OpScene    = "scene"
	OpSceneAct = "sceneact"
	OpCanvases = "canvases"
)

// Conn is a connection carrying whole messages.
type Conn interface {
	Read() ([]byte, error)
	Write([]byte) error
	Close() error
}

// maxLine is the longest message: a whole picture of the rack is about
// 400 kB, and a rack twice the size should not be what breaks the line.
const maxLine = 16 << 20

// LineConn carries messages over a byte stream, one a line: a WebTransport
// stream, or a pipe in a test.
func LineConn(rwc io.ReadWriteCloser) Conn {
	r := bufio.NewReaderSize(rwc, 64<<10)
	return &lineConn{rwc: rwc, r: r}
}

type lineConn struct {
	rwc io.ReadWriteCloser
	r   *bufio.Reader
	wmu sync.Mutex
}

func (c *lineConn) Read() ([]byte, error) {
	var line []byte
	for {
		part, isPrefix, err := c.r.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, part...)
		if len(line) > maxLine {
			return nil, errors.New("racklink: a message longer than the limit")
		}
		if !isPrefix {
			return line, nil
		}
	}
}

func (c *lineConn) Write(b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.rwc.Write(append(b[:len(b):len(b)], '\n')); err != nil {
		return err
	}
	return nil
}

func (c *lineConn) Close() error { return c.rwc.Close() }

// encode writes a message of this package's own. What they carry was
// encoded already (RawMessage, checked) or is plain strings and numbers.
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"err":"racklink: a message that could not be written"}`)
	}
	return b
}

// Where the link is served. The page and a terminal take different ends of
// it, so each has its own path; the WebSocket's is the same path under /ws.
const (
	PathPage     = "/rack/page"
	PathTerminal = "/rack/term"
	PathInfo     = "/rack/info"
	WSSuffix     = "/ws"
)

// Info is what PathInfo answers: where the WebTransport sessions are opened,
// and the fingerprint that makes its generated certificate acceptable. WT is
// empty when the server offers no WebTransport.
type Info struct {
	WT       string `json:"wt,omitempty"`
	CertHash string `json:"certHash,omitempty"`
}

// PageScript is the page's end of the link (page.js): it defines
// window.__racklink, whose start takes the page's answering function.
//
//go:embed page.js
var PageScript string
