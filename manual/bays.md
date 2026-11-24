# Model bays

Bay 1 holds the models: a monitor, the switches under it, and the bank of
knobs beside it, headed by the MODEL knob that chooses which model is playing.

## Monitor

<!-- key: bay-monitor -->
Monitor — the model on a screen of its own, a copy of the picture.
It stands by while the rack is off.
The scrn switch under it turns the copy off, and with it what the copy costs.

### scrn

<!-- key: bay-screen -->
scrn — power to this monitor.
Its picture is a copy out of the drawing buffer every frame; off, the glass goes dark and costs nothing.

## Choosing the model

<!-- key: bay-model-cell -->
Model — the model playing.
The outer ring picks the category and the inner ring the model in it;
OFF at the bottom of the outer ring powers the rack down, the same as the Console's Power switch, and every setting stays as it was.

<!-- key: bay-model -->
Model — pick a model from the list, or turn the knob above it

<!-- key: bay-category -->
Category — which kind of model the inner ring chooses among

<!-- key: bay-model-all -->
Model — every model, by category. The number beside each is how many of the bank's controls it has.

<!-- key: bay-rings -->
Outer ring: which category, or OFF at the bottom, which powers the rack down.
Inner ring: which model in it; turned past the last it goes on into the next category.

## Families

A family is a set of systems that share one position on the MODEL knob, with
a selector of their own in the bank, sys, to choose among them. `{family}` below is the family's
name.

<!-- key: fam-cell -->
{family} — the system the MODEL knob's {family} position plays

<!-- key: fam-select -->
{family} — which system the MODEL knob's {family} position plays

## Categories

<!-- key: cat.Visual -->
Visual — every model, in one bay: attractors, maps, solids, geometry and sequences drawn in three dimensions, and the scope pictures, embeddings and audio displays drawn from a signal.
The outer ring of the MODEL knob picks which kind, the inner ring which model, and the bank beside it is reprogrammed to that model's constants

<!-- key: cat.Attractors -->
Attractors — chaotic flows integrated in three dimensions: Lorenz, Rössler, Chua and the rest, then the nineteen simple systems of J. C. Sprott, 1994, a morph between them, and the system you type in yourself

<!-- key: cat.Maps -->
Maps — discrete iterated systems (Hénon, Ikeda, the standard map…) rather than flows, the bifurcation plot of one, and the Poincaré section that turns a flow into one

<!-- key: cat.Scope -->
Scope — what an oscilloscope draws: Lissajous figures, the Graphic Artist, Pong, Fourier Text, the Scope Clock, the Bouncing Ball and the X/Y scope

<!-- key: cat.Embeddings -->
Embeddings — a state space rebuilt from one signal by delaying it against itself: Takens against its own past, Stereo against the other channel, Polar with the delay wrapped onto an angle.
All three are steered by the delay τ

<!-- key: cat.Geometry -->
Geometry — built, not integrated: the Platonic solids under a Conway operator (17 of them, most of the Archimedeans and their duals), and the globe (the sphere too, at lat 0), the torus, the nested cube and the magnetosphere

<!-- key: cat.Sequences -->
Sequences — the Turtle Path: an integer sequence read as turn-and-step

<!-- key: cat.Solids -->
Solids — what is drawn on a surface: the STL viewer's file from disk, a terminal, a terminal animation, the host's shell, the whole desk, and the Lattice

<!-- key: cat.Audio -->
Audio — displays of the live signal: the spectrogram, the FVF wobbulator, the recurrence plot, the RTA, the transfer function and the waterfall

### The bay's head

<!-- key: cat-head -->
{what}

The head of the bay: the monitor, the switches under it, and the readouts of the model playing.
The MODEL knob and the model's step head the bank beside it, which the MODEL knob reprograms.

## Measure

<!-- key: measure-mode-stack-cell -->
Measure — pick points on the figure and measure the distances and angles between them, named when they are numbers worth naming: √3, φ, 1/φ², 108° = 3π/5.
It stands at 1.12.3 under the figures of lines (the geometry models: Polyhedron, the Chestahedron, Sacred Geometry and the rest), which have no clock, in the place Speed has under the models that do.
A press near a point that matters takes it: a center a circle was drawn about, a place where circles and lines meet (solved exactly, not read off the drawing), a corner, or a place where two lines cross in space.
A ring follows the cursor to show which point a press would take; a press on empty space still turns the view, so the figure can be turned between picks.
The first distance is the unit, and every distance after it reads as a ratio to it: measure the vesica's line of centers, then its chord, and the chord says √3. It measures in a single view.

- <!-- key: measure-mode=0 --> off — presses turn the view, as always
- <!-- key: measure-mode=1 --> dist — two points, and the distance between them
- <!-- key: measure-mode=2 --> angle — three points, and the angle at the second

### Measure buttons

- <!-- key: trio.measure-mode=0 --> U: unit — the last distance measured is the unit from now on (lit while there is one)
- <!-- key: trio.measure-mode=1 --> 1: forget the unit — the next distance measured becomes it
- <!-- key: trio.measure-mode=2 --> C: clear — every measurement, and the unit
