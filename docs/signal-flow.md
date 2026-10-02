# chaosrack's internal wiring, for whoever changes it

The signal flow as the panel presents it — sources, the Mixer, the speakers,
the rack's signal and everything that reads it — is in the manual:
[manual/routing.md](../manual/routing.md). This page is the code underneath
it.

## Where each part is

| part | code |
|---|---|
| the Mixer's pins, its Web Audio graph, the return tap | `pkg/attractor/mixer_js.go` |
| the rack's signal: CAP, GEN and MODEL mixed, plus the return | `pkg/audiosrc/bus_js.go` (`Bus`) |
| tapping a Web Audio node back into Go | `pkg/audiosrc/tap_js.go` (`NewTap`) |
| the generators: Go for the rack's signal, Web Audio for the speakers | `pkg/audiosrc/funcgen_js.go`, `pkg/attractor/gen_js.go` |
| Model Out: three copies of the model (heard, the rack's, the scopes') | `pkg/attractor/sonify_js.go` |
| the scopes' window on the bus | `pkg/attractor/rackscope_js.go` (`drawRackScopes`) |
| the modulation matrix | `pkg/attractor/modmatrix_js.go`, `audiomod_js.go`, `modelmod.go` |

The bus mixes the sources Go makes itself; the instruments that only exist in
Web Audio (keys, drums, tone matrix, the models' own sounds) are summed in
Web Audio on the Mixer's RACK gains and come back through one tap — the
internal return the rack lacked when this page was first written, when the
only way from the output back to the analysis was the host's monitor mix.

The bus has three clocks. The capture and the return have a backlog, and
whichever of them is in the mix (the capture first) decides how many samples
a drain is; the generators and the model are made to match. A return at a
different rate from the capture is resampled.

## The tap is a distribution amplifier, and it was added for the reason one is

`Source.Drain` hands each sample to its caller **exactly once** — the
overlapping STFT needs that. Two consumers on one source therefore do not
each see the stream, they *split* it. That is bridging two devices across one
output and loading it down, and it had the symptom you would expect: the
spectrogram backdrop drained everything available before the model generated,
so Takens got nothing and the attractor sat frozen while the backdrop
scrolled behind it.

`audiotap_js.go` drains once per frame into a pair of rings and gives every
consumer its own read cursor. It also carries **both channels**, because the
fold to mono is a consumer's choice — a module that wants side cannot get
there from a stream already summed. A mono source writes both rings, so
asking for side of a mono source correctly gives silence.

## The modulation matrix is a CV bus with a pin matrix

A route is a source, a destination and a signed depth. The value is applied
for one integration step and **put straight back**, so the knob remains the
base value and the CV swings around it — which is why turning a route off
gives you your setting back rather than wherever the modulation left it.

This is the same mechanism the grid sweep and per-control Link use: a
parameter's value for this pass comes from somewhere other than its knob.
Three features, one idea, and they should share the path.

