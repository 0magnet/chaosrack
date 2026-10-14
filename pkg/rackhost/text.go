package rackhost

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// Text a hand types into the rack: Custom's equations, Fourier Text's
// banner, and the path of an STL file to load, which the page asks for with
// a file picker and the native rack asks for on the panel's status line.

// stlPath is the native rack's own control for the STL file to load: the
// page has none, since its picker hands it the file itself.
const stlPath = "stlfile-path"

// addTextControls adds the text controls the native rack has and the
// catalog does not. Called once, from New.
func (r *Rack) addTextControls() {
	r.addControl(racktui.Control{ControlInfo: controlspec.ControlInfo{
		ID: stlPath, Label: "file", IsText: true, Module: modelModule,
	}})
}

// addControl adds a control the catalog does not have, if the rack does not
// have it already.
func (r *Rack) addControl(c racktui.Control) {
	if _, ok := r.byID[c.ID]; ok {
		return
	}
	r.byID[c.ID] = len(r.ctls)
	r.ctls = append(r.ctls, c)
}

// textSet acts on a text control just typed. Called with r.mu held.
func (r *Rack) textSet(id, v string) error {
	switch {
	case strings.HasPrefix(id, "custom-eq-"):
		i, err := strconv.Atoi(strings.TrimPrefix(id, "custom-eq-"))
		if err != nil {
			return nil
		}
		r.custom.SetEquation(i, v)
		r.custom.Compile()
		r.customControls()
		r.refigure.Store(true)
	case id == stlPath:
		if v == "" {
			return nil
		}
		data, err := os.ReadFile(v) //nolint:gosec // a file the person at the panel named
		if err != nil {
			return err
		}
		w, err := attractor.STLFileWire(data)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(v), ".stl"), ".STL")
		r.stl.wire, r.stl.label, r.stl.built = w, name, ""
		// The picker back on the file, so it is what is shown.
		if i, ok := r.byID["stlfile-builtin"]; ok && r.ctls[i].Value != "" {
			r.ctls[i].Value = ""
			r.turn(&r.ctls[i])
		}
		r.refigure.Store(true)
	}
	return nil
}

// customControls gives each constant the equations name a knob, as the
// page builds one per constant, seeded with its value. Called with r.mu
// held.
func (r *Rack) customControls() {
	for _, name := range r.custom.Params() {
		v, _ := r.custom.Param(name)
		r.addControl(racktui.Control{
			ControlInfo: controlspec.ControlInfo{
				ID: "custom-" + name, Label: name, Module: modelModule,
				Min: -10, Max: 10, Step: 0.01, Def: 1,
			},
			Value: strconv.FormatFloat(float64(v), 'g', -1, 32),
		})
	}
}

// Prompt is the text the rack is waiting for, once: the path STL File's
// load switch asks for.
func (r *Rack) Prompt() (id, ask string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prompt == "" {
		return "", "", false
	}
	id, r.prompt = r.prompt, ""
	return id, "STL file to load", true
}
