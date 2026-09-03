package pisano

// Walking a period a term at a time, forever.
//
// Walk and Walk3 hand back a whole path at once, which is what a renderer that
// draws a finished figure wants. A viewer wants the opposite: the walk never
// finishes, so it cannot be computed and then displayed — it has to be extended
// a few steps per frame, indefinitely, while the oldest of it is dropped off the
// far end. Recomputing the path from the start on every frame is not an option
// at sixty frames a second, and stepping by whole passes would make the drawing
// jump rather than crawl.
//
// So the turtle is kept mid-stride. What that costs is the bookkeeping below:
// where in the run-in or the block the next term comes from, how many passes
// have gone by, and how many steps have been taken — all of which a step
// carries, because a color can be keyed on any of them.
//
// An unbounded period is a truncated prefix rather than a cycle, so there is
// nothing to repeat: the walker walks it once and then reports no further
// movement, forever. A caller that loops "advance n steps" therefore terminates
// rather than drawing a loop the sequence does not have.

// Walker walks a period in the plane, one term at a time.
type Walker struct{ w walkState }

// NewWalker starts a walk at the origin, facing east.
func NewWalker(p Period) *Walker { return &Walker{w: newWalkState(p)} }

// Next consumes one term and reports the step it made, if it made one: a zero
// term turns nothing and goes nowhere, and an exhausted unbounded period never
// moves again.
func (w *Walker) Next() (Step, bool) {
	term, _, pass, ok := w.w.take()
	if !ok {
		return Step{}, false
	}
	from := w.w.t.Pos
	if !w.w.t.Step(term) {
		return Step{}, false
	}
	w.w.n++
	return Step{
		From: from, To: w.w.t.Pos, Term: term,
		Pass: pass, Dir: w.w.t.Dir, Index: w.w.n - 1,
	}, true
}

// Pass is how many complete passes of the repeating block have been walked.
func (w *Walker) Pass() int { return w.w.pass }

// Steps is how many moves the turtle has made, which is fewer than the number
// of terms consumed by however many of them were zero.
func (w *Walker) Steps() int { return w.w.n }

// Walker3 walks a period in space, one term at a time.
type Walker3 struct {
	w walkState
	t Turtle3
}

// NewWalker3 starts a walk at the origin in the identity frame.
func NewWalker3(p Period) *Walker3 {
	return &Walker3{w: newWalkState(p), t: NewTurtle3()}
}

// Next consumes one term and reports the step it made, if it made one. The rule
// in space reads the term that follows as well, which is why the state has to
// know what comes next as well as what comes now.
func (w *Walker3) Next() (Step3, bool) {
	term, next, pass, ok := w.w.take()
	if !ok {
		return Step3{}, false
	}
	from := w.t.Pos
	if !w.t.Step(term, next) {
		return Step3{}, false
	}
	w.w.n++
	return Step3{
		From: from, To: w.t.Pos, Term: term,
		Pass: pass, Dir: w.t.Frame, Index: w.w.n - 1,
	}, true
}

// Pass is how many complete passes of the repeating block have been walked.
func (w *Walker3) Pass() int { return w.w.pass }

// Steps is how many moves the turtle has made.
func (w *Walker3) Steps() int { return w.w.n }

// walkState is the part that does not care how many dimensions there are:
// which term comes next, which pass it belongs to, and whether there is one.
type walkState struct {
	p    Period
	t    Turtle // the plane's turtle; Walker3 carries its own
	i    int    // next index into the run being consumed
	head bool   // still working through the run-in
	pass int    // completed passes of the repeating block
	n    int    // steps taken
	done bool   // an unbounded period, walked out
}

func newWalkState(p Period) walkState {
	return walkState{p: p, head: len(p.Head) > 0}
}

// take consumes one term and reports it, the term after it, and the pass it
// belongs to (-1 for the run-in). The term after is what the rule in space
// needs; in the plane it is ignored.
//
// At the end of the block the term after is the first one, which is not a
// fudge: the block really is periodic there, so that genuinely is the next
// term.
func (s *walkState) take() (term, next, pass int, ok bool) {
	if s.head && len(s.p.Head) == 0 {
		s.head = false
	}
	if s.done || (!s.head && len(s.p.Terms) == 0) {
		return 0, 0, 0, false
	}
	pass = s.pass
	if s.head {
		pass = -1
		term = s.p.Head[s.i]
		switch {
		case s.i+1 < len(s.p.Head):
			next = s.p.Head[s.i+1]
		case len(s.p.Terms) > 0:
			next = s.p.Terms[0]
		}
		s.i++
		if s.i >= len(s.p.Head) {
			s.head, s.i = false, 0
		}
		return term, next, pass, true
	}
	term = s.p.Terms[s.i]
	next = s.p.Terms[(s.i+1)%len(s.p.Terms)]
	s.i++
	if s.i >= len(s.p.Terms) {
		s.i, s.pass = 0, s.pass+1
		if !s.p.Bounded {
			// Not a cycle — a prefix that was cut off. Walking it again would
			// draw a repeat the sequence does not have.
			s.done = true
		}
	}
	return term, next, pass, true
}
