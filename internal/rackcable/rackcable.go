// Package rackcable is the wire to a running rack.
//
// The instrument is wasm in a page: its controls are built by code that needs
// a browser, and the geometry it draws needs a GPU. So a terminal front end
// does not run the rack, it reaches one — and this is the reaching. It is the
// only part of the terminal panel that knows a browser is involved, which is
// the point: pkg/racktui renders a Source, and this is a Source.
//
// Everything it asks for is something the rack already says about itself.
// window.rackctl is the control registry adoptDescControl records as it wires
// the panel; the module names and widths are what the drawing needs measured.
// Nothing here knows what a control MEANS.
package rackcable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/0magnet/cdp"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// Client is a cable to one running rack.
type Client struct {
	c *cdp.Client

	// Monitor is how many slots to give each bay's chassis monitor when
	// drawing, or zero to draw the rack as it is.
	Monitor int
	// Capacity overrides the slots-per-row the page reports.
	Capacity int
}

// Dial finds the rack on a browser's remote-debugging port.
func Dial(port int, target string) (*Client, error) {
	c, err := cdp.Dial(port, target)
	if err != nil {
		return nil, err
	}
	cl := &Client{c: c}
	if v, ok := c.Eval(`typeof rackctl`).(string); !ok || v != "object" {
		return nil, errors.New("the page has no rackctl — is it running a build with the control registry?")
	}
	return cl, nil
}

// ── the control surface ──────────────────────────────────────────────────

// readValues asks for every control's current value and, for a selector, the
// detents it has — in ONE crossing, because the alternative is a round trip
// per control and there are seventy-odd of them.
const readValues = `(function(){
  return JSON.stringify(JSON.parse(rackctl.list()).map(function(c){
    var el = document.getElementById(c.id);
    var opts = null;
    if (el && el.tagName === 'SELECT') {
      opts = [].slice.call(el.options).map(function(o){ return o.value; });
    }
    return {id: c.id, v: el ? String(el.value) : '', o: opts};
  }));
})()`

// List is the registry alone: what the rack's controls ARE.
func (r *Client) List() ([]attractor.ControlInfo, error) {
	s, _ := r.c.Eval(`rackctl.list()`).(string)
	var info []attractor.ControlInfo
	if err := json.Unmarshal([]byte(s), &info); err != nil {
		return nil, fmt.Errorf("reading the control registry: %w", err)
	}
	return info, nil
}

// Controls is the registry with what each control currently says.
func (r *Client) Controls() ([]racktui.Control, error) {
	info, err := r.List()
	if err != nil {
		return nil, err
	}
	vs, _ := r.c.Eval(readValues).(string)
	var vals []struct {
		ID string   `json:"id"`
		V  string   `json:"v"`
		O  []string `json:"o"`
	}
	if err := json.Unmarshal([]byte(vs), &vals); err != nil {
		return nil, fmt.Errorf("reading the control values: %w", err)
	}
	byID := make(map[string]int, len(vals))
	for i, v := range vals {
		byID[v.ID] = i
	}
	out := make([]racktui.Control, 0, len(info))
	for _, in := range info {
		c := racktui.Control{ControlInfo: in}
		if i, ok := byID[in.ID]; ok {
			c.Value, c.Options = vals[i].V, vals[i].O
		}
		out = append(out, c)
	}
	return out, nil
}

// Get is one control's current value.
func (r *Client) Get(id string) (string, error) {
	v := r.c.Eval(fmt.Sprintf(`rackctl.get(%s)`, strconv.Quote(id)))
	if v == nil {
		return "", fmt.Errorf("the rack has no control %q", id)
	}
	return fmt.Sprint(v), nil
}

// Set moves one control, as a hand would.
func (r *Client) Set(id, value string) error {
	ok, _ := r.c.Eval(fmt.Sprintf(`rackctl.set(%s,%s)`, strconv.Quote(id), strconv.Quote(value))).(bool)
	if !ok {
		return fmt.Errorf("the rack has no control %q", id)
	}
	return nil
}

// ── the bays ─────────────────────────────────────────────────────────────

// readRack is evaluated in the page. Slots are derived the way moduleSlots
// does it, from the module's measured width against the slot pitch.
const readRack = `(function(){
  var scale = parseFloat(getComputedStyle(document.documentElement)
      .getPropertyValue('--kscale')) || 1;
  var slot = 140.24, gap = 2.0;               // moduleSlot / moduleGap at scale 1
  var pitch = (slot + gap) * scale;
  var out = [];
  document.querySelectorAll('.sect').forEach(function(m){
    var hdr = m.querySelector('.sect-hdr');
    if (!hdr) return;
    var w = m.offsetWidth;
    var n = w > 0 ? Math.max(1, Math.round((w + gap*scale)/pitch)) : 0;
    if (w === 0 && m.style.display === 'none') n = 0;
    out.push({k: hdr.textContent.trim().toLowerCase(), s: n,
              c: m.getAttribute('data-cat') || ''});
  });
  return JSON.stringify(out);
})()`

// Rack draws the bays.
//
// What comes off the page is only what has to be MEASURED — a module's name,
// how many slots it takes at the interface scale in use, and the category a
// model card belongs to. The sections and the bays are computed by the same
// code the rack itself runs, so the drawing cannot disagree with it.
func (r *Client) Rack() (string, error) {
	keys, cats, slots, err := r.measure()
	if err != nil {
		return "", err
	}
	var monitors map[string]int
	if r.Monitor > 0 {
		monitors = map[string]int{}
		for i, k := range keys {
			monitors[attractor.DrawSectionOf(k, cats, i)] = r.Monitor
		}
	}
	return attractor.DrawRackFrom(keys, cats, slots, r.slotsPerRow(), monitors), nil
}

// slotsPerRow is how many slots a row holds: the figure the page itself
// packs with, which is the rack's width in modules at every scale.
//
// It used to be measured off the frame, whose width includes the rails, so
// it came out at 14 where the page packs 12 and the drawing showed bays the
// page did not have.
func (r *Client) slotsPerRow() int {
	if r.Capacity > 0 {
		return r.Capacity
	}
	return racksurface.UnitCapacity()
}

// Modules is what the frame holds, for a front end that lays the rack out
// rather than printing it.
//
// It reads the same measurement Rack does and stops one step earlier: the
// items, grouped into their bays, instead of the drawing made from them. Both
// go through attractor, so the picture and the geometry cannot disagree.
func (r *Client) Modules() ([]racksurface.Item, int, error) {
	keys, cats, slots, err := r.measure()
	if err != nil {
		return nil, 0, err
	}
	return attractor.RackItemsFrom(keys, cats, slots, nil), r.slotsPerRow(), nil
}

// measure reads what only a laid-out panel can answer: each module's name, the
// slots it takes at the interface scale in use, and the category a model card
// belongs to.
func (r *Client) measure() (keys, cats []string, slots []int, err error) {
	s, _ := r.c.Eval(readRack).(string)
	var mods []struct {
		K string `json:"k"`
		S int    `json:"s"`
		C string `json:"c"`
	}
	if err := json.Unmarshal([]byte(s), &mods); err != nil {
		return nil, nil, nil, fmt.Errorf("reading the panel: %w", err)
	}
	if len(mods) == 0 {
		return nil, nil, nil, errors.New("the page reported no modules")
	}
	keys = make([]string, len(mods))
	cats = make([]string, len(mods))
	slots = make([]int, len(mods))
	for i, m := range mods {
		keys[i], slots[i], cats[i] = m.K, m.S, m.C
	}
	return keys, cats, slots, nil
}
