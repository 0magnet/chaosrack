# chaosrack as hardware: the internal wiring

This is a reverse-derived block diagram. Nothing here is aspirational — every
box is code that exists, named so you can find it. It is written in hardware
terms because the rack is the metaphor the whole interface is built on, and
because the exercise turns out to expose one missing wire and several missing
controls.

## What kind of instrument this is

A 3U rack holding a **chaotic signal generator** whose output is a
three-vector over time, an **analysis bay** metering an audio input, a **CV
matrix** between them, and a **CRT** displaying the generator. It has an
audio input and an audio output, and — this is the part worth knowing — they
are not connected to each other inside the box.

## The block diagram

```
      EXTERNAL                    THE RACK
   ┌───────────┐
   │ microphone│──┐
   └───────────┘  │   ┌──────────────┐
   ┌───────────┐  ├──▶│ INPUT SELECT │   pkg/audiosrc
   │ server    │──┤   │  one source  │   mic_js / ws_js / wt_js
   │ (monitor) │  │   │  at a time   │   funcgen_js / testsig
   └───────────┘  │   └──────┬───────┘
   ┌───────────┐  │          │ Drain: each sample delivered ONCE
   │ test gen  │──┘          ▼
   └───────────┘      ┌─────────────┐
                      │   THE TAP   │   audiotap_js.go
                      │ dist. amp,  │   drains once per frame into L/R rings,
                      │ per-consumer│   every consumer has its own cursor
                      │   cursors   │   fold (mix/L/R/side) chosen on READ
                      └──┬───┬───┬──┘
            ┌────────────┘   │   └────────────┐
            ▼                ▼                ▼
   ┌────────────────┐ ┌─────────────┐ ┌──────────────┐
   │ ANALYSIS BAY   │ │ THE SCOPE   │ │ AUDIO-EMBED  │
   │ loudness (LUFS)│ │ rackscope   │ │ MODES        │
   │ RTA, EQ bands  │ │ own tube,   │ │ takens,      │
   │ THD, wow/flut  │ │ timebase,   │ │ stereo, xy,  │
   │ correlation    │ │ trigger     │ │ recurrence   │
   │ transfer, spect│ └─────────────┘ └──────┬───────┘
   └───────┬────────┘                        │
           │ features: channel × band curve → one scalar
           ▼
   ┌────────────────────────────────────────────────┐
   │ MODULATION MATRIX (CV)     audiomod_js.go      │
   │   sources: audio features, MODEL x/y/z/r       │  modelmod.go
   │   dests:   every attractorParams[mode] entry,  │
   │            + viewModTargets (zoom, pan, spin,  │
   │              period, trail)                    │
   │   each route: signed depth, applied per step   │
   │               and RESTORED                     │
   │   patchbay = pin matrix over sources × dests   │  patchbay_js.go
   │   MIDI CC 21+i → viewModTargets[i]             │  midi_js.go
   └───────────────────────┬────────────────────────┘
                           ▼
   ┌────────────────────────────────────────────────┐
   │ THE GENERATOR — the instrument proper           │
   │   attractorParams[mode]  the panel constants    │
   │   generateForMode()      integrates/evaluates   │
   │   vertBuf                the trail; its head is │
   │                          the current state      │
   └───────┬─────────────────────────┬───────────────┘
           │                         │
           ▼                         ▼
   ┌────────────────┐        ┌──────────────────┐
   │ DEFLECTION+CRT │        │ MODEL OUT        │  sonify_js.go
   │ camera, views/ │        │ x,y,z → L,R      │
   │ grid, sweep,   │        │ map: xy/xz/yz/cam│
   │ colour, phosph │        └────────┬─────────┘
   │ graticule      │                 │
   └────────────────┘                 │
                                      ▼
   ┌────────────────────────────────────────────────┐
   │ OUTPUT BUS → AudioContext destination → card   │
   │   Gen X / Y / Z, Keys, Rhythm, Tone matrix,    │
   │   Test signal, Model Out                       │
   │   each with its own out select: off / L / R /  │
   │   L+R                                          │
   └───────────────────────┬────────────────────────┘
                           ▼
                      loudspeakers
```

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

## The wire that is not there

**Nothing connects the output bus back to the input.** The generators and
Model Out go to the `AudioContext` destination; the analysis bay reads
whatever `pkg/audiosrc` is capturing. The only path from the rack's output to
its input runs *outside the rack*: the server captures the operating system's
monitor mix (`--audio-source monitor`) and streams it back in.

That is a real patch cable, and it is why the tone matrix visibly steadies
the goniometer: its output leaves through the sound card and returns through
the monitor. It works, and it is authentic — a rack with an out bus and an in
bus that you normal together with a cable is a completely ordinary rack. But
right now it is an accident of how the machine is configured rather than
something the panel says.

## Controls that fall out of the diagram

1. **INPUT: EXT / BUS / EXT+BUS.** A normalling switch that sums the rack's
   own output bus into the analysis input internally. Makes the feedback loop
   a feature of the instrument instead of a property of the host's audio
   configuration, and makes it work when the server is not capturing a
   monitor.
2. **A monitor section.** Every generator has its own out select; there is no
   master level, mute, or output meter. Every rack has one.
3. **Per-analyzer input select.** The tap can fold to mix / L / R / side per
   consumer, but only some modules expose the choice. It should be the same
   knob in the same place on each — see the cell work in `panel.css`.
4. **Attenuverter, offset and slew per CV route.** A route has depth only.
   Real CV has attenuation, an offset, and a slew limiter; `modelModCoef` is
   a fixed slew that should be a knob, especially on the model-feedback
   sources where it is what keeps the loop from oscillating at frame rate.
5. **Summing on the CV bus.** One source per destination today. Racks sum.

## What this says about the source layout

`pkg/attractor` is **271 files and 63,904 lines — 88% of the codebase.** It
contains every section of the diagram at once: front end, tap, analysis,
modulation, generator, display and output. The other packages
(`audiosrc`, `rackspec`, `meshstl`, `server`) are the parts that were
already separable and got separated.

The diagram suggests where the seams are, roughly in order of how cleanly
they would come out:

| section | today | would be |
|---|---|---|
| the tap | `attractor/audiotap_js.go` | `pkg/audiobus` |
| analysis | `attractor/loudness.go`, `rta.go`, `distortion.go`, `transfer.go`, `recurrence.go`, `wowflutter.go` | `pkg/analyze` — mostly pure DSP already, and already the best-tested code here |
| modulation | `attractor/audiomod_js.go`, `modelmod.go`, `patchbay_js.go` | `pkg/modmatrix` |
| output | `attractor/sonify_js.go`, `gen_js.go`, `keys_js.go`, `rhythm_js.go`, `tonematrix_js.go` | `pkg/gen` |
| panel | `attractor/panel*.go`, `knobs_js.go`, `rack*.go` | `pkg/panel` |

The analysis DSP is the obvious first move: it is already pure, already
tested without a browser, and has no reason to sit in the same package as
the WebGL pipeline.
