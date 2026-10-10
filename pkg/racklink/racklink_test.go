package racklink

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// fakePage is a page: a rack of one control, and a picture script.
type fakePage struct {
	mu   sync.Mutex
	name string
	val  string
	acts []string
}

func (f *fakePage) Modules() ([]racksurface.Item, int, error) {
	return []racksurface.Item{{Key: "m", Slots: 2}}, 12, nil
}

func (f *fakePage) Controls() ([]racktui.Control, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []racktui.Control{{ControlInfo: controlspec.ControlInfo{ID: "zoom", Label: f.name}, Value: f.val}}, nil
}

func (f *fakePage) Set(id, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.val = v
	return nil
}

func (f *fakePage) PictureJSON() string {
	return `{"gen":7,"w":10,"h":5,"items":[{"x":1,"y":1,"w":2,"h":2,"t":"` + f.name + `","f":1}],"ctls":{}}`
}

func (f *fakePage) ChangesJSON(gen int) string { return `{"gen":7}` }

func (f *fakePage) ActJSON(gen int, x, y float64, kind string, delta float64) string {
	f.mu.Lock()
	f.acts = append(f.acts, kind)
	f.mu.Unlock()
	return `{}`
}

// connectPage runs a page against the hub, answering every call.
func connectPage(h *Hub, f *fakePage) (stop func()) {
	a, b := net.Pipe()
	go h.ServePage(LineConn(a))
	c := LineConn(b)
	go func() {
		for {
			req, err := c.Read()
			if err != nil {
				return
			}
			_ = c.Write(Answer(f, req)) //nolint:errcheck // a test page
		}
	}()
	return func() { _ = c.Close() } //nolint:errcheck // a test page
}

func connectTerminal(h *Hub) *Client {
	a, b := net.Pipe()
	go h.ServeTerminal(LineConn(a))
	return NewClient(LineConn(b), "pipe")
}

func waitPages(t *testing.T, h *Hub, n int) {
	t.Helper()
	for range 200 {
		if h.Pages() == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the hub has %d pages, want %d", h.Pages(), n)
}

// A terminal's calls reach the page and the answers come back: the Source,
// and the picture as the page's script wrote it.
func TestATerminalDrivesThePageThroughTheHub(t *testing.T) {
	h := NewHub()
	f := &fakePage{name: "first", val: "1"}
	defer connectPage(h, f)()
	waitPages(t, h, 1)
	cl := connectTerminal(h)
	defer cl.Close() //nolint:errcheck // teardown

	if err := cl.Set("zoom", "2.5"); err != nil {
		t.Fatal(err)
	}
	cs, err := cl.Controls()
	if err != nil || len(cs) != 1 || cs[0].Value != "2.5" {
		t.Fatalf("after set the page says %+v, %v", cs, err)
	}
	mods, slots, err := cl.Modules()
	if err != nil || len(mods) != 1 || slots != 12 {
		t.Fatalf("modules %+v %d %v", mods, slots, err)
	}
	p, err := cl.Picture()
	if err != nil || p.Gen != 7 || len(p.Items) != 1 || p.Items[0].Text != "first" {
		t.Fatalf("picture %+v, %v", p, err)
	}
	if _, err := cl.PictureChanges(7); err != nil {
		t.Fatal(err)
	}
	if err := cl.Act(7, 1, 1, "click", 0); err != nil || len(f.acts) != 1 {
		t.Fatalf("act: %v, the page saw %v", err, f.acts)
	}
}

// With no page open the terminal is told so, rather than left waiting.
func TestWithNoPageTheTerminalIsTold(t *testing.T) {
	h := NewHub()
	cl := connectTerminal(h)
	defer cl.Close() //nolint:errcheck // teardown
	if _, err := cl.Controls(); err == nil || !strings.Contains(err.Error(), "no chaosrack page") {
		t.Fatalf("with no page, controls answered %v", err)
	}
}

// The newest page answers; when it goes, the one before takes over.
func TestTheNewestPageAnswers(t *testing.T) {
	h := NewHub()
	defer connectPage(h, &fakePage{name: "old"})()
	waitPages(t, h, 1)
	stopNew := connectPage(h, &fakePage{name: "new"})
	waitPages(t, h, 2)
	cl := connectTerminal(h)
	defer cl.Close() //nolint:errcheck // teardown
	if cs, err := cl.Controls(); err != nil || len(cs) != 1 || cs[0].Label != "new" {
		t.Fatalf("with two pages the answer came from %+v, %v", cs, err)
	}
	stopNew()
	waitPages(t, h, 1)
	if cs, err := cl.Controls(); err != nil || len(cs) != 1 || cs[0].Label != "old" {
		t.Fatalf("after the newer page closed the answer came from %+v, %v", cs, err)
	}
}

// Two terminals at once each get their own answers.
func TestTwoTerminalsDoNotCrossAnswers(t *testing.T) {
	h := NewHub()
	defer connectPage(h, &fakePage{name: "p", val: "0"})()
	waitPages(t, h, 1)
	a, b := connectTerminal(h), connectTerminal(h)
	defer a.Close() //nolint:errcheck // teardown
	defer b.Close() //nolint:errcheck // teardown
	var wg sync.WaitGroup
	for _, cl := range []*Client{a, b} {
		wg.Go(func() {
			for range 50 {
				if _, _, err := cl.Modules(); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}

// A line longer than a pipe's buffer, as a whole picture is, arrives whole.
func TestALongLineArrivesWhole(t *testing.T) {
	a, b := net.Pipe()
	ca, cb := LineConn(a), LineConn(b)
	long := strings.Repeat("x", 300<<10)
	go func() { _ = ca.Write([]byte(long)) }() //nolint:errcheck // a test
	got, err := cb.Read()
	if err != nil || string(got) != long {
		t.Fatalf("read %d bytes, %v", len(got), err)
	}
}

func (f *fakePage) SceneJSON(w, h int, shape float64) string {
	return `{"w":1,"h":1,"px":"AQID"}`
}

func (f *fakePage) CanvasesJSON(gen int, want []rackpic.CanvasWant) string {
	return `{"imgs":{"3":{"w":1,"h":1,"px":"BAUG"}}}`
}

// The scene and the canvases come over the link as pixels.
func TestPicturesComeOverTheLink(t *testing.T) {
	h := NewHub()
	defer connectPage(h, &fakePage{})()
	waitPages(t, h, 1)
	cl := connectTerminal(h)
	defer cl.Close() //nolint:errcheck // teardown
	m, err := cl.Scene(1, 1, 1)
	if err != nil || m.At(0, 0).R != 1 || m.At(0, 0).B != 3 {
		t.Fatalf("scene %+v, %v", m, err)
	}
	cs, err := cl.Canvases(7, []rackpic.CanvasWant{{Index: 3, W: 1, H: 1}})
	if err != nil || cs[3] == nil || cs[3].At(0, 0).G != 5 {
		t.Fatalf("canvases %+v, %v", cs, err)
	}
}

func (f *fakePage) SceneActJSON(w, h int, shape, x, y float64, kind string, delta float64) string {
	f.mu.Lock()
	f.acts = append(f.acts, "scene "+kind)
	f.mu.Unlock()
	return `{}`
}

var _ racktui.SceneActor = (*Client)(nil)

// What the mouse does to the scene reaches the page.
func TestASceneActCrossesTheLink(t *testing.T) {
	h := NewHub()
	f := &fakePage{}
	defer connectPage(h, f)()
	waitPages(t, h, 1)
	cl := connectTerminal(h)
	defer cl.Close() //nolint:errcheck // teardown
	if err := cl.SceneAct(10, 10, 1, 2, 3, "down", 0); err != nil || len(f.acts) != 1 || f.acts[0] != "scene down" {
		t.Fatalf("scene act: %v, the page saw %v", err, f.acts)
	}
}
