# Synth

## Synth

<!-- key: synth-synth -->
Synth — the sound the keyboard plays. PRESET sets every knob on the bay to an instrument, as a place to start from; LVL is how loud. Where it is heard is the Mixer's: its KEYS column.

### Preset

<!-- key: syn-preset -->
Preset — sets every knob on the bay to an instrument; turn any of them after to change it from there.

- <!-- key: syn-preset=piano --> piano — a struck string: slightly sharp overtones fading at their own rates, two strings beating in unison, and a hammer
- <!-- key: syn-preset=epiano --> electric piano — a tine piano by FM: a bark at the attack fading into a nearly pure tone
- <!-- key: syn-preset=hpschd --> harpsichord — a plucked string, every harmonic, bright at the pluck and darkening
- <!-- key: syn-preset=organ --> organ — drawbars 16′ 8′ 5⅓′ 4′ and a little 2⅔′, sounding for as long as the key is held
- <!-- key: syn-preset=sine --> sine — the generators' plain sine, held
- <!-- key: syn-preset=tri --> triangle — the generators' triangle, held
- <!-- key: syn-preset=sqr --> square — the generators' square, held
- <!-- key: syn-preset=saw --> saw — the generators' saw, held
- <!-- key: syn-preset=noise --> noise — the shift-register noise, tuned to the key, held

### Level

<!-- key: keys-lvl -->
Level — how loud the keyboard plays (0–100).

## String

<!-- key: synth-string -->
String — the additive voice that is the piano: partials not quite harmonic, each fading at its own rate. STR its level; PART how many partials; TILT how fast they fall away up the series; INHM the string's stiffness, which sharpens the upper partials (×10⁻⁴); STRK where along the string it is struck, which silences the partials with a node there; UNI the detune between the two strings of each low partial, in cents; DAMP how much faster each higher partial dies.

### String level

<!-- key: syn-str -->
String level — the additive layer's share of the sound.

### Partials

<!-- key: syn-part -->
Partials — how many of the string's overtones sound.

### Tilt

<!-- key: syn-tilt -->
Tilt — how fast the partials fall away up the series: low is bright, high is mellow.

### Inharmonicity

<!-- key: syn-inhm -->
Inharmonicity — the string's stiffness, ×10⁻⁴, which sharpens each partial by B·n²: none is an organ pipe's series, a piano's is about 2.5.

### Strike point

<!-- key: syn-strk -->
Strike point — where along the string it is struck, as a fraction: the partials with a node there are thinned. A piano's hammer is at about an eighth; 0 is off.

### Unison

<!-- key: syn-uni -->
Unison — the detune between the two strings of each of the three lowest partials, in cents: their slow beating is a piano's shimmer.

### Damping

<!-- key: syn-damp -->
Damping — how much faster each higher partial dies than the one below it, so the tone darkens as it rings.

## FM

<!-- key: synth-fm -->
FM — a sine modulated by another: the electric piano. FM its level; RATO the modulator's frequency over the note's; INDX how deep the modulation starts; IDEC how fast it fades, which is how fast the bark becomes a pure tone.

### FM level

<!-- key: syn-fm -->
FM level — the FM layer's share of the sound.

### Ratio

<!-- key: syn-rato -->
Ratio — the modulator's frequency over the note's: whole numbers are harmonic, others bell-like.

### Index

<!-- key: syn-indx -->
Index — how deep the modulation starts: higher is brighter and harsher.

### Index decay

<!-- key: syn-idec -->
Index decay — how fast the modulation fades, in seconds.

## Wave · Noise

<!-- key: synth-wave -->
Wave · Noise — WAVE is one oscillator, a generator waveform, the organ's drawbars or tuned noise, at a level; NOISE is a burst of filtered noise at the start of each note — a hammer, a quill, a tine's click — at TONE times the note, dying over NDEC.

### Wave shape

<!-- key: syn-shape -->
Wave shape — the oscillator: sine, triangle, square, saw, the organ's drawbars (16′ 8′ 5⅓′ 4′ 2⅔′) or tuned shift-register noise.

- <!-- key: syn-shape=sine --> sine — one pure frequency
- <!-- key: syn-shape=triangle --> triangle — soft and hollow
- <!-- key: syn-shape=square --> square — hollow and reedy
- <!-- key: syn-shape=sawtooth --> saw — every harmonic, the brightest
- <!-- key: syn-shape=organ --> organ — drawbars 16′ 8′ 5⅓′ 4′ and a little 2⅔′
- <!-- key: syn-shape=noise --> noise — a 15-bit shift register, tuned to the key

### Wave level

<!-- key: syn-wave -->
Wave level — the oscillator layer's share of the sound.

### Noise level

<!-- key: syn-nse -->
Noise level — the burst at the start of each note.

### Noise tone

<!-- key: syn-tone -->
Noise tone — where the burst is centered, in multiples of the note: low is a felt thump, high a click.

### Noise decay

<!-- key: syn-ndec -->
Noise decay — how fast the burst dies, in seconds.

## Filter

<!-- key: synth-filter -->
Filter — a lowpass on each note that opens at BRGT times the note and closes towards DARK times it over FDEC: a plucked string brightest at the pluck. RESO is its resonance.

### Bright

<!-- key: syn-brgt -->
Bright — where the filter opens, in multiples of the note.

### Dark

<!-- key: syn-dark -->
Dark — where the filter settles, in multiples of the note: the same as bright is no sweep at all.

### Filter decay

<!-- key: syn-fdec -->
Filter decay — how long the filter takes to close, in seconds.

### Resonance

<!-- key: syn-reso -->
Resonance — the filter's peak at its cutoff.

## Amp

<!-- key: synth-amp -->
Amp — the envelope every layer follows: ATK up, then towards SUS with time constant DEC (at C3), which KTRK shortens up the keyboard and lengthens down it, as a piano's strings ring longer in the bass; REL is how long letting go takes.

### Attack

<!-- key: syn-atk -->
Attack — how long the note takes to rise, in seconds.

### Decay

<!-- key: syn-dec -->
Decay — the time constant of the fall towards sustain, in seconds, at C3.

### Key tracking

<!-- key: syn-ktrk -->
Key tracking — how much shorter the decay is up the keyboard: 1 is an octave shorter every eighteen semitones, a piano's; 0 is the same everywhere.

### Sustain

<!-- key: syn-sus -->
Sustain — the level held while the key is down: 0 dies away like a string, 1 holds like an organ.

### Release

<!-- key: syn-rel -->
Release — how long the note takes to stop once let go, in seconds.


### The keyboard's keys

<!-- key: keys-key -->
Keys — play {name} (hold and slide for glissando)
