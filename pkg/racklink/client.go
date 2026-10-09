package racklink

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// Client is a terminal's end: a racktui.Source, and a Pictured one, whose
// every call goes over the link to the page.
type Client struct {
	c       Conn
	mu      sync.Mutex
	next    int64
	waiting map[int64]chan Response
	err     error // why the line went, once it has
	// Via names the transport, for the panel to say which it is on.
	Via string
}

// callTimeout is how long a call waits for the page. A whole picture of the
// rack takes the page about a third of a second to draw up; ten seconds is a
// page that is not answering.
const callTimeout = 10 * time.Second

// NewClient starts a client on a connection to a hub's terminal side.
func NewClient(c Conn, via string) *Client {
	cl := &Client{c: c, waiting: map[int64]chan Response{}, Via: via}
	go cl.read()
	return cl
}

func (cl *Client) read() {
	for {
		b, err := cl.c.Read()
		if err != nil {
			cl.mu.Lock()
			cl.err = fmt.Errorf("the link to the server closed: %w", err)
			for id, ch := range cl.waiting {
				ch <- Response{ID: id, Err: cl.err.Error()}
				delete(cl.waiting, id)
			}
			cl.mu.Unlock()
			return
		}
		var r Response
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		cl.mu.Lock()
		ch, ok := cl.waiting[r.ID]
		delete(cl.waiting, r.ID)
		cl.mu.Unlock()
		if ok {
			ch <- r
		}
	}
}

// Close ends the link.
func (cl *Client) Close() error { return cl.c.Close() }

// call sends op and waits for its answer, decoded into out (when not nil).
func (cl *Client) call(op string, args, out any) error {
	var raw json.RawMessage
	if args != nil {
		raw = encode(args)
	}
	ch := make(chan Response, 1)
	cl.mu.Lock()
	if cl.err != nil {
		cl.mu.Unlock()
		return cl.err
	}
	cl.next++
	id := cl.next
	cl.waiting[id] = ch
	cl.mu.Unlock()
	if err := cl.c.Write(encode(Request{ID: id, Op: op, Args: raw})); err != nil {
		cl.mu.Lock()
		delete(cl.waiting, id)
		cl.mu.Unlock()
		return err
	}
	select {
	case r := <-ch:
		if r.Err != "" {
			return errors.New(r.Err)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(r.OK, out)
	case <-time.After(callTimeout):
		cl.mu.Lock()
		delete(cl.waiting, id)
		cl.mu.Unlock()
		return fmt.Errorf("the page did not answer %s in %s", op, callTimeout)
	}
}

// modulesAnswer is how the modules call answers.
type modulesAnswer struct {
	Mods  []racksurface.Item `json:"mods"`
	Slots int                `json:"slots"`
}

// Modules is the rack's modules.
func (cl *Client) Modules() ([]racksurface.Item, int, error) {
	var a modulesAnswer
	err := cl.call(OpModules, nil, &a)
	return a.Mods, a.Slots, err
}

// Controls is every control with its value.
func (cl *Client) Controls() ([]racktui.Control, error) {
	var cs []racktui.Control
	err := cl.call(OpControls, nil, &cs)
	return cs, err
}

type setArgs struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// Set moves one control.
func (cl *Client) Set(id, value string) error {
	return cl.call(OpSet, setArgs{ID: id, Value: value}, nil)
}

// Picture is the panel as the page draws it.
func (cl *Client) Picture() (*rackpic.Picture, error) {
	var p rackpic.Picture
	if err := cl.call(OpPicture, nil, &p); err != nil {
		return nil, err
	}
	if p.Err != "" {
		return nil, errors.New(p.Err)
	}
	return &p, nil
}

type changesArgs struct {
	Gen int `json:"gen"`
}

// PictureChanges is what changed since the picture of generation gen.
func (cl *Client) PictureChanges(gen int) (*rackpic.Patch, error) {
	var d rackpic.Patch
	if err := cl.call(OpChanges, changesArgs{Gen: gen}, &d); err != nil {
		return nil, err
	}
	if d.Err != "" {
		return nil, errors.New(d.Err)
	}
	return &d, nil
}

type actArgs struct {
	Gen   int     `json:"gen"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Kind  string  `json:"kind"`
	Delta float64 `json:"delta"`
}

// Act does what the pointer did, on the page.
func (cl *Client) Act(gen int, x, y float64, kind string, delta float64) error {
	var r struct {
		Err string `json:"err"`
	}
	if err := cl.call(OpAct, actArgs{Gen: gen, X: x, Y: y, Kind: kind, Delta: delta}, &r); err != nil {
		return err
	}
	if r.Err != "" {
		return errors.New(r.Err)
	}
	return nil
}

// ── the page's end ───────────────────────────────────────────────────────

// PageRack is what a page offers: the rack's Source, and its picture as the
// JSON the picture script already writes, which is passed on as it is rather
// than decoded and encoded again in a page that would rather be drawing.
type PageRack interface {
	racktui.Source
	PictureJSON() string
	ChangesJSON(gen int) string
	ActJSON(gen int, x, y float64, kind string, delta float64) string
	SceneJSON(w, h int, shape float64) string
	CanvasesJSON(gen int, want []rackpic.CanvasWant) string
}

// Answer is the page's half of one call: the request in, the answer out.
func Answer(rack PageRack, req []byte) []byte {
	var r Request
	if err := json.Unmarshal(req, &r); err != nil {
		return encode(Response{Err: "racklink: an unreadable request"})
	}
	ok, err := answer(rack, r)
	if err != nil {
		return encode(Response{ID: r.ID, Err: err.Error()})
	}
	return encode(Response{ID: r.ID, OK: ok})
}

func answer(rack PageRack, r Request) (json.RawMessage, error) {
	switch r.Op {
	case OpModules:
		mods, slots, err := rack.Modules()
		if err != nil {
			return nil, err
		}
		return json.Marshal(modulesAnswer{Mods: mods, Slots: slots})
	case OpControls:
		cs, err := rack.Controls()
		if err != nil {
			return nil, err
		}
		return json.Marshal(cs)
	case OpSet:
		var a setArgs
		if err := json.Unmarshal(r.Args, &a); err != nil {
			return nil, err
		}
		return nil, rack.Set(a.ID, a.Value)
	case OpPicture:
		return raw(rack.PictureJSON())
	case OpChanges:
		var a changesArgs
		if err := json.Unmarshal(r.Args, &a); err != nil {
			return nil, err
		}
		return raw(rack.ChangesJSON(a.Gen))
	case OpAct:
		var a actArgs
		if err := json.Unmarshal(r.Args, &a); err != nil {
			return nil, err
		}
		return raw(rack.ActJSON(a.Gen, a.X, a.Y, a.Kind, a.Delta))
	case OpScene:
		var a sceneArgs
		if err := json.Unmarshal(r.Args, &a); err != nil {
			return nil, err
		}
		return raw(rack.SceneJSON(a.W, a.H, a.Shape))
	case OpCanvases:
		var a canvasesArgs
		if err := json.Unmarshal(r.Args, &a); err != nil {
			return nil, err
		}
		return raw(rack.CanvasesJSON(a.Gen, a.Want))
	}
	return nil, fmt.Errorf("racklink: no operation %q", r.Op)
}

// raw passes on JSON the page wrote, once it is known to be JSON: an answer
// that is not would break the line for every call after it.
func raw(s string) (json.RawMessage, error) {
	if !json.Valid([]byte(s)) {
		return nil, errors.New("racklink: the page's picture script answered something that is not JSON")
	}
	return json.RawMessage(s), nil
}

type sceneArgs struct {
	W     int     `json:"w"`
	H     int     `json:"h"`
	Shape float64 `json:"shape"`
}

// Scene is the model's canvas.
func (cl *Client) Scene(w, h int, shape float64) (*rackpic.Image, error) {
	var m rackpic.Image
	if err := cl.call(OpScene, sceneArgs{W: w, H: h, Shape: shape}, &m); err != nil {
		return nil, err
	}
	if m.Err != "" {
		return nil, errors.New(m.Err)
	}
	return &m, nil
}

type canvasesArgs struct {
	Gen  int                  `json:"gen"`
	Want []rackpic.CanvasWant `json:"want"`
}

// Canvases is the panel's canvases asked for.
func (cl *Client) Canvases(gen int, want []rackpic.CanvasWant) (map[int]*rackpic.Image, error) {
	var c rackpic.Canvases
	if err := cl.call(OpCanvases, canvasesArgs{Gen: gen, Want: want}, &c); err != nil {
		return nil, err
	}
	if c.Err != "" {
		return nil, errors.New(c.Err)
	}
	return c.Imgs, nil
}
