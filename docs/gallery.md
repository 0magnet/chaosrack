# Gallery

[← README](../README.md)

## Animated tour

One GIF per model, auto-rotating for depth while walking every distinct
setting of the Colors module's two knobs — 13 of the 16 positions (in
**mono** the source knob has no effect). Each variation plays twice over:
first with the flowing trail, so you can see which way the colors run, then
with **Persist** on, so the accumulation paints the full figure.

- **Variation order:** mono · then 2-color / 3-color / rainbow, each across
  gradient source X · Y · Z · trail

| Lorenz | Rössler |
|---|---|
| ![Lorenz](https://chaosrack.magnetosphere.net/docs/img/gif/lorenz.gif) | ![Rössler](https://chaosrack.magnetosphere.net/docs/img/gif/rossler.gif) |

| Chua | Aizawa |
|---|---|
| ![Chua](https://chaosrack.magnetosphere.net/docs/img/gif/chua.gif) | ![Aizawa](https://chaosrack.magnetosphere.net/docs/img/gif/aizawa.gif) |

| Sprott | Thomas |
|---|---|
| ![Sprott](https://chaosrack.magnetosphere.net/docs/img/gif/sprott.gif) | ![Thomas](https://chaosrack.magnetosphere.net/docs/img/gif/thomas.gif) |

| Halvorsen | Chen |
|---|---|
| ![Halvorsen](https://chaosrack.magnetosphere.net/docs/img/gif/halvorsen.gif) | ![Chen](https://chaosrack.magnetosphere.net/docs/img/gif/chen.gif) |

| Dadras | Rabinovich–Fabrikant |
|---|---|
| ![Dadras](https://chaosrack.magnetosphere.net/docs/img/gif/dadras.gif) | ![Rabinovich–Fabrikant](https://chaosrack.magnetosphere.net/docs/img/gif/rabinovich.gif) |

| Burke–Shaw | Lü |
|---|---|
| ![Burke–Shaw](https://chaosrack.magnetosphere.net/docs/img/gif/burkeshaw.gif) | ![Lü](https://chaosrack.magnetosphere.net/docs/img/gif/lu.gif) |

| Newton–Leipnik | Hyper-Rössler (4-D) |
|---|---|
| ![Newton–Leipnik](https://chaosrack.magnetosphere.net/docs/img/gif/newtonleipnik.gif) | ![Hyper-Rössler (4-D)](https://chaosrack.magnetosphere.net/docs/img/gif/hyperrossler.gif) |

| Lissajous | Graphic Artist |
|---|---|
| ![Lissajous](https://chaosrack.magnetosphere.net/docs/img/gif/lissajou.gif) | ![Graphic Artist](https://chaosrack.magnetosphere.net/docs/img/gif/graphicartist.gif) |

## Chaos monkey jam sessions

The repo ships a CDP-driven fuzzer (`chaosrack uitool monkey`) that hammers the real UI
with seeded random input while checking invariants (no JS errors, no NaN in
any readout, the panel always recoverable, the app never frozen). Its show-biz
sibling (`chaosrack uitool demo`) puts specialized monkeys on stage: one tours the model
catalog, one plays the parameter knobs, one recolors, one wires **audio
modulation** so the music drives the knobs, and one works motion and texture —
with a supervisor watching the screen and stepping in if the picture ever goes
dark. These clips are cut from audio-reactive takes scored with a drum & bass
mix (the audio drove every wiggle you see):

![Cuts from chaos-monkey jam sessions — the music modulates the knobs](https://chaosrack.magnetosphere.net/docs/img/gif/jam.gif)

## Contact sheets (stills)

<details>
<summary><b>Full color-matrix contact sheets</b> — rows: SRC off / 2-color /
3-color / hue sweep · columns: source X / Y / Z / trail. The first row is the
flat trace, which the MAP knob cannot change — it was the "mono palette" until
mono moved onto the source knob, where the absence of a value belongs.</summary>

| Lorenz | Rössler |
|---|---|
| ![Lorenz](img/lorenz.jpg) | ![Rössler](img/rossler.jpg) |

| Chua | Aizawa |
|---|---|
| ![Chua](img/chua.jpg) | ![Aizawa](img/aizawa.jpg) |

| Sprott | Thomas |
|---|---|
| ![Sprott](img/sprott.jpg) | ![Thomas](img/thomas.jpg) |

| Halvorsen | Chen |
|---|---|
| ![Halvorsen](img/halvorsen.jpg) | ![Chen](img/chen.jpg) |

| Dadras | Rabinovich–Fabrikant |
|---|---|
| ![Dadras](img/dadras.jpg) | ![Rabinovich–Fabrikant](img/rabinovich.jpg) |

| Burke–Shaw | Lü |
|---|---|
| ![Burke–Shaw](img/burkeshaw.jpg) | ![Lü](img/lu.jpg) |

| Newton–Leipnik | Hyper-Rössler (4-D) |
|---|---|
| ![Newton–Leipnik](img/newtonleipnik.jpg) | ![Hyper-Rössler (4-D)](img/hyperrossler.jpg) |

| Lissajous | Graphic Artist |
|---|---|
| ![Lissajous](img/lissajou.jpg) | ![Graphic Artist](img/graphicartist.jpg) |

</details>

<details>
<summary><b>The Sprott catalog (cases A–S)</b> — click to expand</summary>

| Sprott A | Sprott B |
|---|---|
| ![Sprott A](img/sprotta.jpg) | ![Sprott B](img/sprottb.jpg) |

| Sprott C | Sprott D |
|---|---|
| ![Sprott C](img/sprottc.jpg) | ![Sprott D](img/sprottd.jpg) |

| Sprott E | Sprott F |
|---|---|
| ![Sprott E](img/sprotte.jpg) | ![Sprott F](img/sprottf.jpg) |

| Sprott G | Sprott H |
|---|---|
| ![Sprott G](img/sprottg.jpg) | ![Sprott H](img/sprotth.jpg) |

| Sprott I | Sprott J |
|---|---|
| ![Sprott I](img/sprotti.jpg) | ![Sprott J](img/sprottj.jpg) |

| Sprott K | Sprott L |
|---|---|
| ![Sprott K](img/sprottk.jpg) | ![Sprott L](img/sprottl.jpg) |

| Sprott M | Sprott N |
|---|---|
| ![Sprott M](img/sprottm.jpg) | ![Sprott N](img/sprottn.jpg) |

| Sprott O | Sprott P |
|---|---|
| ![Sprott O](img/sprotto.jpg) | ![Sprott P](img/sprottp.jpg) |

| Sprott Q | Sprott R |
|---|---|
| ![Sprott Q](img/sprottq.jpg) | ![Sprott R](img/sprottr.jpg) |

| Sprott S |
|---|
| ![Sprott S](img/sprotts.jpg) |

</details>

Regenerate everything against a running tab:
`go run . uitool shots` (stills + hero) · `go run . uitool gifs` (animations).


## Explorations

Things found by playing with the rack, written up with how to get there.

- [A Lorenz that steers itself](self-steering-lorenz.md): ρ routed from the
  trajectory's own height pulses, chaos in bursts a few seconds apart.
- [Attractors found by search](searched-attractor.md): a five-term
  chaotic flow with a line of equilibria, from a random search of small
  systems, and a six-term one with two lobes; both run in Custom.
- [Aizawa: standing colors and their harmonics](aizawa-harmonics.md):
  symmetric mandalas of 3 to 8 petals whose lines, colors or both stand
  still, with one to three rainbows outward or around.
