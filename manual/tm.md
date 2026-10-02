## Matrix

<!-- key: tm -->
Matrix — a pentatonic tonematrix step sequencer with a rhythm section under it: paint pads on the grid (click or drag; rows are pitches, columns are sixteenths) and the playhead loops them as pings at the tempo, over as many columns as the steps knob says. The four lanes at the foot are the drums (bass drum, snare, hi-hat, cymbal), on the same beat; they count in sixteenths or, for a shuffle or swing, in triplets, and each beat of them sits under the same beat of the tune. A preset tab loads a pattern into them to play or to edit. The pentatonic rows mean any pattern is consonant.

### Tempo (beats per minute)

<!-- key: tm-tempo-led -->
Tempo (beats per minute) — the playhead steps four columns per beat

<!-- key: rst-tm-tempo -->
Reset the tempo

<!-- key: rst-tm-steps -->
Reset the step count and root

### Loop length

<!-- key: tm-steps -->
Loop length — how many columns of the grid the playhead sweeps (outer ring: 12 / 16 / 24 / 32 / 48 / 64, three to sixteen beats). The grid stays; the columns past the loop dim and keep their pads. A rhythm preset moves it to a whole number of its bars

- <!-- key: tm-steps=12 --> 12 steps — three beats: one bar of a waltz
- <!-- key: tm-steps=16 --> 16 steps — four beats: one bar of four
- <!-- key: tm-steps=24 --> 24 steps — six beats: two bars of a waltz
- <!-- key: tm-steps=32 --> 32 steps — eight beats: two bars of four
- <!-- key: tm-steps=48 --> 48 steps — twelve beats: three bars of four, or four of a waltz
- <!-- key: tm-steps=64 --> 64 steps — sixteen beats: the whole grid

### Root octave

<!-- key: tm-root -->
Root octave — the C the bottom row starts on; rows climb the major pentatonic from there (inner knob)

- <!-- key: tm-root=1 --> C1 — the bottom row starts two octaves below middle C
- <!-- key: tm-root=2 --> C2 — the bottom row starts one octave below middle C
- <!-- key: tm-root=3 --> C3 — the bottom row starts at middle C
- <!-- key: tm-root=4 --> C4 — the bottom row starts one octave above middle C

<!-- key: tm-lvl-led -->
Matrix output level (0–100)

<!-- key: rst-tm-lvl -->
Reset the level

<!-- key: rst-tm-wave -->
Reset the waveform

### Matrix voice

<!-- key: tm-wave -->
Matrix voice — the pings' shape: sine, tri, sqr, saw or noise. Where the Matrix and its drums are heard is the Mixer's: its MATRIX and DRUMS columns.

- <!-- key: tm-wave=0 --> sine — one pure frequency and no harmonics at all
- <!-- key: tm-wave=1 --> triangle — odd harmonics falling away steeply: soft and hollow
- <!-- key: tm-wave=2 --> square — odd harmonics falling away slowly: hollow and reedy
- <!-- key: tm-wave=3 --> sawtooth — every harmonic, the brightest of the four
- <!-- key: tm-wave=4 --> noise — a 15-bit shift register; the pad's pitch sets its playback rate

### Run

<!-- key: tm-run-cell -->
Run — start/stop the playhead loop, tune and drums together (painting works either way). Pressing a preset tab starts it too.

### Beat lamps

<!-- key: rhythm-beats -->
Beat lamps — one per beat of the bar, lit on the beat you are hearing; the downbeat is the leftmost

### Clear

<!-- key: tm-clear -->
Clear — wipe every pad off the grid, drum lanes included

<!-- key: rhythm-lvl-led -->
Drum level (0–100), beside the pads' own; the drums follow the out ring

<!-- key: rst-rhythm-lvl -->
Reset the drum level

### Tonematrix grid

<!-- key: tm-grid -->
Tonematrix grid — click or drag to paint pads; rows are pentatonic pitches (top = high), columns are sixteenths, and the dimmed ones are past the loop

### Drum lanes

<!-- key: tm-drums -->
Drum lanes — bass drum, snare, hi-hat, cymbal; click or drag to paint. Four to the beat, or three when a preset is in triplets; each beat lines up under the same beat of the grid

### Rhythm presets

<!-- key: rhythm-tabs -->
Rhythm presets — a pattern for the drum lanes, one at a time. Pressing one pops the last one out, loads its bar into the lanes (repeated across the grid), counts the lanes in triplets or sixteenths as it needs, and moves the loop to a whole number of its bars; the lanes are yours to edit from there.

### The pads

`{step}` and `{note}` are a pad's column and pitch; on a drum lane, `{lane}`
is the drum, `{beat}` the beat and `{unit}` `{n}` which division of it.

<!-- key: tm-pad -->
Tonematrix pad — step {step}, {note} (click to toggle, drag to paint)

<!-- key: tm-drum-pad -->
Drum lane — {lane}, beat {beat}, {unit} {n} (click to toggle, drag to paint)
