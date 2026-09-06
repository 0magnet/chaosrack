//go:build !js

package hostagent

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Telling the operator what they left running.
//
// # Why this is not the listing endpoint session.go refuses to have
//
// The rule next door is absolute and stays absolute: over the wire, a miss
// creates rather than reports, and nothing answers "which sessions exist". The
// reasoning there is about an ORACLE — a caller holding the token can already
// open a shell, so the only new thing an endpoint could give it is knowledge
// of which OTHER shells are live, and that is knowledge about a person rather
// than a machine ("is anyone else on this box", "was that build left running")
// which nothing legitimate needs and no revocation takes back.
//
// The operator is not that caller and the difference is not a matter of
// degree. They own the process, so they can already read every one of these
// facts out of it — ps(1) shows the shells, /proc shows their ptys, a debugger
// shows the ring buffers, and `kill` ends any of it. Nothing here is a
// capability they did not have; it is the same capability without the digging.
// So the question to ask of any way of exposing this is not "is the data
// sensitive" but WHO CAN ASK, and that is a property of the channel:
//
//   - an HTTP or WebSocket endpoint is reachable by anything that can reach
//     the listener, which is the whole point of a listener and is exactly the
//     caller the rule above is about. Guarding it with the token does not
//     help: the token is what that caller already has.
//   - the process's own stdout is reachable by whoever started the process.
//     A browser cannot read it, a page cannot address it, and a local attacker
//     who can read it is already the user who owns the shells.
//
// Hence a signal and a terminal, and hence Registry.List being an ordinary Go
// method with no transport anywhere near it. A caller that wires this into an
// endpoint has undone the argument in session.go; there is nothing this file
// can do to stop that except say so here.
//
// WHAT IS DELIBERATELY NOT REPORTED is the registry key, the token, and the
// contents of the ring buffer. The key and the token are secrets whose value
// is that they never leave the process — printing the key would put a value
// derived from the per-run secret into a terminal, a scrollback and possibly a
// log, for no gain, since the operator addresses a session by name and pid.
// The buffer is reported as a SIZE and never as text: it holds whatever the
// shell printed, which is the one thing here that can be a password, and
// "dump what that window said" is a different feature with a different
// conversation attached to it. The name is reported because it is the only
// handle a person has on which window this was, and it is reported through
// printName because it came off the wire — see there.
//
// # Two questions, two answers
//
// Registry.List answers "what is running NOW". The Notify callback answers
// "what happened". They are not substitutes and neither subsumes the other: a
// session created and reaped between two signals never appears in any listing,
// and a log scrolled off the top of a terminal cannot tell you what is live
// right now. Both are a few lines, so both exist rather than picking the one
// that covers more cases.

// SessionEvent says what happened to a session.
//
// The three endings are separate values rather than one Ended with a reason
// string because they are the three answers to the question an operator
// actually asks of a shell that is gone — did the timeout get it, did somebody
// type exit, or did I stop the server — and a caller that wants to treat them
// alike can, while one that wants to notice that nothing is ever reaped can
// too.
type SessionEvent int

const (
	// SessionCreated is a shell that did not exist a moment ago. It is
	// always immediately attached; the client that named it is what started
	// it.
	SessionCreated SessionEvent = iota

	// SessionAttached is a client taking over a session that already
	// existed — a window reopened, or a takeover of a socket the agent has
	// not yet noticed is dead.
	SessionAttached

	// SessionDetached is the socket going away with the shell still
	// running. This is the event the whole file is for.
	SessionDetached

	// SessionReaped is the idle timeout ending a session nobody came back to.
	SessionReaped

	// SessionExited is the shell ending on its own — `exit` at a prompt, or
	// a kill from outside.
	SessionExited

	// SessionStopped is Registry.Close ending it, which is the server going
	// away.
	SessionStopped
)

// String is the verb used in a log line, in the past tense of something that
// has already happened, because by the time a callback runs it has.
func (e SessionEvent) String() string {
	switch e {
	case SessionCreated:
		return "started"
	case SessionAttached:
		return "attached"
	case SessionDetached:
		return "detached"
	case SessionReaped:
		return "reaped"
	case SessionExited:
		return "exited"
	case SessionStopped:
		return "killed"
	default:
		return "event(" + strconv.Itoa(int(e)) + ")"
	}
}

// SessionInfo is one session as an operator needs to see it.
//
// Times and not durations, deliberately. A snapshot taken now and printed a
// moment later would otherwise be reporting durations measured at an instant
// nobody can identify, and the arithmetic that turns these into "detached for
// 4m12s" belongs where the rendering is — which is also what makes it testable
// without waiting for a clock.
type SessionInfo struct {
	// Name is what the client called the session. Untrusted; render it with
	// printName.
	Name string

	// PID is the shell's process id, which is what makes this report
	// actionable: ps(1) to see what it is doing, kill(1) to end it. Included
	// after asking whether it should be — it is one more fact about the
	// machine in one more place — and the answer is that it is the
	// operator's own process on the operator's own terminal, they can get it
	// from ps anyway, and without it the listing can tell you a shell you
	// forgot is running and not which one it is.
	PID int

	// Attached says a client is connected right now, which is to say there
	// is a window somewhere showing this. The interesting rows are the ones
	// where this is false: those are the shells with nothing on screen to
	// say they exist.
	Attached bool

	// Started is when the shell was forked; Since is when it last attached
	// or detached, so now-Since is how long it has been in this state.
	Started time.Time
	Since   time.Time

	// ReapAt is when the idle timeout will end it. Zero means nothing will:
	// either a client is attached, or IdleTimeout is negative and these
	// shells live until the server stops.
	ReapAt time.Time

	// Buffered is how many bytes of replay are held — a size, never the
	// text. See the note above about what is deliberately not reported.
	Buffered int
}

// infoLocked builds a report for one session. m.mu must be held.
//
// Under the lock because sink, since and reapAt are all written under it, and
// a snapshot that read them piecemeal could report a session as both detached
// and not scheduled for reaping, which is the one combination that cannot
// happen and would send somebody looking for a bug that is not there.
func (m *managed) infoLocked() SessionInfo {
	return SessionInfo{
		Name:     m.name,
		PID:      m.s.PID(),
		Attached: m.sink != nil,
		Started:  m.started,
		Since:    m.since,
		ReapAt:   m.reapAt,
		Buffered: m.ring.buffered(),
	}
}

// notify hands one event to the caller's callback, if there is one.
//
// Every call site is outside both mutexes and that is not incidental: the
// callback is code this package cannot see, and calling it under m.mu would
// deadlock the moment somebody's logger asked the registry a question.
func (r *Registry) notify(ev SessionEvent, info SessionInfo) {
	if r.cfg.Notify != nil {
		r.cfg.Notify(ev, info)
	}
}

// List reports every live session, detached ones first.
//
// The order is the answer to the question being asked. "What did I leave
// running" is about the detached ones — an attached session is a window
// somebody is looking at — so they sort to the top where they will be read,
// and within each group by name so that two runs of this produce two listings
// that can be compared.
//
// THE LOCKS ARE TAKEN IN SEQUENCE, NOT NESTED, and that is a real constraint
// rather than tidiness: managed.attach holds m.mu while it writes the replay
// transcript to a client, which can block for as long as the stall timeout, so
// a List that held r.mu across m.mu would stop every new attachment in the
// process for ten seconds because one browser stopped reading. Copying the
// pointers under r.mu and letting go first means the worst case is a listing
// that waits on one slow session, and it also means the listing can be racing
// a reap — a row that describes a session which ended microseconds ago. That
// is fine and is the nature of the question; the alternative is holding a lock
// that the sessions themselves need in order to finish dying.
func (r *Registry) List() []SessionInfo {
	r.mu.Lock()
	all := make([]*managed, 0, len(r.by))
	for _, m := range r.by {
		all = append(all, m)
	}
	r.mu.Unlock()

	out := make([]SessionInfo, 0, len(all))
	for _, m := range all {
		m.mu.Lock()
		dead := m.dead
		info := m.infoLocked()
		m.mu.Unlock()
		if dead {
			// Removed from the map and not yet observed as gone, or on its
			// way out right now. Reporting it would be reporting a shell
			// that is not there.
			continue
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b SessionInfo) int {
		if a.Attached != b.Attached {
			if a.Attached {
				return 1
			}
			return -1
		}
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return a.PID - b.PID
	})
	return out
}

// PrintSessions writes the listing, one line per session, each line beginning
// with prefix.
//
// The prefix is a parameter because the two commands that mount the agent
// stamp their own name on every line they print and a library that hardcoded
// "desk: " would be wrong in one of them. It is applied per line rather than
// wrapping the writer, which is the simplest thing that survives tabwriter —
// a wrapper that inserted the prefix on every Write would put it in the middle
// of the padding.
//
// Fixed columns and not JSON. The reader is a person who just pressed a key in
// a terminal to answer one question; anything that has to be piped through a
// formatter to be read has failed at that. A machine-readable version can be
// built from List by whoever needs one.
func PrintSessions(w io.Writer, prefix string, ss []SessionInfo) {
	now := time.Now()
	if len(ss) == 0 {
		line(w, "%sno host shells are running.\n", prefix)
		return
	}
	detached := 0
	for _, s := range ss {
		if !s.Attached {
			detached++
		}
	}
	// The count first, because it is the whole answer for the common case
	// where the answer is "none, you are fine".
	line(w, "%s%d host shell(s), %d of them detached:\n", prefix, len(ss), detached)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	line(tw, "%sNAME\tPID\tSTATE\tFOR\tREAPED IN\tBUFFER\n", prefix)
	for _, s := range ss {
		state, reap := "detached", "never"
		if s.Attached {
			state = "attached"
			// Not "never": nothing reaps an attached session, but that is
			// because the clock has not started, and printing "never" here
			// would say the opposite of what happens the moment the window
			// closes.
			reap = "-"
		} else if !s.ReapAt.IsZero() {
			reap = shortDuration(s.ReapAt.Sub(now))
		}
		line(tw, "%s%s\t%d\t%s\t%s\t%s\t%s\n",
			prefix, printName(s.Name), s.PID, state,
			shortDuration(now.Sub(s.Since)), reap, shortBytes(s.Buffered))
	}
	// The error is dropped for the same reason every other write here does
	// not check: this is a report to a terminal, and a terminal that cannot
	// be written to has no second channel to be told about it on.
	_ = tw.Flush() //nolint:errcheck // nowhere to report a failure to report
}

// PrintEvent writes one line about one thing that happened.
//
// Deliberately one line and deliberately in the same vocabulary as the table
// above, so that a scrollback containing both reads as one account of the
// machine rather than two.
func PrintEvent(w io.Writer, prefix string, ev SessionEvent, s SessionInfo) {
	var tail string
	switch ev {
	case SessionDetached:
		// The most useful thing to say at exactly this moment: the shell is
		// still there, and here is the deadline you have to change your
		// mind by.
		tail = "; still running, nothing on screen shows it"
		if !s.ReapAt.IsZero() {
			tail += ", reaped in " + shortDuration(time.Until(s.ReapAt))
		} else {
			tail += ", and nothing will reap it"
		}
	case SessionReaped:
		tail = " after " + shortDuration(time.Since(s.Since)) + " with nobody attached"
	case SessionStopped:
		tail = " because the server is stopping"
	case SessionCreated, SessionAttached, SessionExited:
		// Nothing to add: the verb is the whole event.
	}
	line(w, "%ssession %s (pid %d) %s%s\n", prefix, printName(s.Name), s.PID, ev, tail)
}

// printName renders a client-supplied session name for a terminal.
//
// THE NAME CAME OFF THE WIRE AND THE DESTINATION IS A TERMINAL, which is the
// whole reason this is not a plain %s. A name is up to maxSessionIDLen
// arbitrary bytes chosen by whoever holds the token, and printing those raw
// into the operator's terminal hands that caller an escape sequence: at the
// mild end it repaints the listing, at the sharp end it uses carriage returns
// and cursor movement to overwrite the row above, so that the one session you
// were trying to find is the one the report does not show. A tool whose entire
// job is to reveal something must not be steerable by the thing it is
// revealing.
//
// strconv.Quote rather than stripping the offending bytes, because a name that
// is not what it appears to be is itself the interesting fact and silently
// cleaning it up would hide it. Plain names — which is all a pane ever sends —
// are printed as they are, so the usual listing has no quotes in it and the
// quoted row stands out.
func printName(name string) string {
	if name == "" {
		return `""`
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r > 0x7e {
			return strconv.Quote(name)
		}
	}
	return name
}

// shortDuration renders a duration for a column, rounded to the unit somebody
// would say out loud rather than to the nanosecond the clock knows.
func shortDuration(d time.Duration) string {
	// A negative here is a deadline that has passed while this was being
	// formatted: the timer is firing right now. "0s" is the truth about what
	// happens next; a negative duration in a REAPED IN column reads as a
	// bug.
	if d < 0 {
		d = 0
	}
	// Seconds up to an hour, because "3m12s" is how long ago you closed
	// that window; minutes past it, because the seconds in "2h14m53s" are
	// noise in a column being scanned for which row to worry about.
	if d < time.Hour {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}

// shortBytes renders a byte count in the units the buffer is configured in.
func shortBytes(n int) string {
	switch {
	case n < 1024:
		return strconv.Itoa(n) + " B"
	case n < 1024*1024:
		return strconv.Itoa(n/1024) + " KiB"
	default:
		return strconv.Itoa(n/(1024*1024)) + " MiB"
	}
}

// line writes one line of a report and discards the error.
//
// A helper rather than an ignored return at each call site, because there are
// six of them and six identical apologies for not checking is worse than one.
// The apology: this is a report being printed to a terminal, and a terminal
// that cannot be written to leaves nowhere to say so — returning the error
// would only push the same problem up to a caller in a signal handler, which
// has even less to do about it.
func line(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...) //nolint:errcheck // a report has nowhere to report a failure to report
}
