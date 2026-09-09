//go:build js && wasm

package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// stubSource stands in for a live audio source so that the paths which merely
// ASK one for its sample rate can be reached from a test. It delivers nothing:
// everything here is driven from the mode's own ring, which the test fills.
type stubSource struct{}

func (stubSource) TimeDomain(dst []float32) []float32 { return dst }
func (stubSource) TimeDomainStereo(l, r []float32)    {}
func (stubSource) Drain([]float32) int                { return 0 }
func (stubSource) SampleRate() int                    { return 24000 }
func (stubSource) Channels() int                      { return 1 }
func (stubSource) Ready() bool                        { return true }
func (stubSource) Err() error                         { return nil }
func (stubSource) Close()                             {}

var _ audiosrc.Source = stubSource{}

// polarMaxLen is the longest a raw delay vector can be: three coordinates, each
// a sample bounded to ±1, all peaking together. It is the cube's corner — the
// thing this mode exists to stop being the shape of the figure — and it is what
// the bound below has to survive.
const polarMaxLen = 1.7320508

// THE WHOLE MODE RESTS ON THIS. Every map has to return a radius inside the
// unit sphere for every input the app can produce, because polarFitExtent fits
// the camera to exactly the sphere and nothing anywhere clamps afterwards. A
// map that could exceed 1 would put peaks off the screen with no symptom other
// than a figure that occasionally leaves the frame.
func TestPolarRadiusStaysInsideTheSphere(t *testing.T) {
	for m := 0; m < polarMapCount; m++ {
		for _, drive := range []float32{0.2, 1, 2, 10, 1000} {
			for _, r := range []float32{0, 1e-9, 0.001, 0.1, 1, polarMaxLen, 10, 1e6} {
				got := polarRadius(m, r, drive)
				if !(got >= 0 && got <= 1) {
					t.Errorf("map %d drive %v r %v: radius %v is outside [0,1]", m, drive, r, got)
				}
				if math.IsNaN(float64(got)) {
					t.Errorf("map %d drive %v r %v: radius is NaN", m, drive, r)
				}
			}
		}
	}
}

// A zero vector has no direction, so there is nothing for a radius map to
// preserve and the only answer that is not invented is the origin. It matters
// because tanh(d·r)/r and 1/r are both 0/0 there, and a NaN written into the
// vertex buffer is a hole in the trail that GL reports to nobody.
func TestPolarHandlesTheZeroVectorAndRubbish(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for m := 0; m < polarMapCount; m++ {
		for _, r := range []float32{0, -1, nan} {
			if got := polarRadius(m, r, 1); got != 0 {
				t.Errorf("map %d at r=%v: radius %v, want 0", m, r, got)
			}
			if got := polarScale(m, r, 1); got != 0 {
				t.Errorf("map %d at r=%v: scale %v, want 0", m, r, got)
			}
		}
		// An infinite length can only come from an infinite sample, but the
		// answer still has to be a drawable number.
		if got := polarRadius(m, inf, 1); !(got >= 0 && got <= 1) {
			t.Errorf("map %d at r=+Inf: radius %v is outside [0,1]", m, got)
		}
		// A drive knob driven to zero or below by audio modulation must not
		// collapse the figure to a point or divide by nothing.
		for _, d := range []float32{0, -5, nan} {
			got := polarRadius(m, 1, d)
			if !(got > 0 && got <= 1) {
				t.Errorf("map %d at drive=%v: radius %v, want something drawable in (0,1]", m, d, got)
			}
		}
	}
}

// The direction is the reconstruction. Takens' theorem is about the geometry
// of the delay vectors, so a map that changed the angle between two of them
// would be drawing a different manifold and calling it the same one. Every map
// here is one non-negative factor applied to all three coordinates, which is
// what makes that true — checked as an angle rather than as an inspection of
// the code, because the property is the point and the implementation is not.
func TestPolarPreservesDirection(t *testing.T) {
	vecs := [][3]float32{
		{1, 0, 0}, {0.3, -0.7, 0.2}, {-1, 1, -1}, {0.01, 0.02, -0.005}, {1, 1, 1},
	}
	for m := 0; m < polarMapCount; m++ {
		for _, drive := range []float32{0.2, 1, 2, 10} {
			for _, v := range vecs {
				r := float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])))
				s := polarScale(m, r, drive)
				if s < 0 {
					t.Fatalf("map %d drive %v: a negative scale %v flips the figure through the origin", m, drive, s)
				}
				// cos of the angle between v and s·v is 1 for any s > 0.
				out := [3]float32{v[0] * s, v[1] * s, v[2] * s}
				outLen := math.Sqrt(float64(out[0]*out[0] + out[1]*out[1] + out[2]*out[2]))
				dot := float64(v[0]*out[0] + v[1]*out[1] + v[2]*out[2])
				if cos := dot / (float64(r) * outLen); math.Abs(cos-1) > 1e-5 {
					t.Errorf("map %d drive %v v %v: the mapped vector is %v off the original's direction (cos %v)",
						m, drive, v, out, cos)
				}
				// And the length it came out at is the map's answer.
				if want := float64(polarRadius(m, r, drive)); math.Abs(outLen-want) > 1e-5 {
					t.Errorf("map %d drive %v v %v: drawn at radius %v, want %v", m, drive, v, outLen, want)
				}
			}
		}
	}
}

// What each map is FOR, stated as behavior rather than as its formula.
func TestPolarMapsDoWhatTheyAreOffered_For(t *testing.T) {
	const drive = 2

	// tanh and algebraic must be strictly increasing in r: louder draws
	// larger, which is the fixed-scale honesty the Takens mode argues for and
	// this mode does not abandon, it only bounds.
	for _, m := range []int{polarMapTanh, polarMapAlgebraic} {
		prev := float32(-1)
		for r := float32(0.01); r <= 3; r += 0.01 {
			got := polarRadius(m, r, drive)
			if got <= prev {
				t.Errorf("map %d: radius did not increase at r=%v (%v after %v)", m, r, got, prev)
				break
			}
			prev = got
		}
	}

	// Direction only removes loudness completely: two vectors of wildly
	// different length come back the same size, which is the point — what is
	// left on screen is the angular motion the amplitude was hiding.
	quiet := polarRadius(polarMapUnit, 0.001, drive)
	loud := polarRadius(polarMapUnit, polarMaxLen, drive)
	if quiet != loud || quiet != 1 {
		t.Errorf("direction only drew %v and %v; both should be the surface, 1", quiet, loud)
	}
	// And drive is inert there, which descriptions.go tells the user.
	if polarRadius(polarMapUnit, 0.5, 0.2) != polarRadius(polarMapUnit, 0.5, 10) {
		t.Error("drive changed the direction-only map, which has no length left to compress")
	}

	// Algebraic sits below tanh everywhere past the origin at the same drive —
	// that is the "gentler knee" the knob offers, and if it ever stopped being
	// true the two positions would be doing the same job.
	for r := float32(0.05); r <= 3; r += 0.05 {
		a := polarRadius(polarMapAlgebraic, r, drive)
		h := polarRadius(polarMapTanh, r, drive)
		if !(a < h) {
			t.Errorf("at r=%v algebraic (%v) is not below tanh (%v)", r, a, h)
			break
		}
	}

	// Drive is a compression control: more of it draws a given length larger,
	// which is what makes a quiet passage usable without auto-ranging.
	for _, m := range []int{polarMapTanh, polarMapAlgebraic} {
		if !(polarRadius(m, 0.1, 1) < polarRadius(m, 0.1, 4)) {
			t.Errorf("map %d: turning drive up did not push a quiet vector further out", m)
		}
	}
}

// The default drive has to actually show the mode doing something. At 1 the
// maps are so nearly the identity for ordinary program material that the first
// look would be the Takens mode with the corners very slightly rounded — a mode
// that appears to be broken. This pins the reasoning behind the default rather
// than the number: whatever it is, a typical signal must be drawn visibly
// larger than a linear plot would draw it.
func TestPolarDefaultDriveIsVisiblyDoingSomething(t *testing.T) {
	var drive float32
	for _, p := range attractorParams["polar"] {
		if p.ID == "polar-drive" {
			drive = p.Def
		}
	}
	if drive == 0 {
		t.Fatal("the polar mode has no drive knob")
	}
	const typical = 0.1 // an unremarkable RMS for music
	if got := polarRadius(polarMapTanh, typical, drive); got < typical*1.5 {
		t.Errorf("at the default drive %v a typical vector of %v draws at %v — "+
			"near enough the identity that the mode looks like it is doing nothing", drive, typical, got)
	}
	// ...and the loud end still saturates, or it is only a gain control.
	if got := polarRadius(polarMapTanh, polarMaxLen, drive); got < 0.95 {
		t.Errorf("at the default drive %v full scale reaches only %v of the sphere", drive, got)
	}
}

// THE FIT IS THE SPHERE AND NOT THE CUBE. takensFitExtent carries a √3 because
// its three coordinates are three independent samples and the vector reaches
// the cube's CORNER; here the bound is on the length itself, so the reachable
// set is the ball of radius gain exactly and the √3 would be empty space the
// figure can never occupy.
func TestPolarFitIsTheSphereNotTheCube(t *testing.T) {
	for _, gain := range []float32{0.5, 1, 10, 50} {
		fit := polarFitExtent(gain)
		if fit != gain {
			t.Errorf("gain %v: fitted to %v, want the sphere's radius %v", gain, fit, gain)
		}
		if fit >= takensFitExtent(gain) {
			t.Errorf("gain %v: the sphere fit %v is not tighter than the cube fit %v; the whole "+
				"point of bounding the radius is that the corner is not reachable",
				gain, fit, takensFitExtent(gain))
		}
		// Nothing drawn can exceed it, at any map, drive or input.
		for m := 0; m < polarMapCount; m++ {
			for _, drive := range []float32{0.2, 2, 10, 1000} {
				if got := polarRadius(m, polarMaxLen, drive) * gain; got > fit+1e-5 {
					t.Errorf("gain %v map %d drive %v: full scale draws at %v, past the fitted %v",
						gain, m, drive, got, fit)
				}
			}
		}
	}
}

// WHY THE MAP IS APPLIED AFTER THE SPLINE, as arithmetic rather than as an
// assertion in a comment.
//
// A Catmull-Rom is not confined to the hull of its control points: the four
// basis weights sum, in absolute value, to 1 + f − f², which peaks at 1.25.
// So mapping each source point first and interpolating the results would let
// the drawn beam reach 1.25 times the sphere's radius, and the camera fit would
// need a factor that means nothing to anyone. Mapping the interpolated vector
// instead bounds every point that is actually drawn.
func TestCatmullRomBulgesPastControlPointsOnTheSphere(t *testing.T) {
	// Four unit vectors arranged so the outer two oppose the inner two, which
	// is the arrangement that maximizes the overshoot — and is an ordinary
	// thing for audio, where consecutive delay vectors reverse all the time.
	p := [4][3]float32{{-1, 0, 0}, {1, 0, 0}, {1, 0, 0}, {-1, 0, 0}}
	worst := 0.0
	for i := 0; i <= 100; i++ {
		f := float32(i) / 100
		var out [3]float32
		for c := 0; c < 3; c++ {
			out[c] = 0.5 * (2*p[1][c] + (-p[0][c]+p[2][c])*f +
				(2*p[0][c]-5*p[1][c]+4*p[2][c]-p[3][c])*f*f +
				(-p[0][c]+3*p[1][c]-3*p[2][c]+p[3][c])*f*f*f)
		}
		if l := math.Sqrt(float64(out[0]*out[0] + out[1]*out[1] + out[2]*out[2])); l > worst {
			worst = l
		}
	}
	if worst <= 1.0001 {
		t.Fatalf("the spline through unit vectors stayed at %v; if it really cannot leave the "+
			"sphere then generatePolar's ordering argument is wrong and should be rewritten", worst)
	}
	// The bound the weights predict, so a change to takensSmooth's basis would
	// be caught here rather than by a figure quietly leaving the frame.
	if worst > 1.25+1e-6 {
		t.Errorf("the spline reached %v, past the 1.25 the basis weights allow", worst)
	}
}

// The dial, its position names and its ring labels are three lists that have to
// stay the same length and the same order. A name indexing a map it does not
// describe would be a detent pointing at the wrong curve.
func TestPolarMapTablesLineUp(t *testing.T) {
	if len(polarMapNames) != polarMapCount {
		t.Errorf("%d position names for %d maps", len(polarMapNames), polarMapCount)
	}
	if len(polarMapRing) != polarMapCount {
		t.Errorf("%d ring labels for %d maps", len(polarMapRing), polarMapCount)
	}
	if got := paramLabels["polar-map"]; len(got) != polarMapCount {
		t.Errorf("paramLabels has %d positions for %d maps — the dial and the drawing disagree",
			len(got), polarMapCount)
	}
	for _, p := range attractorParams["polar"] {
		if p.ID != "polar-map" {
			continue
		}
		if int(p.Max) != polarMapCount-1 {
			t.Errorf("the map knob runs to %v for %d maps", p.Max, polarMapCount)
		}
		if int(p.Def) < 0 || int(p.Def) >= polarMapCount {
			t.Errorf("the map knob defaults to %v, which is not a map", p.Def)
		}
	}
}

// Audio modulation can drive any registered parameter, and this one indexes a
// table. Anything the modulator produces has to land on a real map — including
// the values a float-to-int conversion is not defined for.
func TestPolarMapSelClampsWhateverModulationDoes(t *testing.T) {
	saved := polarMapF
	defer func() { polarMapF = saved }()
	for _, v := range []float32{
		-1000, -1, -0.4, 0, 0.6, 1, 2, 2.4, 99,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		polarMapF = v
		if i := polarMapSel(); i < 0 || i >= polarMapCount {
			t.Errorf("map = %v selected %d", v, i)
		}
	}
	// And the detents themselves must round to themselves, not to a neighbor.
	for want := 0; want < polarMapCount; want++ {
		polarMapF = float32(want)
		if got := polarMapSel(); got != want {
			t.Errorf("detent %d selected map %d", want, got)
		}
	}
}

// The color source has to know about this mode. audioColorWindow returns a
// per-position window only for modes whose vertices carry aTrailT = m/(nv−1),
// and this one's do; a mode that fills the attribute that way and is NOT listed
// falls through to a flat fill, which with the gradient following the sound can
// be one dark color for the whole trail — a correct figure drawn in black on
// black, which is exactly how the Stereo Embedding came to be reported as
// showing nothing at all.
func TestPolarIsInTheAudioColorSources(t *testing.T) {
	savedRing, savedW := polarRing, polarW
	savedSrc, savedTried := audioSource, audioSourceTried
	defer func() {
		polarRing, polarW = savedRing, savedW
		audioSource, audioSourceTried = savedSrc, savedTried
	}()
	// A source has to be in place before this runs: ensureAudioSource reads
	// window.location for ?wsurl=, and there is no window under Node. Handing
	// it one it already has is the only way in from a test, and it is also
	// what the running app looks like by the time a color window is asked for.
	audioSource, audioSourceTried = stubSource{}, true

	// No audio yet: the flat fill is the honest answer and nothing must panic.
	polarRing, polarW = nil, 0
	if w, _ := audioColorWindow("polar"); w != nil {
		t.Error("a window came back before any audio had been captured")
	}

	// A ring with a full window in it: the mode must be recognized, and the
	// window must be the ring's newest samples rather than an empty slice.
	n, stride := takensWindow(polarWin, 24000, steps)
	span := (n-1)*stride + 2*int(polarTau)
	polarRing = make([]float32, span+1)
	for i := range polarRing {
		polarRing[i] = float32(i%17) / 17
	}
	polarW = len(polarRing)
	w, sr := audioColorWindow("polar")
	if w == nil {
		t.Fatal("the polar mode is not one of audioColorWindow's sources; with the gradient " +
			"following the sound its trail would be filled one flat color")
	}
	if len(w) != n {
		t.Errorf("the color window is %d samples, want the %d source points the trail was drawn from", len(w), n)
	}
	if sr <= 0 {
		t.Errorf("sample rate %d; shortTimeCentroids cannot bin a spectrum without one", sr)
	}
}
