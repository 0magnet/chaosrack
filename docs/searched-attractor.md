# Attractors found by search

[← Gallery](gallery.md)

A five-term chaotic flow, found by a random search of small quadratic
systems and typed into the **Custom** model:

    x′ = −y + y·z
    y′ = −x·z
    z′ = a·z² + y          a = −0.45

[Open it](https://chaosrack.magnetosphere.net/#custom&ar=0&eq=-y%20%2B%20y*z;-x*z;a*z%5E2%20%2B%20y&cp.a=-0.45&cdt=0.01)
— or on your own server, append
`#custom&ar=0&eq=-y%20%2B%20y*z;-x*z;a*z%5E2%20%2B%20y&cp.a=-0.45&cdt=0.01`.

It is not among Sprott's nineteen simplest flows (A–S) or any system the
rack already has. Whether it is new to the literature I cannot say: it
belongs to a family that has been searched (below), and nothing here checks
the papers.

## How it was found

The search drew systems of three equations with five terms in all, each
term ±1 times a monomial of degree up to two, with one term carrying a
coefficient `a` from a short list. Each was integrated with RK4 and kept
only if it passed all of these:

- it stays within a reach of 100 for the whole run;
- its largest Lyapunov exponent is between 0.01 and 5;
- that exponent agrees at two step sizes (a numerical blow-up does not);
- it contracts volume on average (a dissipative attractor, not a
  conservative sea).

The first, looser search was mostly junk: exponents near 480 that were
the integrator diverging. The survivors were then run 50 000 time units
from two starts each. Both had to stay within three times their early
reach, agree on the exponent, and have a Kaplan–Yorke dimension between 2
and 3.

Eight systems passed. Three were Sprott's C, D and E, under a relabeling of
the variables. Finding known chaotic flows again is the check that the
search finds real ones. This one matched nothing known by its structure:
which monomial is in which equation, under every relabeling and sign flip.

## What it is like

| a | behavior |
|---|---|
| −0.70 | periodic: a closed loop |
| −0.65 … −0.20 | chaotic |
| −0.45 | λ₁ ≈ 0.066, Kaplan–Yorke dimension ≈ 2.11, reach ≈ 2.4 |

The page's Lyapunov readout agrees: +0.065/t, "chaotic".

Its equilibria are unusual. Every point of the x axis, (x, 0, 0), is one:
a whole line of them, centers for x > 0 and saddles for x < 0. The only
other is (0, −a, 1). At a = −0.45 that is a saddle-focus: one direction
pulls in, and in a plane the trajectory spirals out of it, the arrangement
behind Shilnikov's route to chaos.

Systems with a line of equilibria are the family Jafari and Sprott
searched in 2013 ("Simple chaotic flows with a line equilibrium"). This
system may be one of theirs, under other names.

## Reproducing it by hand

1. MODEL: **Custom**.
2. Type the three equations: `-y + y*z`, `-x*z`, `a*z^2 + y`.
3. Set **a** to −0.45 and **dt** to 0.01.
4. Auto-rotate off. Turn **a** slowly from −0.2 toward −0.7 and watch the
   curl tighten and then close into a loop.

## A second one, from six terms

The same search with six terms passed 73 systems. None is a known system
by structure, but that proves less than the five-term run did: Sprott's
six-term flows (K–S) need coefficients such as 2.7 or 3.9 that the search
never tried, so it could not have found them again.

The search integrates with RK4. The Custom model steps forward Euler unless
its **INTG** selector, in the bank after the constants, is turned to rk4,
so each survivor was also run the page's default way, from the page's
start. Most look the same either way. Some do not: the strongest of them
all (λ₁ ≈ 1.0) blows up under Euler at every step size, down to dt 0.001,
and needs INTG at rk4.

The best of those that survive Euler:

    x′ = a·y·z + z
    y′ = x·z − 1
    z′ = x − z            a = −3

[Open it](https://chaosrack.magnetosphere.net/#custom&ar=0&eq=a*y*z%20%2B%20z;x*z%20-%201;x%20-%20z&cp.a=-3&cdt=0.01)
— or append `#custom&ar=0&eq=a*y*z%20%2B%20z;x*z%20-%201;x%20-%20z&cp.a=-3&cdt=0.01`.

λ₁ ≈ 0.29 (the page reads +0.27/t under Euler), Kaplan–Yorke dimension
≈ 2.23, reach about 7. It has two equilibria, (±1, 1/3, ±1). The orbit
spirals out from around one, then swings over to the other. One lobe is
wound tight and the other loose.
