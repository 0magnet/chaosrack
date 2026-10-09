package racklink

import (
	"encoding/json"
	"errors"
	"sync"
)

// Hub is the server's half: it holds the pages that offer the rack and
// passes the terminals' calls to the newest of them.
//
// The newest because that is the tab just opened, or just reloaded, which is
// the one being looked at; an older tab stays connected and takes over when
// the newer one goes.
type Hub struct {
	mu      sync.Mutex
	pages   []*page
	next    int64
	waiting map[int64]asker
}

type page struct{ c Conn }

// asker is who a renumbered call came from, and what they called it.
type asker struct {
	term *term
	id   int64
	page *page
}

type term struct {
	c Conn
	// gone is closed when the terminal disconnects, so an answer arriving
	// after it went is dropped rather than written to a closed line.
	gone chan struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{waiting: map[int64]asker{}} }

// Pages is how many pages are offering the rack.
func (h *Hub) Pages() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pages)
}

// ServePage takes a page's connection and serves it until it closes: the
// page reads requests and writes answers.
func (h *Hub) ServePage(c Conn) {
	p := &page{c: c}
	h.mu.Lock()
	h.pages = append(h.pages, p)
	h.mu.Unlock()
	defer h.dropPage(p)
	for {
		b, err := c.Read()
		if err != nil {
			return
		}
		var r Response
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		h.mu.Lock()
		a, ok := h.waiting[r.ID]
		delete(h.waiting, r.ID)
		h.mu.Unlock()
		if !ok {
			continue
		}
		r.ID = a.id
		a.term.send(r)
	}
}

// dropPage takes a page away, and answers what it was asked with an error:
// a terminal waiting on a tab that was closed should hear so, not hang.
func (h *Hub) dropPage(p *page) {
	h.mu.Lock()
	for i, q := range h.pages {
		if q == p {
			h.pages = append(h.pages[:i], h.pages[i+1:]...)
			break
		}
	}
	var lost []asker
	for id, a := range h.waiting {
		if a.page == p {
			lost = append(lost, a)
			delete(h.waiting, id)
		}
	}
	h.mu.Unlock()
	_ = p.c.Close() //nolint:errcheck // it is going either way
	for _, a := range lost {
		a.term.send(Response{ID: a.id, Err: "the page closed"})
	}
}

// ErrNoPage is the answer when no page is offering the rack.
var ErrNoPage = errors.New("no chaosrack page is open on this server; open one in a browser")

// ServeTerminal takes a terminal's connection and serves it until it closes.
func (h *Hub) ServeTerminal(c Conn) {
	t := &term{c: c, gone: make(chan struct{})}
	defer close(t.gone)
	defer h.forget(t)
	for {
		b, err := c.Read()
		if err != nil {
			return
		}
		var r Request
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		h.mu.Lock()
		if len(h.pages) == 0 {
			h.mu.Unlock()
			t.send(Response{ID: r.ID, Err: ErrNoPage.Error()})
			continue
		}
		p := h.pages[len(h.pages)-1]
		h.next++
		n := h.next
		h.waiting[n] = asker{term: t, id: r.ID, page: p}
		h.mu.Unlock()
		asked := r.ID
		r.ID = n
		if err := p.c.Write(encode(r)); err != nil {
			h.mu.Lock()
			delete(h.waiting, n)
			h.mu.Unlock()
			t.send(Response{ID: asked, Err: "the page did not take the call: " + err.Error()})
		}
	}
}

// forget drops what a departed terminal was waiting for.
func (h *Hub) forget(t *term) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, a := range h.waiting {
		if a.term == t {
			delete(h.waiting, id)
		}
	}
}

func (t *term) send(r Response) {
	select {
	case <-t.gone:
		return
	default:
	}
	_ = t.c.Write(encode(r)) //nolint:errcheck // a terminal that cannot be written to is one that has gone; its read loop ends it
}
