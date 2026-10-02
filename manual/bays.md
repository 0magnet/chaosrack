# Model bays

The bays that hold the models: each has a monitor, the switches under it, and
the knobs that choose which of its models is playing.

## Monitor

<!-- key: bay-monitor -->
Monitor — this bay's screen. It shows the model while this bay is the one driving the rack, and stands by when another is: only one model is drawn, so only one screen can carry a picture. The Screen switch under it cuts the copy out of the drawing buffer that the picture costs.

### Screen

<!-- key: bay-screen -->
Screen — power to this monitor. Its picture is a copy out of the drawing buffer every frame; off, the glass goes dark and costs nothing.

## Choosing the model

<!-- key: bay-model-cell -->
Model — which of the generators in THIS bay is playing. Off means another bay is driving the rack; every bay keeps what it was set to either way.

<!-- key: bay-model -->
Model — pick one of this bay's generators, or turn the knob above it

<!-- key: bay-category -->
Category — which kind of model the inner ring chooses among

<!-- key: bay-model-all -->
Model — every model in this bay, by category. The number beside each is how many of the bank's controls it has.

## Families

A family is a set of systems that share one position on the MODEL knob, with
a dial of their own to choose among them. `{family}` below is the family's
name.

<!-- key: fam-module -->
{family} — J. C. Sprott's nineteen simple chaotic flows (1994). The MODEL knob has one position for all of them; this dial picks which one it plays.

<!-- key: fam-cell -->
{family} — the system the MODEL knob's {family} position plays

<!-- key: fam-select -->
{family} — which system the MODEL knob's {family} position plays

## Generator modules

<!-- key: gen-unit -->
One unit, {count} generator(s): {names}.

A module is a hardware unit — the same inputs and outputs as any other, dedicated to what is printed on it. A unit per generator is honest about the models and wrong about the hardware, because most of them are a knob wide. A panel is three control rows deep, so generators share one when between them they fill it; one that fills its own columns keeps them.

<!-- key: bay-rings -->
Outer ring: which category, or OFF at the bottom. Inner ring: which model in it; turned past the last it goes on into the next category.

## Categories

<!-- key: cat.Visual -->
Visual — every model, in one bay: attractors, maps, solids, geometry and sequences drawn in three dimensions, and the scope pictures, embeddings and audio displays drawn from a signal. The outer ring of the MODEL knob picks which kind, the inner ring which model, and the bank beside it is reprogrammed to that model's constants

<!-- key: cat.Attractors -->
Attractors — chaotic flows integrated in three dimensions: Lorenz, Rössler, Chua and the rest, then the nineteen simple systems of J. C. Sprott, 1994, a morph between them, and the system you type in yourself

<!-- key: cat.Maps -->
Maps — discrete iterated systems (Hénon, Ikeda, the standard map…) rather than flows, the bifurcation plot of one, and the Poincaré section that turns a flow into one

<!-- key: cat.Scope -->
Scope — what an oscilloscope draws: Lissajous figures, the Graphic Artist, and the audio displays

<!-- key: cat.Embeddings -->
Embeddings — a state space rebuilt from one signal by delaying it against itself: Takens against its own past, Stereo against the other channel, Polar with the delay wrapped onto an angle. All three are steered by the delay τ

<!-- key: cat.Geometry -->
Geometry — built, not integrated: the Platonic solids under a Conway operator (17 of them, most of the Archimedeans and their duals), and the sphere, torus, globe and magnetosphere

<!-- key: cat.Sequences -->
Sequences — the Turtle Path: an integer sequence read as turn-and-step

<!-- key: cat.Solids -->
Solids — the STL viewer: a file from disk, a terminal, or the whole desk as an object

<!-- key: cat.Audio -->
Audio — displays of the live signal: spectrogram, goniometer, the FVF wobbulator and the delay embeddings

### A row's head

<!-- key: cat-head -->
{what}

The head of the row: the monitor, the knob that chooses which of this category's models is playing, its integration step, and anything that belongs to the category rather than to one model. Each model with constants of its own has a card of its own further along the row.
