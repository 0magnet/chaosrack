package rackhost

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// The keys in the picture: each lit while it sounds, as the page lights a
// held key (panel.css .held), and played by a press, a drag across them and a
// release, which no control's box describes. The catalog marks each key
// "key N", N its midi note.

// The held keys' colors: their gradients' bodies, as the picture reads a
// gradient.
const (
	heldWhite = 0x5fd894
	heldBlack = 0x0b3a20
)

// keysPic is where the keys are in the picture, what they show, and the key
// the pointer holds down.
type keysPic struct {
	item  map[int]int // midi note → its item
	base  map[int]rackpic.Item
	shown map[int]bool

	mouse        int // the note the pointer holds, or -1
	lastX, lastY float64
	lastT        time.Time
}

// findKeys finds the keys among the items outside the bank.
func (r *Rack) findKeys() {
	kp := &r.keys
	kp.item, kp.base, kp.shown, kp.mouse = map[int]int{}, map[int]rackpic.Item{}, map[int]bool{}, -1
	for i, it := range r.pic.Items[:r.outside] {
		if midi, ok := keyOf(it); ok {
			kp.item[midi] = i
			kp.base[midi] = *it
		}
	}
}

// keyOf is the note of a key's item.
func keyOf(it *rackpic.Item) (int, bool) {
	if it == nil {
		return 0, false
	}
	return keyMark(it.Mark)
}

// keyMark reads "key N".
func keyMark(mark string) (int, bool) {
	s, ok := strings.CutPrefix(mark, "key ")
	if !ok {
		return 0, false
	}
	m, err := strconv.Atoi(s)
	return m, err == nil
}

// isBlack reports whether midi is a black key.
func isBlack(midi int) bool {
	switch midi % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// showKeys lights the keys sounding and puts out the rest. Called with r.mu
// held.
func (r *Rack) showKeys() {
	kp := &r.keys
	if len(kp.item) == 0 {
		return
	}
	s := r.sound
	s.mu.Lock()
	lit := s.keys.lit()
	s.mu.Unlock()
	for midi, i := range kp.item {
		if lit[midi] == kp.shown[midi] {
			continue
		}
		it := kp.base[midi]
		if lit[midi] {
			c := heldWhite
			if isBlack(midi) {
				c = heldBlack
			}
			it.Fill = &c
			kp.shown[midi] = true
		} else {
			delete(kp.shown, midi)
		}
		r.setItem(i, &it)
	}
}

// keysAct is what the pointer does to the keys: a press holds the key under
// it down, a drag moves to the key it is over, sounding every key on the way,
// and a release lets go. It reports whether the keys took it. Called with
// r.mu held.
func (r *Rack) keysAct(x, y float64, kind string) bool {
	kp := &r.keys
	k := &r.sound.keys
	switch kind {
	case "click":
		midi, ok := keyMark(r.markAt(x, y))
		if !ok {
			return false
		}
		r.sound.mu.Lock()
		if kp.mouse >= 0 {
			k.up(kp.mouse)
		}
		k.down(midi, 0)
		r.sound.mu.Unlock()
		kp.mouse, kp.lastX, kp.lastY, kp.lastT = midi, x, y, time.Now()
		return true
	case "drag":
		if kp.mouse < 0 {
			return false
		}
		r.glide(x, y)
		return true
	case "release":
		if kp.mouse < 0 {
			return false
		}
		r.sound.mu.Lock()
		k.up(kp.mouse)
		r.sound.mu.Unlock()
		kp.mouse = -1
		return true
	}
	return false
}

// glide moves the held note to the key under x, y, sounding every key on the
// straight line from where the pointer was last seen: a pointer faster than a
// key an event crosses several between two, and a run that skipped them
// would not be the run the hand played. The keys passed are short notes
// spread across the time since the last event, and the key landed on sounds
// after them, as the page's glide plays them. Called with r.mu held.
func (r *Rack) glide(x, y float64) {
	kp := &r.keys
	now := time.Now()
	x0, y0, dt := kp.lastX, kp.lastY, now.Sub(kp.lastT).Seconds()
	kp.lastX, kp.lastY, kp.lastT = x, y, now
	var path []int
	steps := int(math.Hypot(x-x0, y-y0)/3) + 1
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		m, ok := keyMark(r.markAt(x0+(x-x0)*f, y0+(y-y0)*f))
		if !ok || m == kp.mouse || len(path) > 0 && path[len(path)-1] == m {
			continue
		}
		path = append(path, m)
	}
	if len(path) == 0 {
		return
	}
	per := int(min(max(dt, 0.02), 0.15) / float64(len(path)) * sampleRate)
	k := &r.sound.keys
	r.sound.mu.Lock()
	defer r.sound.mu.Unlock()
	k.up(kp.mouse)
	passing := path[:len(path)-1]
	for i, m := range passing {
		k.pass(m, i*per, per)
	}
	kp.mouse = path[len(path)-1]
	k.down(kp.mouse, len(passing)*per)
}
