# chaosrack

**An analog attractor computer in the browser** — 30+ chaotic systems
rendered in real time by a Go→WebAssembly core, driven from an
analog-equipment control surface: knobs with fine-trim rings, seven-segment
LED readouts, toggle switches, concentric selector dials. Rotate and zoom the
models, **type your own differential equations**, route live audio into any
parameter, paint with persistence — or flip the Model Out ring and **hear the
attractor itself**. An homage to the analog attractor computers at
[glensstuff.com](https://glensstuff.com).

**Live:** [0magnet.github.io/chaosrack](https://0magnet.github.io/chaosrack/) · [tinygo build](https://0magnet.github.io/chaosrack/tinygo/)

![The control panel docked at the bottom with the Lorenz attractor above it](docs/img/hero.jpg)

## Run

No build step — the WebAssembly is embedded in the server binary:

```
go run github.com/0magnet/chaosrack@master
```

Then open:
* [127.0.0.1:8080/](http://127.0.0.1:8080/) — standard Go build
* [127.0.0.1:8080/tinygo/](http://127.0.0.1:8080/tinygo/) — smaller TinyGo build

(The same entrypoint is also at `github.com/0magnet/chaosrack/cmd/chaosrack`.)

### Serverless

The wasm is inlined (base64) into a single self-contained HTML file, so it
also runs straight from static hosting with no server:
* [index.html](index.html) · [tinygo/index.html](tinygo/index.html)

## What's inside

- **The catalog:** Lorenz, Rössler, Chua, Aizawa, Thomas, Halvorsen, Chen,
  Dadras, Rabinovich–Fabrikant, Burke–Shaw, Lü, Newton–Leipnik, the 4-D
  **hyperchaotic Rössler**, and all of **J. C. Sprott's cases A–S** — plus
  Lissajous, the Graphic Artist (four-oscillator XY art), platonic solids and
  geometric primitives. Every built-in flow's default parameters are
  **certified chaotic in CI** by a Lyapunov-exponent test run under the app's
  own integrator (a periodic-window default shipped once; never again).
- **Custom equation mode:** type `dx/dt`, `dy/dt`, `dz/dt` (and an optional
  4th dimension); every parameter you name automatically gets a control knob.
  `Edit eqn` seeds the editor from any built-in system.
- **Two trail engines:** classic **scan** (the whole curve re-integrates every
  frame, so parameter and audio changes reshape it instantly) and the
  **Ring** switch's scope-style beam (only the advancing head integrates; the
  trail is its history, and knob changes bend the path from the head forward).
  **Persist** accumulates either into a long-exposure painting.
- **Colors module:** gradient source ring (X / Y / Z / trail) × palette ring
  (mono / 2-color / 3-color / animated rainbow), color-wheel knobs for the
  gradient stops and background, CRT **phosphor** presets (P31/P7/P33…) with
  afterglow for the scope modes.
- **Model Out — hear the attractor:** the trail plays through the speakers.
  **FLOW** integrates the same vector field the renderer draws at audio rate,
  so the pitch is the system's *own* orbital frequency — chaos chirps,
  periodic windows lock into tones, parameter changes are audible
  bifurcations — with a RATE knob that transposes in exact musical intervals.
  **SCAN** traces the drawn trail as a wavetable at an exact concert-pitch
  rate. The MAP ring picks the stereo projection (CAM = the screen's x/y, so
  rotating the model changes the sound).
- **Signal generators:** three oscillators (sine/tri/square/saw) on a
  concert-pitch A0–A10 scale with octave dials and a clickable piano-key
  register display, routable to either speaker channel — and usable as the
  audio source for every audio-reactive feature, no server needed.
- **Shareable permalinks** capture the full state — mode, parameters,
  equations, colors, pose, audio-mod routing — in the URL, round-trip-tested.
- **Fit and finish:** the control panel docks to any edge or floats, the
  whole interface scales, knobs come in six styles, a Template module
  documents every module slot, and rendering is devicePixelRatio-native.

## Gallery

![Attract-mode reel — every model, rainbow gradient, Persist on, rotating](docs/img/gif/reel.gif)

### Animated tour

One GIF per model, auto-rotating for depth while walking every distinct
setting of the Colors module's two knobs — 13 of the 16 positions (in
**mono** the source knob has no effect). Each variation plays twice over:
first with the flowing trail, so you can see which way the colors run, then
with **Persist** on, so the accumulation paints the full figure.

- **Variation order:** mono · then 2-color / 3-color / rainbow, each across
  gradient source X · Y · Z · trail

| Lorenz | Rössler |
|---|---|
| ![Lorenz](docs/img/gif/lorenz.gif) | ![Rössler](docs/img/gif/rossler.gif) |

| Chua | Aizawa |
|---|---|
| ![Chua](docs/img/gif/chua.gif) | ![Aizawa](docs/img/gif/aizawa.gif) |

| Sprott | Thomas |
|---|---|
| ![Sprott](docs/img/gif/sprott.gif) | ![Thomas](docs/img/gif/thomas.gif) |

| Halvorsen | Chen |
|---|---|
| ![Halvorsen](docs/img/gif/halvorsen.gif) | ![Chen](docs/img/gif/chen.gif) |

| Dadras | Rabinovich–Fabrikant |
|---|---|
| ![Dadras](docs/img/gif/dadras.gif) | ![Rabinovich–Fabrikant](docs/img/gif/rabinovich.gif) |

| Burke–Shaw | Lü |
|---|---|
| ![Burke–Shaw](docs/img/gif/burkeshaw.gif) | ![Lü](docs/img/gif/lu.gif) |

| Newton–Leipnik | Hyper-Rössler (4-D) |
|---|---|
| ![Newton–Leipnik](docs/img/gif/newtonleipnik.gif) | ![Hyper-Rössler (4-D)](docs/img/gif/hyperrossler.gif) |

| Lissajous | Graphic Artist |
|---|---|
| ![Lissajous](docs/img/gif/lissajou.gif) | ![Graphic Artist](docs/img/gif/graphicartist.gif) |

### The control surface

| Ring trail (scope-style beam) | Signal generators + Model Out |
|---|---|
| ![Ring trail mode with the panel docked](docs/img/ui/ring-trail.jpg) | ![The three generator modules and the Model Out sonification module](docs/img/ui/generators-model-out.jpg) |

| FVF Wobbulator panel | XY oscilloscope (Catmull-Rom beam) |
|---|---|
| ![The FVF Wobbulator's parameter panel](docs/img/ui/fvf-panel.jpg) | ![The XY scope's smoothed Lissajous trace](docs/img/ui/xy-scope.jpg) |

### Chaos monkey jam sessions

The repo ships a CDP-driven fuzzer (`uitool monkey`) that hammers the real UI
with seeded random input while checking invariants (no JS errors, no NaN in
any readout, the panel always recoverable, the app never frozen). Its show-biz
sibling (`uitool demo`) puts specialized monkeys on stage: one tours the model
catalog, one plays the parameter knobs, one recolors, one wires **audio
modulation** so the music drives the knobs, and one works motion and texture —
with a supervisor watching the screen and stepping in if the picture ever goes
dark. These clips are cut from audio-reactive takes scored with a drum & bass
mix (the audio drove every wiggle you see):

![Cuts from chaos-monkey jam sessions — the music modulates the knobs](docs/img/gif/jam.gif)

### Contact sheets (stills)

<details>
<summary><b>Full color-matrix contact sheets</b> — rows: mono / 2-color /
3-color / rainbow · columns: source X / Y / Z / trail</summary>

| Lorenz | Rössler |
|---|---|
| ![Lorenz](docs/img/lorenz.jpg) | ![Rössler](docs/img/rossler.jpg) |

| Chua | Aizawa |
|---|---|
| ![Chua](docs/img/chua.jpg) | ![Aizawa](docs/img/aizawa.jpg) |

| Sprott | Thomas |
|---|---|
| ![Sprott](docs/img/sprott.jpg) | ![Thomas](docs/img/thomas.jpg) |

| Halvorsen | Chen |
|---|---|
| ![Halvorsen](docs/img/halvorsen.jpg) | ![Chen](docs/img/chen.jpg) |

| Dadras | Rabinovich–Fabrikant |
|---|---|
| ![Dadras](docs/img/dadras.jpg) | ![Rabinovich–Fabrikant](docs/img/rabinovich.jpg) |

| Burke–Shaw | Lü |
|---|---|
| ![Burke–Shaw](docs/img/burkeshaw.jpg) | ![Lü](docs/img/lu.jpg) |

| Newton–Leipnik | Hyper-Rössler (4-D) |
|---|---|
| ![Newton–Leipnik](docs/img/newtonleipnik.jpg) | ![Hyper-Rössler (4-D)](docs/img/hyperrossler.jpg) |

| Lissajous | Graphic Artist |
|---|---|
| ![Lissajous](docs/img/lissajou.jpg) | ![Graphic Artist](docs/img/graphicartist.jpg) |

</details>

<details>
<summary><b>The Sprott catalog (cases A–S)</b> — click to expand</summary>

| Sprott A | Sprott B |
|---|---|
| ![Sprott A](docs/img/sprotta.jpg) | ![Sprott B](docs/img/sprottb.jpg) |

| Sprott C | Sprott D |
|---|---|
| ![Sprott C](docs/img/sprottc.jpg) | ![Sprott D](docs/img/sprottd.jpg) |

| Sprott E | Sprott F |
|---|---|
| ![Sprott E](docs/img/sprotte.jpg) | ![Sprott F](docs/img/sprottf.jpg) |

| Sprott G | Sprott H |
|---|---|
| ![Sprott G](docs/img/sprottg.jpg) | ![Sprott H](docs/img/sprotth.jpg) |

| Sprott I | Sprott J |
|---|---|
| ![Sprott I](docs/img/sprotti.jpg) | ![Sprott J](docs/img/sprottj.jpg) |

| Sprott K | Sprott L |
|---|---|
| ![Sprott K](docs/img/sprottk.jpg) | ![Sprott L](docs/img/sprottl.jpg) |

| Sprott M | Sprott N |
|---|---|
| ![Sprott M](docs/img/sprottm.jpg) | ![Sprott N](docs/img/sprottn.jpg) |

| Sprott O | Sprott P |
|---|---|
| ![Sprott O](docs/img/sprotto.jpg) | ![Sprott P](docs/img/sprottp.jpg) |

| Sprott Q | Sprott R |
|---|---|
| ![Sprott Q](docs/img/sprottq.jpg) | ![Sprott R](docs/img/sprottr.jpg) |

| Sprott S |
|---|
| ![Sprott S](docs/img/sprotts.jpg) |

</details>

Regenerate everything against a running tab:
`go run ./cmd/uitool shots` (stills + hero) · `go run ./cmd/uitool gifs` (animations).

## Audio

The spectrogram, XY oscilloscope, FVF Wobbulator, **audio-modulated
attractors**, the **spectrogram skin** (paint the live spectrogram onto any
model), and the spectro/XY **backdrops** all need an audio source:

- **Microphone** (default) — the page uses `getUserMedia`.
- **Signal generators** — flip on the built-in oscillators; fully client-side.
- **System audio over WebSocket** — run the bundled server, which captures
  the default **PulseAudio / PipeWire** monitor and streams it to the page
  (the page auto-connects via `?audio=ws`):

  ```
  go run github.com/0magnet/chaosrack/cmd/audiows      # serve on :8080
  # open http://127.0.0.1:8080/  → redirects to /?audio=ws
  ```

  `cmd/audiows` mirrors the wire format of
  [**0magnet/audioprism-go**](https://github.com/0magnet/audioprism-go) — whose
  spectrogram engine is embedded here (the WebAssembly build effectively
  includes a port of it).

**Audio-modulated attractors:** the Modulation and EQ modules appear beside
each control group when **Audio mod** is on. Route any parameter — or the
view itself (zoom, pan, spin, rainbow, trail) — from a channel (L / R / mono
/ bass / beat / …) with a per-route level knob, or paint frequency bands on
the draggable graphic-EQ strips. Features are adaptively normalized, so the
modulation depth tracks the music's dynamics, not the system volume.

## FVF — Harmonic Wobbulator

The **FVF Wobbulator** mode is a software analog of the
Frequency→Voltage→Frequency converter with balanced modulator designed at
[bunkerofdoom.com](https://bunkerofdoom.com) (hardware built 1984):
it tracks the pitch of the incoming audio, scales/offsets it into a new
carrier frequency, and ring- or AM-modulates that carrier back with the
original signal — the metallic, glitchy timbre that "made guitar/voice sound
very strange." The processed audio is shown on the spectrogram and, with the
**🔊 Listen** switch, played back out. Knobs: gain, offset, fmin, fmax, duty,
mix (dry↔wet), glide, plus waveform (square/pulse/sub-÷2) and modulator
(ring/AM) selectors.

The **🎛 FX** switch is an instant A/B between the raw incoming audio (off) and
the wobbulated signal (on), independent of the `mix` knob; `mix` itself sweeps
dry↔wet continuously.

**Hear it with a microphone** (no setup):
open the page in mic mode (the default — not the `?audio=ws` URL), choose
**FVF Wobbulator**, turn on **Listen**, and use **headphones** (mic-in and
speaker-out are different devices, so there's no loop; headphones stop
acoustic feedback).

**See/hear it react to *any* app (system audio).** `cmd/audiows` captures the
default sink's monitor by default (`-source monitor`), so — exactly like
audioprism — it picks up whatever is playing (VLC, a browser tab, anything)
with no per-app routing:

```
go run github.com/0magnet/chaosrack/cmd/audiows   # -source monitor is the default
# open http://127.0.0.1:8080/?audio=ws  → FVF → (with headphones) Listen
```

That alone is perfect for the spectrogram and for **Listen over headphones**.
To hear the wobbulated result **out the speakers** you must break the feedback
loop (speaker output being re-captured). The `-wobbulate` flag automates it —
it inserts a temporary null sink, makes it the default so *every* app routes
into it, captures that, and restores your default sink on Ctrl-C:

```
go run github.com/0magnet/chaosrack/cmd/audiows -wobbulate
```

Flow: any app → null sink (silent) → its monitor → audiows → browser wobbulates
→ real speakers (never captured, so no loop). The wasm page's *own* Listen
output is **auto-routed to your speakers** — audiows watches for the browser's
playback stream on the null sink and moves it back — so there's no pavucontrol
step (the `-out-apps` flag controls which app names count as "the browser"; the
source app you're wobbulating should be a *different* app, e.g. VLC).

**Reverting** is automatic (Ctrl-C restores the previous default sink and
removes the null sink) and non-destructive; it's runtime-only anyway, so a
logout/reboot also clears it. To tap a specific source instead, pass its name,
e.g. `-source fvf_in.monitor`.

**One-command toggle.** `scripts/fvf` flips the whole thing on and off so you
can switch back and forth:

```
scripts/fvf on      # start -wobbulate in the background (FVF_PORT=8080 default)
scripts/fvf off     # stop it → normal audio restored
scripts/fvf         # toggle
```

## Testing & tooling

The UI is tested against a real browser over the Chrome DevTools Protocol
(`internal/cdp`, no dependencies). One dev binary, `cmd/uitool`, bundles the
harnesses:

| subcommand | what it does |
|---|---|
| `uitool monkey` | seeded random-walk fuzzer with invariants: no JS errors, no NaN in any readout, the panel always recoverable, the main thread never frozen |
| `uitool golden` | drives a fixed matrix of known states and checks render sanity, permalink round-trip idempotency, and golden images |
| `uitool shots` | regenerates the README's hero + contact-sheet gallery |
| `uitool gifs` | regenerates the animated gallery + attract-mode reel |
| `uitool demo` | records chaos-monkey runs — including a performance mode with specialized monkey roles, audio-modulation routing, an on-screen countdown for cueing external audio, and a screen-liveness supervisor |

Native tests cover the pure logic (LED formatting, concert-pitch math, pose
decomposition, spline smoothing, the equation parser) plus the
**chaos guard**: every registered flow's defaults must show a positive
largest Lyapunov exponent long past any periodic-window collapse horizon.

`make lint` runs golangci-lint **twice** — once natively and once under
`GOOS=js GOARCH=wasm`, because the default pass never loads the js/wasm-tagged
files that make up most of the app. CI (GitHub Actions) runs the tests, a
wasm compile gate, both lint passes, and a pinned TinyGo build.

## Build

The Makefile rebuilds both WebAssembly binaries (standard Go **and** TinyGo)
and their matching `wasm_exec.js` runtimes into the `assets/` package, where
they are `//go:embed`-ed by the server:

```
make wasms     # rebuild assets/chaosrack.wasm, assets/chaosrack-tiny.wasm + wasm_exec.js runtimes
make build     # wasms + the native server binary
make pages     # regenerate the self-contained index.html / tinygo/index.html
```

Layout: `cmd/wasm` (the WebAssembly attractor app) · `cmd/chaosrack` &
repo-root `main.go` (the web server) · `cmd/audiows` (PulseAudio→WebSocket
audio server) · `cmd/uitool` (CDP test & capture harnesses) · `assets`
(embedded wasm/js/template) · `pkg/server` · `pkg/attractor` · `pkg/audiosrc`
· `internal/cdp`.

## Related / prior art

Other browser strange-attractor visualizers (most are single-system and/or
JavaScript; this project is distinguished by its WebAssembly core, the large
catalogue incl. all Sprott cases + 4-D hyperchaos, user-editable equations,
and audio-reactive modulation):

- [ibiblio e-notes — Lorenz / Rössler (WebGL, incl. GPU 1M-point)](https://www.ibiblio.org/e-notes/webgl/gpu/chaos/lorenz_gpu.html)
- [Harvard/Knill — animated Lorenz](https://people.math.harvard.edu/~knill/pedagogy/techdemo2/webgl/lorenz.html)
- [daybarr/lorenz-webgl](https://github.com/daybarr/lorenz-webgl)
- [Simulations4All — Lorenz with σ/ρ/β sliders](https://simulations4all.com/simulations/lorenz-attractor-3d)
- [KinnyTools — audio-reactive Lorenz](https://www.kinnytools.com/lorenz-attractor.html)
- [jujiplay — interactive Rössler](https://strange-attractors.jujiplay.com/rossler)
- [GitHub `strange-attractors` topic](https://github.com/topics/strange-attractors)


## Dependency Graph

Made with [goda](https://github.com/loov/goda):

```
# GOOS=js: the wasm app's import edges (cmd/wasm → pkg/attractor → pkg/audiosrc)
# live in js/wasm-tagged files and are invisible to a host-context run
GOOS=js GOARCH=wasm go run github.com/loov/goda@latest graph github.com/0magnet/chaosrack/... | dot -Tsvg -o docs/chaosrack-goda-graph.svg
```

![Dependency Graph](docs/chaosrack-goda-graph.svg "github.com/0magnet/chaosrack Dependency Graph")

## Lines of Code

Made with [gocloc](https://github.com/hhatto/gocloc) (excludes `vendor/`, `node_modules/`, `.git/`):

```
gocloc --not-match-d='(vendor|node_modules|\.git)' .
```

```
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                              79           1179           2773          12771
HTML                             4            246            166           2131
JavaScript                       2            117             82            935
CSS                              1              0            270            354
Markdown                         1             94              0            327
YAML                             1              0             17             76
BASH                             1              7             11             45
Makefile                         1             12             14             39
JSON                             1              0              0             23
-------------------------------------------------------------------------------
TOTAL                           91           1655           3333          16701
-------------------------------------------------------------------------------
```
