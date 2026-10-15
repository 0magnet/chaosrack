package rackhost

import (
	"image/gif"
	"os"
	"strings"
	"testing"
	"time"
)

// RECORD takes the scene and STOP writes it out as a GIF in the working
// directory; the counter runs while it records and the log says what it came
// out as.
func TestRecordWritesATake(t *testing.T) {
	t.Chdir(t.TempDir())
	r, c := idleRack(t, "lorenz")
	r.mu.Lock()
	r.press("btn rec-btn")
	r.mu.Unlock()
	watch(t, r, c, 1.2)
	r.mu.Lock()
	counter := ledText(r, "rec-status")
	r.press("btn rec-stop-btn")
	r.mu.Unlock()
	if counter != "00:00:01" {
		t.Errorf("after a second the counter reads %q", counter)
	}
	deadline := time.Now().Add(10 * time.Second)
	saved := ""
	for saved == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		r.mu.Lock()
		saved = r.recSaved
		r.mu.Unlock()
	}
	if saved == "" {
		t.Fatal("the take was never written")
	}
	f, err := os.Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) < 8 || g.Config.Width != recWidth {
		t.Errorf("the take is %d frames %d wide, want a second's at %d", len(g.Image), g.Config.Width, recWidth)
	}
	r.mu.Lock()
	log := ledText(r, "rec-log")
	r.mu.Unlock()
	if !strings.HasPrefix(log, "GIF  00:00:01  ") {
		t.Errorf("the log reads %q", log)
	}
}
