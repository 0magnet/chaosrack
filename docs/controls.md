# Which control does what, on which model

[← README](../README.md)

Every knob on the rack is always there, because the rack is a fixed panel and a
control that comes and goes is a control nobody learns. The cost is that some of
them do nothing on some models and the panel does not say so. This is the map.

## The global controls

| Control | What it sets | Where it does nothing |
|---|---|---|
| **Speed** (View) | The integration rate: sub-steps per frame at ×1 and above, a dt multiplier below it | Everything that is not integrated — the maps, all four audio-driven models (XY Scope, Takens, Stereo, Polar), Spectrogram, FVF, Recurrence, Bifurcation, Scope Clock, the polyhedra and the solids |
| **Trail** (Colors) | The **vertex budget**: how many points a model may draw into. On an integrated model that is also a length; on an audio embedding it is a resolution (below) | Hidden on the modes that are a texture rather than a trail — Spectrogram, XY Scope, FVF, Recurrence |
| **Persist** (Trace) | Stops the color buffer being cleared, so successive frames pile up | Forced off while a backdrop visualizer is painting, and while the Fore knob is splitting the draw across two canvases |
| **Points** (Trace) | Draws the trail as separated points rather than a joined line | The texture-plane modes |
| **Line** (Display) | `gl.lineWidth`, which most browsers and drivers cap at 1 — so usually nothing, anywhere | — |
| **Phosphor** (Style) | Trace color plus per-channel afterglow; on an integrated model, also a short advancing beam instead of the whole curve | The afterglow needs a frame that is not cleared, so the audio modes (Spectrogram, FVF, Recurrence) take the color and not the glow — the XY Scope has its own **glow** knob instead. The beam is skipped on the three audio embeddings, where the budget is a resolution and shortening it would decimate the window rather than the figure |
| **src** (Colors) | Which quantity the gradient follows: an axis, trail age, or the sound | `aud` needs an audio source. Where the trail is a time axis it paints the spectrum ALONG the figure; everywhere else it tints the whole figure with one feature |
| **Zoom / X / Y** (Position) | The camera | Fixed face-on on the texture-plane modes |

**Trail is not a duration on the audio embeddings.** On an integrated model it is
both — more points is more trajectory. On Takens, Stereo and Polar the span comes
from **WIN**, and Trail only decides how finely that span is sampled. A long WIN
against a small Trail decimates, and the trace is then spline-smoothed through
samples further apart than the signal's own detail.

## The audio-driven models

**XY Scope**, **Takens Embedding**, **Stereo Embedding** and **Polar Embedding**
all read the live audio source, and none of them integrate anything — so **Speed
is inert on all four**.

The XY Scope's deflection is squared against the shorter side of the canvas, so
a correlated pair reads at 45° and a quarter-cycle phase difference draws a
circle whatever shape the window is. Its own grid:

| | |
|---|---|
| **axes** | L/R, or mid/side — the 45° rotation broadcast goniometers ship in, where width is an extent along one axis rather than the eccentricity of a tilted ellipse |
| **gain** | deflection |
| **win** | time base, ms — short for the instantaneous phase relationship, long for the width of a whole mix |
| **glow** | afterglow. At zero the frame is cleared as it always was; above it the trace decays instead, which is the control a hardware vectorscope is actually used through |
| **lag** | how far a mono source is delayed against itself, ms |
| **smth** | Catmull-Rom steps per sample — how hard the beam is slew-limited |
| **corr** | correlation meter, −1…+1, measured on L against R whatever **axes** is set to |

The other three share a grid:

| | Takens | Stereo | Polar |
|---|---|---|---|
| **src** | which signal of the live pair is embedded: mix, left, right, mid or **side** — what the channels do *not* share, which on a real mix is the room rather than the instruments | — | the same |
| **τ** | delay, in samples at a 48 kHz reference (so: a duration) | the same, on the two delay positions only | the same |
| **win** | span of the figure, ms | span, ms (capped at 250 — the snapshot cannot outrun the source's ring) | span, ms |
| **gain** | world units per full-scale sample | the same | the sphere's radius |
| **axes** | — | which signal each of x, y, z carries | — |
| **algn** | — | inter-channel delay, ±2 ms. Dial an offset out until the figure collapses onto the diagonal and it reads how far apart the channels were | — |
| **wide** | — | side against mid, applied to the drawing rather than the audio — what widening would do, and what would collapse in mono | — |
| **map / drv** | — | — | the radius curve and how hard the length is driven into it. **dB** is a decibel radius: full scale at the surface, **drv** the window in decades below it |
| **meas / corr** | measures τ and the FNN dimension — once by itself, then on the button | correlation meter, −1…+1, read on the source as it is whatever **algn** and **wide** are doing | — |

**τ is counted in samples at a fixed 48 kHz reference**, so one knob position is
one *duration*: the same delay whether the sound is arriving from a 48 kHz
microphone or a 24 kHz server feed. Read as raw samples — which is what it was —
the same setting was twice the delay on the feed that it was on the microphone,
and a permalink meant different things on two machines with nothing saying so.
The default of 72 is 1.5 ms, and the MEAS readout gives the current setting in
milliseconds.

The Takens mode **measures τ by itself, once**, as soon as it has enough audio —
and again if the source is swapped, because τ is a property of what is playing.
Once is the whole of it: nothing re-tunes per frame, because a knob that moves
with the music makes the figure move with it. Press **MEAS** to measure again,
or turn the knob and it stays where you put it. It is worth pressing after a
change of material: the estimator asked for 1.52 ms on a three-tone signal and
the difference between that and a τ that is too short is the difference between
an open reconstruction and a streak along the diagonal.



## Controls that pull in the same direction

Several pairs change the same thing on screen by different means, which is worth
knowing before reaching for the second one.

- **GAIN and Zoom** both make the figure bigger. GAIN scales the model against
  the fixed bound the camera is framed to; Zoom moves the camera. Turning GAIN
  re-frames, so the figure keeps its place and changes its size relative to what
  full scale can reach; Zoom leaves the model alone and changes how much of the
  frame you are looking at. Reach for Zoom unless it is the headroom you want to
  change.
- **WIN and Trail** both change how much is on screen: WIN adds signal, Trail
  adds resolution. A dense tangle wants less WIN; a coarse, angular figure wants
  more Trail.
- **WIN and Persist** both make the picture denser. WIN draws one longer window;
  Persist superimposes many different ones. A drifting figure fills in under
  Persist and merely blurs under WIN.
- **τ and AXES** (Stereo) both change the figure's shape, but τ is inert on the
  two `time` positions — there is no delay coordinate there to delay.
- **DRV and GAIN** (Polar) both change how large a loud passage draws. DRV moves
  where the radius curve saturates; GAIN sets the size of the sphere it
  saturates against. DRV does nothing on the `unit` map, where the length has
  already been thrown away.

