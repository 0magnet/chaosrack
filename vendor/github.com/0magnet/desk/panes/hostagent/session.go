//go:build !js

package hostagent

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Sessions that outlive the socket that opened them.
//
// The rest of this package is built on the assumption that a session and a
// connection are the same thing: Start when the socket opens, Close when it
// ends. That is the right default and it stays the default — closing the
// window revokes the shell, which is the simplest revocation there is and the
// only one that needs no timer, no registry and no explaining. This file is
// what happens when someone asks for the other model: close the window, open
// it again, and be back where you were, the way tmux and screen work.
//
// # What actually changes, and it is not "the pty stays open"
//
// Keeping the pty open is the easy half. The half that matters is that
// SOMETHING HAS TO KEEP READING IT. A pty master has a fixed kernel buffer of
// a few tens of kilobytes; a shell whose output nobody drains blocks in
// write(2) once it fills, and the long-running build that reconnection exists
// to protect is then frozen at exactly the moment its owner walked away. So a
// registered session owns a goroutine — the pump — that reads the pty for its
// whole life whether or not anyone is attached, feeding a ring buffer and,
// when there is one, the client. Detaching removes the client. It does not
// stop the reading.
//
// # Security, which is the reason most of this file is comments
//
// A detached session is a live shell running as you, sitting on the machine
// with no window attached to it and nothing on screen to remind anybody it
// exists. Before this file, closing the tab took that capability away. Now
// three things do, and they are worth naming because they are the whole
// safety story:
//
//   - stopping the agent, which is unchanged and still absolute: sessions are
//     process memory and a process that exits takes its children's ptys with
//     it (Close kills, see Session.Close);
//   - the idle timeout, which reaps a session nobody has attached to for long
//     enough;
//   - the shell exiting on its own, which the pump notices as EOF and which
//     un-registers the session immediately, so that `exit` in a window still
//     means the shell is gone rather than leaving a corpse to re-attach to.
//
// ADDRESSING IS THE PART THAT COULD GO WRONG QUIETLY, so it is worth being
// exact. The client supplies a session id on the handshake. That id is a NAME
// and never a key: what addresses an entry in the registry is
//
//	HMAC-SHA256(per-run secret, len(token) || token || name)
//
// and the two consequences are both load-bearing.
//
// First, a client that names its session badly cannot thereby create a
// guessable session. We do not control the browser and cannot make it choose
// well — a pane could reasonably use a window number, and a person hand-typing
// a URL will use "1" — but the registry key is a 256-bit function of a secret
// from crypto/rand that never leaves this process, so predicting the name buys
// nothing. Making unguessability a property of the SERVER rather than a rule
// the client is trusted to follow is the only version of this that survives
// contact with a client someone else wrote.
//
// Second, the token is mixed in, which is what binds a session to the
// credential that created it. The alternative considered was the obvious one:
// store the token beside the session and compare it on re-attach. That works
// and it is worse, because it is a check that can be forgotten — a second code
// path that attaches by key and neglects the comparison hands a shell to the
// wrong caller, and nothing about the types makes that mistake visible.
// Deriving the key from the token instead means a session created under one
// token is not "refused" under another, it is NOT ADDRESSABLE: there is no
// code path to forget. Today the agent has exactly one token per run and this
// is therefore invisible; it is written this way so that it stays true if that
// ever stops being so.
//
// The length prefix in front of the token is not decoration. Without it,
// concatenation is ambiguous — token "ab" with name "c" hashes the same bytes
// as token "a" with name "bc" — and an ambiguity in a key derivation is a way
// to reach a session you were not given.
//
// A MISS CREATES RATHER THAN REPORTS. Presenting an id that names no session
// starts a new one; there is no "no such session" answer, and no endpoint that
// lists what exists. That is deliberate: an error would make this endpoint an
// oracle for testing whether a given id is live, which is the one useful thing
// an attacker who already holds the token could learn from it. The cost is
// that a typo silently strands the old session until the idle timeout, which
// is what MaxSessions is for.
//
// MAXSESSIONS IS NOT A TIDINESS SETTING. Before this file the number of live
// shells was bounded by the number of open sockets, and closing them was the
// bound. Detached sessions have no such bound: a caller holding the token can
// name a new id for every request and leave a shell behind each time, and a
// few thousand of those is a fork bomb with extra steps. The cap is what puts
// the old bound back.
//
// What the ring buffer holds is terminal output, which is to say anything the
// shell printed — the output of `env`, a key pasted into a prompt, whatever
// was on screen. It lives in this process's memory, is never written to disk,
// and goes away with the session. It is not more sensitive than the pty it
// came from, but it does mean that output survives the window that showed it,
// which the window's owner may not expect.

// The defaults, and why these numbers.
const (
	// defaultIdleTimeout is how long a session with nobody attached lives.
	//
	// An hour is long enough to cover the cases this feature exists for —
	// closing a laptop lid, a reload, moving to another desk — and short
	// enough that a shell forgotten on a Friday is not still there on
	// Monday. It is deliberately NOT extended by the session producing
	// output: a build that prints all night would then keep itself alive
	// forever, which leaves the timeout unable to do the one thing it is
	// for. The consequence, and it is a real cost rather than a detail, is
	// that a job that runs longer than the timeout dies with the session
	// that was running it. Raise it, or set it negative for the tmux
	// semantics of never reaping, if that trade is the wrong way round for
	// what you are doing.
	defaultIdleTimeout = time.Hour

	// defaultBuffer is how much output is kept for replay.
	//
	// 256 KiB is roughly three thousand lines of eighty columns, which is
	// enough that a make(1) that finished while nobody was watching is
	// still readable end to end, and small enough that the default cap of
	// sessions costs eight megabytes in the worst case — nothing against
	// the shells themselves.
	defaultBuffer = 256 << 10

	// defaultMaxSessions bounds how many shells can be left lying around.
	//
	// Thirty-two is far more windows than anybody opens and far fewer than
	// a loop can create. The number matters much less than the existence of
	// one; see the note on MaxSessions above.
	defaultMaxSessions = 32

	// defaultStallTimeout bounds how long the pump waits on one write to a
	// client before giving up on it.
	//
	// The case is a socket whose far end is gone without having said so —
	// a suspended laptop, a yanked cable — where writes do not fail, they
	// hang, for as long as the kernel's retransmit schedule takes. Because
	// the pump is what drains the pty, a hang there is a stalled shell, so
	// the timeout is not about the client at all: it is about not letting a
	// dead browser wedge a running job. On expiry the client is detached
	// and its output goes to the ring, which is exactly where it would have
	// gone had the socket closed politely.
	defaultStallTimeout = 10 * time.Second
)

// maxSessionIDLen bounds the name a client may present.
//
// Nothing legitimate needs more: the name it is meant to carry is a random
// identifier a pane generated and stored, and 128 bytes is four of those. The
// bound exists so that the handshake cannot be used to make the agent hash
// megabytes per request, and because a name that long is a mistake worth
// refusing loudly rather than hashing obligingly.
const maxSessionIDLen = 128

// RegistryConfig is how long sessions live and how much they remember.
//
// Every field has a usable zero, so a caller that wants reconnection and no
// opinions writes NewRegistry(RegistryConfig{}).
type RegistryConfig struct {
	// IdleTimeout is how long a session with no client attached survives.
	// Zero picks defaultIdleTimeout. NEGATIVE MEANS NEVER, which is tmux's
	// behavior and is a deliberate choice to leave shells on the machine
	// until the agent stops.
	IdleTimeout time.Duration

	// Buffer is how many bytes of recent output are kept for replay. Zero
	// picks defaultBuffer; negative disables replay, which gives a session
	// that survives a reconnect but shows nothing of what it did while
	// nobody was looking.
	Buffer int

	// MaxSessions caps how many sessions may exist at once. Zero picks
	// defaultMaxSessions. It cannot be disabled — see the note above about
	// what removes the old bound.
	MaxSessions int

	// StallTimeout bounds one write to an attached client. Zero picks
	// defaultStallTimeout; negative waits forever, which reproduces the
	// non-reconnecting handler's behavior of letting a wedged socket wedge
	// the shell.
	StallTimeout time.Duration
}

// Registry holds sessions that outlive their connections.
//
// It is created by the caller and handed to Config, rather than being made
// lazily inside Handler, for two reasons that both come down to ownership:
// reconnection is opt-in and a nil registry has to be the exact old behavior
// with no reachable new code, and something has to be able to Close every live
// shell when the server stops, which requires a handle the caller kept.
type Registry struct {
	cfg RegistryConfig

	// secret is what makes a client's session name unguessable as a key. Per
	// run, from crypto/rand, never leaves this process and never appears in
	// anything the agent sends — the same reasoning as NewToken, one level
	// down.
	secret [32]byte

	mu sync.Mutex
	by map[string]*managed
}

// NewRegistry makes a registry. It panics only if the system has no entropy,
// which is not a condition worth returning an error for: the alternative is a
// registry whose keys are predictable, and there is no safe way to continue.
func NewRegistry(cfg RegistryConfig) *Registry {
	r := &Registry{cfg: cfg, by: map[string]*managed{}}
	if _, err := rand.Read(r.secret[:]); err != nil {
		panic(fmt.Sprintf("hostagent: no entropy for a session secret: %v", err))
	}
	if r.cfg.IdleTimeout == 0 {
		r.cfg.IdleTimeout = defaultIdleTimeout
	}
	if r.cfg.Buffer == 0 {
		r.cfg.Buffer = defaultBuffer
	}
	if r.cfg.MaxSessions <= 0 {
		r.cfg.MaxSessions = defaultMaxSessions
	}
	if r.cfg.StallTimeout == 0 {
		r.cfg.StallTimeout = defaultStallTimeout
	}
	return r
}

// key derives the registry key for one token and one client-supplied name.
//
// The whole security argument for this file is in the package comment above;
// what is here is only the mechanics. Note that the result is used as a Go map
// key and the lookup is not constant time — that is fine and is not an
// oversight, because the key is already a 256-bit value derived from a secret
// the caller does not have, and a timing signal about which bucket it landed
// in does not help produce one.
func (r *Registry) key(token, name string) string {
	mac := hmac.New(sha256.New, r.secret[:])
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(token)))
	mac.Write(n[:])          //nolint:errcheck // hash.Hash never errors
	mac.Write([]byte(token)) //nolint:errcheck // hash.Hash never errors
	mac.Write([]byte(name))  //nolint:errcheck // hash.Hash never errors
	return string(mac.Sum(nil))
}

// ErrTooManySessions is what attaching returns when the cap is reached, so the
// handler can say something useful into the terminal instead of dropping a
// socket for no visible reason.
var ErrTooManySessions = errors.New("hostagent: too many detached sessions")

// ErrSessionIDTooLong is the other refusal a client can provoke.
var ErrSessionIDTooLong = errors.New("hostagent: session id is too long")

// Attach connects a client to the named session, starting one if the name does
// not resolve to a live entry.
//
// The Attachment that comes back is a hold on THIS attachment and not on the
// session, which is what makes Detach safe to defer. Between attaching and
// detaching another client may have taken the session over, and a detach that
// did not know which attachment it was ending would tear the new client's
// connection down when the old one finally noticed its socket had closed. The
// generation carried inside makes a late detach a no-op instead.
func (r *Registry) Attach(token, name string, cfg SessionConfig, sink io.Writer, kick func()) (*Attachment, error) {
	if len(name) > maxSessionIDLen {
		return nil, ErrSessionIDTooLong
	}
	k := r.key(token, name)

	var fresh bool
	r.mu.Lock()
	m := r.by[k]
	if m == nil {
		if len(r.by) >= r.cfg.MaxSessions {
			r.mu.Unlock()
			return nil, ErrTooManySessions
		}
		// Started under the registry lock, and forking under a mutex is
		// ugly enough to be worth defending: the alternative is a window
		// in which two handshakes with the same name both find nothing
		// and both start a shell, and the loser's shell is then an orphan
		// nobody can ever reach or reap. Contention here is a handful of
		// windows being opened by one person, so the cost is a few
		// milliseconds nobody is waiting on.
		s, err := Start(cfg)
		if err != nil {
			r.mu.Unlock()
			return nil, err
		}
		m = &managed{
			reg:  r,
			key:  k,
			s:    s,
			ring: newRing(r.cfg.Buffer),
		}
		r.by[k] = m
		fresh = true
		go m.pump()
	}
	r.mu.Unlock()

	at := m.attach(sink, kick)
	at.Created = fresh
	return at, nil
}

// Close ends every session in the registry, which is what the process should
// do on the way out.
//
// Not strictly required — the sessions are children of a process that is
// exiting — but a shell that ignored SIGHUP would otherwise survive the agent
// that started it, and that is precisely the case Session.Close exists to
// handle. Doing it explicitly means the kill happens while there is still
// something around to do it.
func (r *Registry) Close() {
	r.mu.Lock()
	all := make([]*managed, 0, len(r.by))
	for k, m := range r.by {
		all = append(all, m)
		delete(r.by, k)
	}
	r.mu.Unlock()
	// In parallel, because each of these can wait up to the grace period
	// Session.Close gives a shell to honor its SIGHUP before being killed.
	// Serially, a full registry would take that many times longer, and this
	// runs from a signal handler where somebody is watching a terminal and
	// deciding whether Ctrl-C worked.
	var wg sync.WaitGroup
	for _, m := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.shutdown()
		}()
	}
	wg.Wait()
}

// Len reports how many sessions are live. It exists for tests and for anything
// that wants to say so on a status page; nothing in the agent's own behavior
// depends on it.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.by)
}

// forget removes a session from the map if it is still the one registered.
//
// The guard matters: a session that was reaped and a session that exited race
// against a new session started under the same name, and deleting by key alone
// would remove the new one.
func (r *Registry) forget(m *managed) {
	r.mu.Lock()
	if r.by[m.key] == m {
		delete(r.by, m.key)
	}
	r.mu.Unlock()
}

// Attachment is one client's hold on a session.
type Attachment struct {
	m   *managed
	gen uint64

	// Created says this attachment started the session rather than finding
	// it. The handler needs the distinction for one thing: a session it just
	// started already has the client's grid, because Start was given it,
	// whereas one that was found has whatever grid the PREVIOUS window had
	// and needs a resize. Resizing unconditionally would send a SIGWINCH to
	// a shell in the middle of drawing its first prompt, which is exactly
	// the sequence SessionConfig.Cols exists to avoid.
	Created bool
}

// Write sends input to the shell.
func (a *Attachment) Write(p []byte) (int, error) { return a.m.s.Write(p) }

// Resize passes a new grid to the pty.
func (a *Attachment) Resize(cols, rows int) error { return a.m.s.Resize(cols, rows) }

// Detach gives up this client's hold without ending the session.
//
// A no-op if another client has taken over in the meantime; see the note on
// generations in Attach.
func (a *Attachment) Detach() { a.m.detach(a.gen) }

// managed is a session plus everything needed to survive not being watched.
type managed struct {
	reg  *Registry
	key  string
	s    *Session
	ring *ring

	mu   sync.Mutex
	gen  uint64
	sink io.Writer
	kick func()
	stop *time.Timer
	dead bool
}

// attach installs a client, displacing whatever was there.
//
// TAKEOVER RATHER THAN REFUSAL, and the choice is not a coin flip. The
// tempting alternative is to refuse a second attachment on the grounds that
// one shell with two keyboards is a mess — which it is — but consider what
// actually produces a second attachment in practice. It is almost never two
// people; it is one person whose previous socket is dead and whose deadness
// the agent has not noticed yet, because the laptop suspended or the network
// moved and TCP will take minutes to admit it. Refusing means the answer to
// "reopen the window and get your shell back" is "not for another few
// minutes", which is a failure at the one thing this feature exists to do.
// Taking over is also what a person would ask for if asked: `tmux attach -d`
// is the flag everybody ends up using.
//
// What is NOT done here is real sharing — both clients seeing the same output
// and both able to type. That needs output fanned out to several sinks and
// some notion of which of them may write, and two live writers into one pty
// interleave keystrokes in the middle of escape sequences unless somebody
// arbitrates. It is a bigger feature than this one and it does not have to be
// built to make reconnection work.
//
// The replay is taken while the lock is held, in the same critical section
// that installs the sink, and that is not incidental tidiness. deliver puts a
// chunk into the ring and picks up the sink under this same mutex, so the two
// orderings are the only ones possible: either a chunk is in the snapshot and
// went to the client being displaced, or it is not in the snapshot and goes to
// the new one. Snapshotting outside the lock — which is how this was written
// first — admits a third ordering where a chunk lands in the ring, is replayed
// to the new client, and is then ALSO sent to it live, so a reconnect shows
// the last line of output twice.
func (m *managed) attach(sink io.Writer, kick func()) *Attachment {
	m.mu.Lock()
	if m.stop != nil {
		m.stop.Stop()
		m.stop = nil
	}
	old := m.kick
	m.gen++
	m.sink, m.kick = sink, kick
	gen := m.gen
	// THE REPLAY IS WRITTEN HERE, UNDER THE LOCK, and both halves of that
	// are a bug fix rather than a style. Handing the bytes back for the
	// caller to write was the first version, and it puts two goroutines on
	// one socket at once: deliver can pick this sink up the instant the lock
	// is released and start sending live output while the caller is still
	// writing the transcript. A WebSocket connection is not safe for
	// concurrent writes — the frames interleave and the client's parser sees
	// a corrupted stream — so the transcript is written before anything else
	// can be, and deliver's own writes are ordered after it because it
	// cannot observe this sink without first taking the mutex this holds.
	//
	// The cost is that a client which completes a handshake and then reads
	// nothing can hold this lock for as long as its sink is willing to
	// block, and the session's pty goes undrained for that long. Bounded by
	// the sink's own write deadline — see wsSink — and confined to the one
	// session, which is why it is a cost rather than a hole.
	if replay := m.ring.replay(); len(replay) > 0 {
		// The error is deliberately dropped. A client that cannot be
		// written to is a client that is already gone, and the read loop
		// the caller is about to enter will find that out and detach; the
		// session itself is not in any trouble.
		_, _ = sink.Write(replay) //nolint:errcheck // the caller's read loop reports a dead client
	}
	m.mu.Unlock()

	if old != nil {
		// Outside the lock. The displaced client's own goroutine will wake
		// from its blocked read and call Detach, which wants this mutex,
		// and calling out to unknown code while holding it is how that
		// becomes a deadlock the first time somebody makes kick do more
		// than close a socket.
		old()
	}
	return &Attachment{m: m, gen: gen}
}

// detach removes a client if it is still the current one, and starts the clock.
func (m *managed) detach(gen uint64) {
	m.mu.Lock()
	if m.gen != gen || m.dead {
		m.mu.Unlock()
		return // taken over, or already gone; either way not ours to end
	}
	m.sink, m.kick = nil, nil
	if d := m.reg.cfg.IdleTimeout; d > 0 {
		m.stop = time.AfterFunc(d, func() { m.reap(gen) })
	}
	m.mu.Unlock()
}

// reap ends a session that nobody came back to.
func (m *managed) reap(gen uint64) {
	m.mu.Lock()
	if m.gen != gen || m.sink != nil || m.dead {
		m.mu.Unlock()
		return // somebody attached between the timer firing and this lock
	}
	m.mu.Unlock()
	m.reg.forget(m)
	m.shutdown()
}

// shutdown ends the pty for good. Safe to call twice, because reaping, the
// shell exiting and the registry closing can all reach it.
func (m *managed) shutdown() {
	m.mu.Lock()
	if m.dead {
		m.mu.Unlock()
		return
	}
	m.dead = true
	if m.stop != nil {
		m.stop.Stop()
		m.stop = nil
	}
	kick := m.kick
	m.sink, m.kick = nil, nil
	m.mu.Unlock()

	_ = m.s.Close() //nolint:errcheck // the session is going away regardless
	if kick != nil {
		kick() // so an attached client's socket closes rather than hanging
	}
}

// pump reads the pty for the whole life of the session.
//
// This is the goroutine that makes detaching safe. Without it, a detached
// session is a shell writing into a pty buffer nobody empties, and the first
// few tens of kilobytes of output are the last it ever produces — the process
// blocks in write(2) and stays blocked until somebody re-attaches, which for a
// background build is indistinguishable from having killed it.
func (m *managed) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := m.s.Read(buf)
		if n > 0 {
			m.deliver(buf[:n])
		}
		if err != nil {
			break
		}
	}
	// EOF here is the shell having exited, which is the one ending that must
	// not leave anything re-attachable behind: `exit` typed at a prompt has
	// always meant the shell is gone, and a registry entry that outlived it
	// would hand the next window a dead pty and no explanation.
	m.reg.forget(m)
	m.shutdown()
}

// deliver records one chunk and sends it to whoever is attached.
//
// Recording and picking up the sink happen together under the lock, which is
// what makes attach's snapshot exact — see the long note there. The WRITE is
// deliberately outside it: a client whose far end has vanished blocks in
// Write, and with the mutex held that would block the takeover which is the
// only thing able to rescue the situation. The consequence is that a write can
// land on a sink that was detached a moment ago, which is harmless — that
// socket is closed and the write fails into the branch below, where a stale
// generation makes both the detach and the kick no-ops.
func (m *managed) deliver(p []byte) {
	m.mu.Lock()
	m.ring.write(p)
	sink, kick, gen := m.sink, m.kick, m.gen
	m.mu.Unlock()
	if sink == nil {
		return
	}
	if _, err := sink.Write(p); err != nil {
		// The client is gone, or too slow to keep waiting for. Detaching
		// rather than ending the session is the entire point of this file:
		// the output keeps accumulating in the ring, and whoever comes back
		// is shown it.
		//
		// kick was captured above rather than read back after the detach,
		// because detach clears it — reading it afterwards is how the first
		// version of this managed to leave a wedged socket open forever
		// while believing it had closed it.
		m.detach(gen)
		if kick != nil {
			kick()
		}
	}
}

// ring is a fixed-size window over the most recent output.
//
// A byte ring and not a line buffer, because terminal output is not lines: it
// is a stream in which the interesting parts are escape sequences that move
// the cursor around, and splitting it on newlines would keep a transcript that
// does not correspond to anything that was on screen.
//
// It has no lock of its own. Every caller holds managed.mu, and that is not a
// coincidence to be defended against with a second mutex but the property the
// no-duplicate-replay argument in attach depends on: a ring that could be
// written while managed.mu was held elsewhere would break exactly the ordering
// that argument needs.
type ring struct {
	buf  []byte
	w    int
	full bool
}

func newRing(n int) *ring {
	if n <= 0 {
		return &ring{} // replay disabled; write and replay both no-op
	}
	return &ring{buf: make([]byte, n)}
}

func (r *ring) write(p []byte) {
	if len(r.buf) == 0 {
		return
	}
	// A chunk at least as large as the whole ring can only leave its own
	// tail behind, so it replaces the contents rather than wrapping around
	// the buffer several times to the same effect.
	if len(p) >= len(r.buf) {
		copy(r.buf, p[len(p)-len(r.buf):])
		r.w, r.full = 0, true
		return
	}
	n := copy(r.buf[r.w:], p)
	if n < len(p) {
		copy(r.buf, p[n:])
		r.full = true
	}
	r.w += len(p)
	if r.w >= len(r.buf) {
		r.w -= len(r.buf)
		r.full = true
	}
}

// replay returns the buffered output in order, resynchronized.
//
// THE RESYNCHRONIZATION IS THE SUBTLE PART. Once the ring has wrapped, its
// first byte is wherever the oldest surviving write happened to land, which
// can be the middle of an escape sequence — and a terminal handed the tail of
// a CSI reads the following printable text as its parameters and swallows it.
// The symptom is a re-attached window missing its first line and drawing the
// next few in the wrong color, which looks like a rendering bug and is really
// a framing one.
//
// Three approaches were considered. Parsing the stream here so that only whole
// sequences are ever dropped is the correct one and means writing a VT parser
// in the agent, which is the client's entire job done twice. Prepending a
// reset (ESC c, or SGR 0) fixes inherited colors and does nothing at all about
// a half-eaten CSI, because the reset's own bytes are then read as that
// sequence's parameters — it makes the corruption harder to recognize rather
// than smaller. What is done instead is to skip forward to the first ESC in
// the retained data: ESC aborts whatever sequence a parser is in the middle
// of, so a stream that begins with one begins in the ground state no matter
// what the cut destroyed. The scan is bounded because a partial sequence is at
// most a few dozen bytes; if there is no ESC in the first few kilobytes then
// this is plain text, safe to replay whole, and skipping into it would discard
// output for nothing.
func (r *ring) replay() []byte {
	if len(r.buf) == 0 {
		return nil
	}
	if !r.full {
		// Never wrapped, so it starts where the session did and there is
		// nothing to resynchronize to.
		return append([]byte(nil), r.buf[:r.w]...)
	}
	out := append([]byte(nil), r.buf[r.w:]...)
	out = append(out, r.buf[:r.w]...)
	const scan = 4096
	if i := bytes.IndexByte(out[:min(len(out), scan)], 0x1b); i > 0 {
		out = out[i:]
	}
	return out
}
