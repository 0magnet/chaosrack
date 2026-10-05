package lattice

import (
	"bytes"
	"math"
	"strconv"
	"strings"
)

// What a program reads: its input, as any terminal program reads its own.
//
// Keys turn the solid — ← and → about the vertical, ↑ and ↓ toward and away
// from the viewer, five degrees a press — and r puts it back. Every sheet's
// program has to be told, and tmux's synchronize-panes is exactly that: one
// keypress in every pane.
//
// A host that turns the stack itself can hand every program a POSE instead:
//
//	ESC ] lattice ; pose ; w ; x ; y ; z BEL
//
// an orientation, as a unit quaternion, applied to the solid after its own
// turning. Holding the solid still while the stack turns is the inverse of
// the stack's turn; turning the solid with the view while the stack stands
// still is the turn itself. It is an operating system command in form, which
// a terminal ignores when it is not one it knows.

// keyStep is how far a key turns the solid, in radians.
const keyStep = 5 * math.Pi / 180

// Quat is a rotation as a unit quaternion, w first.
type Quat [4]float64

// Identity is the quaternion that turns nothing.
var Identity = Quat{1, 0, 0, 0}

// PoseSequence is the input that sets a program's pose to q.
func PoseSequence(q Quat) string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	return "\x1b]lattice;pose;" + f(q[0]) + ";" + f(q[1]) + ";" + f(q[2]) + ";" + f(q[3]) + "\a"
}

// rotate is q applied to p.
func (q Quat) rotate(p Vec3) Vec3 {
	u := Vec3{q[1], q[2], q[3]}
	t := u.Cross(p).Scale(2)
	return p.Add(t.Scale(q[0])).Add(u.Cross(t))
}

// conj is q's inverse, q being a unit quaternion.
func (q Quat) conj() Quat { return Quat{q[0], -q[1], -q[2], -q[3]} }

// Posed is s turned by q.
func Posed(s Shape, q Quat) Shape {
	if q == Identity {
		return s
	}
	inv := q.conj()
	return func(p Vec3) float64 { return s(inv.rotate(p)) }
}

// Input takes what the program's terminal sends it: keys, and pose
// sequences. It reports whether the input asked the program to quit (q, or
// ^C, which a terminal in raw mode sends as a byte). A sequence may arrive in
// pieces; the start of one is kept until the rest comes.
func (p *Program) Input(b []byte) (quit bool) {
	p.in = append(p.in, b...)
	for len(p.in) > 0 {
		c := p.in[0]
		if c != 0x1b {
			p.in = p.in[1:]
			switch c {
			case 'q', 3:
				quit = true
			case 'r':
				p.keyYaw, p.keyPitch = 0, 0
			}
			continue
		}
		n, done := p.escape(p.in)
		if !done {
			if len(p.in) > 4096 { // not a sequence that will ever end
				p.in = p.in[:0]
			}
			return quit
		}
		p.in = p.in[n:]
	}
	return quit
}

// escape reads the escape sequence at the start of b: how long it is, or
// done false if it is not all there yet.
func (p *Program) escape(b []byte) (n int, done bool) {
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '[', 'O': // a cursor key: ESC [ A, or ESC O A in application mode
		if len(b) < 3 {
			return 0, false
		}
		switch b[2] {
		case 'A':
			p.keyPitch -= keyStep
		case 'B':
			p.keyPitch += keyStep
		case 'C':
			p.keyYaw += keyStep
		case 'D':
			p.keyYaw -= keyStep
		}
		return 3, true
	case ']':
		end := bytes.IndexByte(b, '\a')
		st := bytes.Index(b, []byte("\x1b\\"))
		switch {
		case end < 0 && st < 0:
			return 0, false
		case end < 0 || (st >= 0 && st < end):
			p.osc(string(b[2:st]))
			return st + 2, true
		}
		p.osc(string(b[2:end]))
		return end + 1, true
	}
	return 2, true // an escape this program has no use for
}

// osc acts on an operating system command addressed to lattice.
func (p *Program) osc(s string) {
	f := strings.Split(s, ";")
	if len(f) != 6 || f[0] != "lattice" || f[1] != "pose" {
		return
	}
	var q Quat
	for i := range 4 {
		v, err := strconv.ParseFloat(f[2+i], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		q[i] = v
	}
	l := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if l == 0 {
		return
	}
	for i := range q {
		q[i] /= l
	}
	p.pose = q
}
