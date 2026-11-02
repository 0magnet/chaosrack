//go:build !js

package subcmds

import (
	"sync"
	"testing"
	"time"
)

// put is a chunk of n samples arriving from the sound server.
func (c *liveCapture) put(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for range n {
		c.l, c.r = append(c.l, 1), append(c.r, -1)
	}
	c.cond.Broadcast()
}

// TestALiveRecordingIsDrawnWhole: a frame waits for its samples rather than
// drawing short, and once the recording ends, what had come in is still
// drawn before the video ends.
func TestALiveRecordingIsDrawnWhole(t *testing.T) {
	c := &liveCapture{stop: func() {}}
	c.cond = sync.NewCond(&c.mu)
	got := make(chan int)
	go func() {
		l, _ := c.take(800)
		got <- len(l)
	}()
	c.put(500)
	select {
	case n := <-got:
		t.Fatalf("a frame took %d samples of the 800 it stands for", n)
	case <-time.After(50 * time.Millisecond):
	}
	c.put(500)
	if n := <-got; n != 800 {
		t.Fatalf("a frame took %d samples, not 800", n)
	}
	c.end()
	if l, _ := c.take(800); len(l) != 200 {
		t.Errorf("after the end, %d samples were left to draw, not 200", len(l))
	}
	if l, _ := c.take(800); len(l) != 0 {
		t.Errorf("an ended recording still gave %d samples", len(l))
	}
	c.end() // Ctrl+C after it ended by itself
}
