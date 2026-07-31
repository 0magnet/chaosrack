//go:build js && wasm

package attractor

// Takens delay embedding — the mode that turns LIVE AUDIO into an attractor.
// Takens' theorem (1981): the delay vector (s(t), s(t−τ), s(t−2τ)) of a
// single observable reconstructs a manifold diffeomorphic to the source
// system's attractor. Feed it a pure tone and you get a closed loop (a
// Lissajous-like ellipse whose shape is set by τ); feed it music, speech or
// a chaotic circuit's output and you see the geometry of whatever generated
// the signal, live. The reconstructed trail rides the NORMAL 3D pipeline —
// rotate, zoom, persist, gradient, beam-dwell, even Model Out SCAN all work.
//
// The τ knob is in samples of the source stream. Rule of thumb: a quarter
// period of the dominant frequency (τ ≈ sr/(4·f)); too small stretches the
// figure along the diagonal, too large folds it. The Speed knob sets the
// stride (samples per drawn point), i.e. how much time the trail spans.

var (
	takensTau     float32 = 32 // delay τ, in source samples
	takensGain    float32 = 10 // audio (±1) → world units
	takensRing    []float32
	takensW       int // monotonic write cursor into takensRing
	takensScratch []float32
	takensFitted  bool // camera auto-fitted since real audio arrived
)

func init() {
	registerGenerate("takens", generateTakens)
	attractorParams["takens"] = []paramDef{
		{"takens-tau", "τ", &takensTau, 32, 1, 512, 1},
		{"takens-gain", "gain", &takensGain, 10, 0.5, 50, 0.5},
	}
	attractorDescriptions["takens"] = "Takens Delay Embedding — attractor reconstruction " +
		"from a single signal (F. Takens, \"Detecting strange attractors in turbulence\", 1981). " +
		"Each trail point is the delay vector (s(t), s(t−τ), s(t−2τ)) of the live audio: a pure " +
		"tone draws a closed loop, music and speech trace the geometry of whatever produced them. " +
		"τ is the embedding delay in samples (≈ a quarter period of the dominant frequency works " +
		"well); GAIN scales the audio into world units; the Speed knob sets how many samples the " +
		"beam advances per drawn point, i.e. how much time the trail spans. Audio comes from the " +
		"active source — websocket stream, microphone, or the signal generators."
}

// generateTakens drains the audio source into a persistent ring and draws the
// newest span of delay vectors. When there's no (or not yet enough) audio the
// previous frame is re-uploaded, so the model doesn't flicker while the
// source spins up.
func generateTakens() {
	src := ensureAudioSource()
	tau := int(takensTau)
	if tau < 1 {
		tau = 1
	}
	stride := speedSteps
	if stride < 1 {
		stride = 1
	}
	span := (steps-1)*stride + 2*tau
	if need := span + 1; len(takensRing) < need {
		takensRing = make([]float32, need+need/2)
		takensW = 0
	}
	if takensScratch == nil {
		takensScratch = make([]float32, 8192)
	}
	if src != nil && src.Ready() {
		// Per-frame drain CAP, not drain-until-dry: the function generator
		// synthesizes on demand and always fills the buffer, so an uncapped
		// loop never terminates (froze the page on the first fg-on test).
		for drained := 0; drained < 16384; {
			n := src.Drain(takensScratch)
			if n <= 0 {
				break
			}
			for i := 0; i < n; i++ {
				takensRing[takensW%len(takensRing)] = takensScratch[i]
				takensW++
			}
			drained += n
			if n < len(takensScratch) {
				break
			}
		}
	}
	avail := takensW
	if avail > len(takensRing) {
		avail = len(takensRing)
	}
	if avail < span+1 {
		takensFitted = false // camera was fitted to silence — refit on real data
		uploadVerticesOnly(vertBuf[:steps*4], attractorDrawMode, steps)
		return
	}
	rn := len(takensRing)
	base := takensW - 1 - span // oldest sample the trail needs
	g := takensGain
	invN := float32(1) / float32(steps-1)
	vertices := vertBuf[:steps*4]
	for i := 0; i < steps; i++ {
		k := base + 2*tau + i*stride
		j := i * 4
		vertices[j] = takensRing[k%rn] * g
		vertices[j+1] = takensRing[(k-tau)%rn] * g
		vertices[j+2] = takensRing[(k-2*tau)%rn] * g
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
	if !takensFitted {
		// The mode-entry auto-fit saw silence (a dot); refit once the first
		// full span of real audio is on screen.
		takensFitted = true
		autoFitCamera()
	}
}
