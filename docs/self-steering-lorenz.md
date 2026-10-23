# A Lorenz that steers itself

[← Gallery](gallery.md)

A Lorenz attractor whose ρ follows its own height. Fed back strongly
enough, it stops switching wings at random and **pulses** instead: a few
seconds of quiet spiraling in one wing, then a break across to the other,
then quiet again.

[Open it](https://chaosrack.magnetosphere.net/#lorenz&ar=0&m.r=mz~0,0,0,0,0,0,0,0~0.38)
— or on your own server, append `#lorenz&ar=0&m.r=mz~0,0,0,0,0,0,0,0~0.38`.

## What it is

ρ is the Lorenz system's drive: how hard the convection it models is heated.
Below ρ ≈ 24.74 the system is not chaotic; it spirals into one wing and
stays there. At the usual 28 it is chaotic, circling one wing a few times
and jumping to the other at moments no one can predict.

Here ρ is not a constant. A route in the Mod matrix from **HEAD z** (where
the trajectory is drawn now, its height) to **ρ** moves ρ with the
trajectory's own z, at a depth of 0.38:

- high in the figure, ρ rises — the system is driven harder and spirals
  outward further;
- low, ρ falls, below the threshold where chaos stops.

The loop is smoothed (a HEAD source follows the model through a filter,
about a quarter of a second), so ρ answers z late.

## What it does

Measured over five minutes of simulated 60 Hz frames
(`pkg/attractor/selfsteer_test.go`): a quiet spell is a second or more
without a wing switch.

| depth | wing switches | quiet spells | median spell |
|---|---|---|---|
| 0 (plain Lorenz) | 541 | 61 | 1.3 s |
| 0.30 | 399 | 95 | 1.4 s |
| 0.33 | 154 | 75 | 3.2 s |
| 0.36 | 110 | 62 | 4.1 s |
| 0.38 | 122 | 57 | 4.4 s |
| 0.40 | 134 | 63 | 4.3 s |
| 0.42 | 182 | 61 | 4.1 s |

Between 0.30 and 0.33 the behavior changes, not gradually: the median quiet
spell more than doubles and settles on a plateau around 4 seconds. Each
cycle, ρ climbs from about 22 to about 35 while the orbit winds outward in
one wing, the orbit is thrown across, z collapses, ρ drops back, and the
next quiet spell starts. The Lyapunov readout stays positive: it is still
chaos, but chaos with a pulse.

**The delay matters as much as the depth.** At depth 0.38:

| the feedback's filter | wing switches in 290 s | median spell |
|---|---|---|
| half as fast | 330 | 1.7 s |
| as built | 122 | 4.4 s |
| twice as fast | 0 | — |

Too slow, and ρ is near its average and the system is the ordinary Lorenz.
Too fast, and the feedback catches every excursion before it can grow: the
orbit is held in one wing for good. The pulse lives between — the regime of
delayed-feedback control of chaos, where a lag in the loop decides whether
the chaos survives. (Before the rack's followers kept time rather than
frames, a 144 Hz display ran this filter 2.4 times as fast as a 60 Hz one,
which is past the point where the pulse disappears.)

## Reproducing it by hand

1. MODEL: **Lorenz**.
2. Auto-rotate off (it reads far better still: the two wings stay where they
   are, and the quiet spiral and the break are easy to tell apart).
3. In the **Mod** matrix (routing bay), press the **z** pin under **HEAD**
   on the **ρ** row. ρ's knob is outlined in gold: it is routed, and it
   still reads 28, its setting — the route moves what the model is given.
4. Set **DPTH** to **0.38**. SHAPE stays **push**.

To see the cycle, shorten TRAIL so each quiet spell and each break stand
out.

## What does not pulse

The same scan (200 s each, depths −0.4 to +0.4) on the other routes:

- **z → ρ at a negative depth** (high in the figure lowers the drive): it
  switches wings *more* often, a median gap of 0.45 s against the plain
  Lorenz's 0.5. The pulse needs the feedback to be positive.
- **z → σ, z → β**: ordinary chaos at every depth; the switching rate
  hardly moves.
- **head r → ρ, σ or β**: r hardly varies once smoothed, so the route mostly
  shifts the constant, and at the ends shifts it into a stable point (β 4.9
  at +0.4, ρ 19 or σ 3.7 at −0.4) where the switching stops for good.

So it is the height fed back into the drive, and fed back positively, that
makes the pulse. DPTH 0.30, then 0.33, is the place to watch it appear.

The simulation stands in for the page's head normalization (it centers z at
24 across a reach of 26); timings on the page differ a little with the
figure's fit, but the change near 0.33 and the plateau after it are there.
