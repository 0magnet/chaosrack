# Aizawa: standing colors and their harmonics

[← Gallery](gallery.md)

Viewed end-on, the Aizawa attractor can be made into a mandala: a rosette of
loops, colored in rings, where the lines, the colors, or both stand still.
Something that stands still makes a good base for audio modulation, because
whatever the sound does to it is all that moves.

Every link below opens end-on (`rot=0,0,0`) with auto-rotate off. Add
`rot=70,0,0` instead to see it side-on as a globe with a vortex through it.
The view scales to fit, so zoom out if the rosette fills the screen.

## How it works

**The color.** With the color source on **trail** and the **hue** map, each
point is colored by its place in the trail: hue = period × i / (N − 1), for
point i of a trail of N. In the default scan mode the rack draws N new
points every frame, carrying on from where the last frame ended, and the hue
advances 0.003 every frame on top of that.

**The orbit.** Aizawa spirals out over a sphere and drops back down a vortex
through the middle. Call one of those an excursion. It takes L points and
turns W times around the axis, about three. With **f = 0** the attractor is
symmetric about its axis: every excursion is the same as the last, only
rotated, so the end-on view is radially symmetric. If W is a fraction p/q,
the orbit closes after q excursions, T = q·L points, and draws a q-petal
rosette. The lines then stand still: every frame draws the same closed
curve.

**Standing colors.** Give the hue k cycles over the closed orbit (period
F = k(N − 1)/T). From one frame to the next, the color at any spot on the
curve shifts by 0.003 − k·N/T (mod 1). When that is zero the colors stand
still. When it is small they flow; the "flowing" links below shift 1/300 per
frame, one full cycle every 5 seconds at 60 fps. This holds at any frame
rate; the frame rate only sets how fast any leftover drift goes.

**The harmonics.** For all the orbits here, W = 3 + 1/q, so each excursion
lands one petal further round. Then k = m·q + j gives:

- **m rainbows from the center outward,** because each excursion runs from
  the center to the rim and back;
- **j rainbows around the circle,** one color per petal, in order.

Mixing the two makes a rainbow spiral. A negative j winds the other way.

**Finding the values.** The knobs snap to fine steps (a, d and f to 1e-5, dt
to 1e-7, period to 5e-5), and the trail is a whole number. Those snaps are
coarse enough to make a "still" mandala turn within seconds. Every link was
therefore found by a search. The search measured W and L with the page's own
float32 Euler step at each dt on its grid. It set W with d, which turns the
orbit without changing L, and chose the trail N. It kept the combination
that leaves the least drift in both color and petal position. Every link
stays visibly still for at least 13 minutes, most of them for over 20.

The period stops at 20, and the trail has to cover the whole closed orbit
(N ≥ T). That caps k at about 20, which is why the 8-petal mandala has no
three-ring version.

## Lines and colors still, or colors flowing

| Petals | Rings out | Rainbows around | Lines and colors still | Colors flowing |
|---|---|---|---|---|
| 7 | 1 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=9652&rf=9.00205&gs=3&gc=4&ar=0&p.dt=0.0059994&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.06999&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=12942&rf=11.99875&gs=3&gc=4&ar=0&p.dt=0.0059599&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07189&p.e=0.236&p.f=0&rot=0,0,0) |
| 7 | 2 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=7609&rf=14.00115&gs=3&gc=4&ar=0&p.dt=0.0059101&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07429&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=8044&rf=14.9978&gs=3&gc=4&ar=0&p.dt=0.0059965&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07013&p.e=0.236&p.f=0&rot=0,0,0) |
| 7 | 0 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=7630&rf=1.00285&gs=3&gc=4&ar=0&p.dt=0.0059101&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07429&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=7605&rf=0.99955&gs=3&gc=4&ar=0&p.dt=0.0059101&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07429&p.e=0.236&p.f=0&rot=0,0,0) |
| 7 | 0 | 2 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=11336&rf=3.00275&gs=3&gc=4&ar=0&p.dt=0.0059599&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07189&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=7606&rf=1.9994&gs=3&gc=4&ar=0&p.dt=0.0059101&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07429&p.e=0.236&p.f=0&rot=0,0,0) |
| 7 | 1 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=15066&rf=16.00195&gs=3&gc=4&ar=0&p.dt=0.0059757&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07113&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=15051&rf=15.9986&gs=3&gc=4&ar=0&p.dt=0.0059809&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07088&p.e=0.236&p.f=0&rot=0,0,0) |
| 7 | 1 | -1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=10070&rf=8.0022&gs=3&gc=4&ar=0&p.dt=0.0059599&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.07189&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=13758&rf=10.99885&gs=3&gc=4&ar=0&p.dt=0.0059994&p.a=0.83976&p.b=0.654&p.c=0.623&p.d=3.06999&p.e=0.236&p.f=0&rot=0,0,0) |
| 5 | 1 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=18765&rf=17.0021&gs=3&gc=4&ar=0&p.dt=0.0059267&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07373&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=6570&rf=5.99875&gs=3&gc=4&ar=0&p.dt=0.005978&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07118&p.e=0.236&p.f=0&rot=0,0,0) |
| 5 | 2 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=7572&rf=14.00115&gs=3&gc=4&ar=0&p.dt=0.0060612&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.06706&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=8276&rf=14.99785&gs=3&gc=4&ar=0&p.dt=0.0059275&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07369&p.e=0.236&p.f=0&rot=0,0,0) |
| 5 | 3 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=6255&rf=17.0003&gs=3&gc=4&ar=0&p.dt=0.0059267&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07373&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=5885&rf=15.99695&gs=3&gc=4&ar=0&p.dt=0.0059277&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07368&p.e=0.236&p.f=0&rot=0,0,0) |
| 5 | 0 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=27342&rf=5.0028&gs=3&gc=4&ar=0&p.dt=0.0059903&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07057&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=27589&rf=4.9995&gs=3&gc=4&ar=0&p.dt=0.0059267&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.07373&p.e=0.236&p.f=0&rot=0,0,0) |
| 5 | 1 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=11822&rf=13.0019&gs=3&gc=4&ar=0&p.dt=0.0060026&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.06996&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=6341&rf=6.99855&gs=3&gc=4&ar=0&p.dt=0.0060266&p.a=0.84906&p.b=0.654&p.c=0.623&p.d=3.06877&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | 1 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=4493&rf=4.0021&gs=3&gc=4&ar=0&p.dt=0.006087&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.06529&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=5738&rf=4.9988&gs=3&gc=4&ar=0&p.dt=0.0059376&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.07325&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | 2 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=6836&rf=12.00125&gs=3&gc=4&ar=0&p.dt=0.0059878&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.07057&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=6798&rf=11.9979&gs=3&gc=4&ar=0&p.dt=0.0060233&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.06868&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | 3 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=5669&rf=15.00035&gs=3&gc=4&ar=0&p.dt=0.0060201&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.06885&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=6755&rf=17.997&gs=3&gc=4&ar=0&p.dt=0.0060662&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.0664&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | 0 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=23653&rf=7.0027&gs=3&gc=4&ar=0&p.dt=0.0060662&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.0664&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=6749&rf=1.99935&gs=3&gc=4&ar=0&p.dt=0.0060711&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.06614&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | 1 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=6032&rf=7.00185&gs=3&gc=4&ar=0&p.dt=0.0059331&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.07349&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=12814&rf=14.9985&gs=3&gc=4&ar=0&p.dt=0.0059878&p.a=0.86847&p.b=0.654&p.c=0.623&p.d=3.07057&p.e=0.236&p.f=0&rot=0,0,0) |
| 8 | 1 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=12789&rf=12.00205&gs=3&gc=4&ar=0&p.dt=0.0060023&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.06998&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=12901&rf=11.99875&gs=3&gc=4&ar=0&p.dt=0.0059431&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.0728&p.e=0.236&p.f=0&rot=0,0,0) |
| 8 | 2 | 0 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=10075&rf=19.0011&gs=3&gc=4&ar=0&p.dt=0.0060343&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.06846&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=10655&rf=19.9978&gs=3&gc=4&ar=0&p.dt=0.0060021&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.06999&p.e=0.236&p.f=0&rot=0,0,0) |
| 8 | 0 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=8627&rf=1.0029&gs=3&gc=4&ar=0&p.dt=0.0059431&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.0728&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=8598&rf=0.99955&gs=3&gc=4&ar=0&p.dt=0.0059431&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.0728&p.e=0.236&p.f=0&rot=0,0,0) |
| 8 | 1 | 1 | [still](https://chaosrack.magnetosphere.net/#aizawa&tr=11368&rf=12.00195&gs=3&gc=4&ar=0&p.dt=0.0060023&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.06998&p.e=0.236&p.f=0&rot=0,0,0) | [flowing](https://chaosrack.magnetosphere.net/#aizawa&tr=12238&rf=12.9986&gs=3&gc=4&ar=0&p.dt=0.0060423&p.a=0.83668&p.b=0.654&p.c=0.623&p.d=3.06808&p.e=0.236&p.f=0&rot=0,0,0) |

## Lines moving, colors standing

Pick W far from any simple fraction and the orbit never closes. Each frame's
rosette is the last one turned, so the lines keep moving. The colors can
still stand: k = m rainbows per excursion, with k·N/L ≡ 0.003 (mod 1). These
use W = 3.381966…, three plus the golden ratio's fractional part squared,
the number hardest to approximate by a fraction, so no petal count ever
settles in.

| Rings out | Link |
|---|---|
| 1 | [lines moving](https://chaosrack.magnetosphere.net/#aizawa&tr=22252&rf=19.00215&gs=3&gc=4&ar=0&p.dt=0.005898&p.a=0.87481&p.b=0.654&p.c=0.623&p.d=3.07001&p.e=0.236&p.f=0&rot=0,0,0) |
| 2 | [lines moving](https://chaosrack.magnetosphere.net/#aizawa&tr=9144&rf=16.00125&gs=3&gc=4&ar=0&p.dt=0.0060652&p.a=0.87481&p.b=0.654&p.c=0.623&p.d=3.06999&p.e=0.236&p.f=0&rot=0,0,0) |
| 3 | [lines moving](https://chaosrack.magnetosphere.net/#aizawa&tr=7309&rf=19.0004&gs=3&gc=4&ar=0&p.dt=0.0059983&p.a=0.87481&p.b=0.654&p.c=0.623&p.d=3.06999&p.e=0.236&p.f=0&rot=0,0,0) |

In the 3-ring link the trail start moves a third of an excursion each frame.
Three successive frames each show a different third, with the colors
matching, so at 60 fps your eye sees them laid over each other. That is
one way frame rate and orbit can line up to put the same line in more than
one place.

## With f above 0

With f > 0 the attractor is no longer symmetric, and closed orbits lock in
over a range of settings instead of at one exact value. The first versions
of these three effects used that:

- [lines move, colors stand](https://chaosrack.magnetosphere.net/#aizawa&tr=15277&rf=14.0022&gs=3&gc=4&ar=0&p.dt=0.006&p.a=0.85&p.b=0.654&p.c=0.623&p.d=3.07&p.e=0.236&p.f=0.02&rot=0,0,0)
  (a precessing 5-petal rosette)
- [lines still, colors flow](https://chaosrack.magnetosphere.net/#aizawa&tr=19185&rf=17.99815&gs=3&gc=4&ar=0&p.dt=0.006&p.a=0.84&p.b=0.654&p.c=0.623&p.d=3.07&p.e=0.236&p.f=0.025&rot=0,0,0)
- [both still](https://chaosrack.magnetosphere.net/#aizawa&tr=20255&rf=19.002&gs=3&gc=4&ar=0&p.dt=0.006&p.a=0.84&p.b=0.654&p.c=0.623&p.d=3.07&p.e=0.236&p.f=0.025&rot=0,0,0)
  (an 8-petal mandala, locked at exactly 25/8 turns per excursion)
