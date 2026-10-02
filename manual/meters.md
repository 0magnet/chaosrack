# Meters

## Counter

<!-- key: counter -->
Counter — a frequency counter for the rack, in the spirit of the glensstuff.com NAND-gate counter: it counts trigger crossings of the live audio source over the gate window and shows cycles per second, the way the discrete-logic original did. Feed it the mic, the ws stream, or the signal generator.

### Measured frequency in Hz

<!-- key: counter-led -->
Measured frequency in Hz — cycles counted over the last closed gate window

### Gate lamp

<!-- key: counter-gate -->
Gate lamp — flashes each time the gate window closes and the readout updates

<!-- key: rst-counter-gate -->
Reset the gate time

### Gate time in seconds

<!-- key: counter-gatesel -->
Gate time in seconds — how long the counter counts before updating (longer gate = finer resolution, slower updates)

- <!-- key: counter-gatesel=0.1 --> 0\.1 s gate — the fastest update and the coarsest resolution
- <!-- key: counter-gatesel=0.5 --> 0\.5 s gate — twice a second
- <!-- key: counter-gatesel=1 --> 1 s gate — the standard frequency-counter gate: the reading is hertz directly
- <!-- key: counter-gatesel=2 --> 2 s gate — the slowest update and the finest resolution

### Trigger level (% of full scale)

<!-- key: counter-trig-led -->
Trigger level (% of full scale) — the hysteresis threshold a cycle must swing across to count; raise it to reject noise

<!-- key: rst-counter-trig -->
Reset the trigger level

## Distortion

<!-- key: thd -->
Distortion — THD, THD+N, SINAD and ENOB of the live audio, measured against whichever tone is in it. Feed it a generator's sine at 1 kHz (type 1000 into its Hz readout) through whatever you want measured and read how much of what comes back is not that tone. THD is the harmonics alone, which is the number a specification quotes; THD+N is everything that is not the fundamental, which is the honest one and always the larger; SINAD is the same ratio in decibels, the way a converter is specified; ENOB runs the ideal-converter relation backwards and says how many bits would sound this clean. The gap between THD and THD+N is how much of the rubbish is hiss rather than distortion. The instrument's own floor is about 0.0015% THD+N (96.5 dB SINAD, 17.3 effective bits), measured on a synthesized pure tone — that is the Blackman-Harris window's leakage past the notch, and nothing quieter than it can be read.

### Total harmonic distortion

<!-- key: thd-led -->
Total harmonic distortion — the harmonics alone, as a percentage of the fundamental. Noise is not counted, which is what makes this the flattering number.

<!-- key: thd-fund-led -->
The fundamental the measurement is against, in Hz — the largest peak above DC, found between bins

### THD+N

<!-- key: thdn-led -->
THD+N — everything that is not the fundamental, as a percentage of it: harmonics, noise, hum, the lot. Always at least THD.

<!-- key: thd-level-led -->
Level of the fundamental in dBFS. A distortion figure is quoted at a stated level, and near full scale is where it is usually worst.

### SINAD in dB

<!-- key: thd-sinad-led -->
SINAD in dB — signal-plus-noise-and-distortion over noise-and-distortion, the way a converter is specified.

<!-- key: thd-enob-led -->
Effective number of bits: (SINAD − 1.76) / 6.02. What an ideal converter would have to be to sound this clean.

<!-- key: rst-thd-chan -->
Reset the measured channel

### Source

<!-- key: thd-chan -->
Source — which signal distortion is measured on: the mix, one channel, or the mid or side of the pair

<!-- key: thd-harm-led -->
How many harmonics are summed into THD (2–20). Harmonics past Nyquist are never counted: they are not there.

<!-- key: rst-thd-harm -->
Reset the harmonic count

## Loudness

<!-- key: lufs -->
Loudness — LUFS to ITU-R BS.1770 and EBU R 128, the scale everything is delivered against. M is the momentary loudness over 400 ms, S the short-term over 3 s, I the INTEGRATED reading over everything since the last reset — gated, so silence and quiet passages do not drag a programme's number down, which is what makes it the number a delivery spec means. LRA is the loudness range, how far the loud parts sit above the quiet ones: one number for how dynamic the material is, and the thing a loudness target alone says nothing about. TP is the TRUE peak in dBTP, the peak of the reconstructed signal rather than of the samples — a full-scale tone at a quarter of the sample rate can have every sample at −3 dBFS and still clip the converter, which is why a delivery ceiling is a true-peak ceiling. K-weighting is derived from the analog prototype at whatever rate the audio is arriving at, not the 48 kHz coefficients the standard prints, so the weighting is the same filter at every rate.

### Momentary loudness, LUFS over 400 ms

<!-- key: lufs-m-led -->
Momentary loudness, LUFS over 400 ms — ungated. What it sounds like right now.

### Short-term loudness, LUFS over 3 s

<!-- key: lufs-s-led -->
Short-term loudness, LUFS over 3 s — the window a person actually judges level over.

<!-- key: lufs-i-led -->
Integrated loudness, LUFS over everything since the last reset, gated. The number a delivery spec means.

<!-- key: lufs-lra-led -->
Loudness range in LU: the 95th percentile of the short-term distribution less the 10th. Percentiles rather than extremes, so one cymbal does not set the range of a whole program.

### True peak in dBTP

<!-- key: lufs-tp-led -->
True peak in dBTP — the peak of the reconstructed signal, found by oversampling four times. Higher than the sample peak whenever the waveform crests between samples.

<!-- key: lufs-reset -->
Start a new integrated measurement. The integrated reading and the loudness range are over everything since this was last pressed; M and S are windows and are not affected.

### Delivery target in LUFS

<!-- key: lufs-target-led -->
Delivery target in LUFS — the readout beside it shows how far the integrated reading is from it. −23 is EBU R 128, −14 is where most streaming sits, −16 is the podcast convention.

<!-- key: lufs-delta-led -->
How far the integrated reading is from the target, in LU. Positive is too loud.

<!-- key: rst-lufs-target -->
Reset the loudness target

### Meters

<!-- key: meters-cell -->
Meters — the top-left audio feature strip (amp / bass / mid / treble / cntr / beat). It is a meter, so its switch is in the metering bay rather than on the Console.

<!-- key: show-meters -->
Show / hide the top-left audio feature meters (amp / bass / mid / treble / cntr / beat) while Mod is on

## Wow & Flutter

<!-- key: wf -->
Wow \& Flutter — speed stability, measured off a 3150 Hz test tone. Every other analyzer here asks about amplitude; this asks whether the TIME AXIS is steady, which is what a turntable, a tape deck or a cassette is judged by. Play a generator's sine at 3150 Hz through the deck, capture the result, and read it here: SPEED is the mean frequency error as a percentage (a constant error is a pitch shift, a different fault from a wobble), WOW the slow 0.5–6 Hz modulation that once-per-revolution faults produce, FLUTTER the fast 6–100 Hz kind from capstans and idlers, and W\&F the DIN-weighted quasi-peak that a specification quotes — the deviation through a filter peaked at 4 Hz, where the ear is most sensitive to pitch movement. 3150 Hz is the figure every test record carries (DIN 45507, IEC 60386). With no deck in the loop at all it reads 0.000% on the generator itself, which is the check that the instrument is not inventing the number.

<!-- key: wf-speed-led -->
Mean frequency error as a percentage of nominal. Positive is running fast. A constant error is a pitch shift, which is a different fault from a wobble.

<!-- key: wf-carrier-led -->
The carrier as measured, in Hz. Found from the signal rather than assumed, because a deck running fast puts the tone somewhere else.

<!-- key: wf-wow-led -->
Slow modulation, 0.5–6 Hz, RMS as a percentage. An off-center spindle hole is wow at exactly the platter's rate.

<!-- key: wf-flutter-led -->
Fast modulation, 6–100 Hz, RMS as a percentage — capstan and idler faults, which the ear hears as roughness rather than as pitch movement.

<!-- key: wf-weighted-led -->
The DIN-weighted quasi-peak figure, which is the one a specification quotes: the deviation through a filter peaked at 4 Hz, read on meter ballistics that rise quickly and fall slowly. A two-pole approximation of the published curve rather than the tabulated response.

### Nominal carrier frequency in Hz

<!-- key: wf-nom-led -->
Nominal carrier frequency in Hz — what the tone is supposed to be. 3150 is what the test records carry. Set it to 0 to measure wow and flutter without a speed figure, which is the honest reading when the tone's true frequency is not known.

<!-- key: rst-wf-nom -->
Reset the nominal frequency

## Timing

<!-- key: timing -->
Timing — the rack metering itself. FRAME is the average interval in milliseconds and FPS its reciprocal, taken over the mean rather than the median so one hesitation a second cannot hide behind fifty-nine good frames. MIN and MAX are the best and worst frame in the window. LATE is the share of frames that missed the display's own interval, which is measured rather than assumed: nothing renders faster than vsync, so the shortest frame in a window IS the interval, and the reading means the same on a 60 Hz panel and a 144 Hz one. The four figures below it are where the frame went — MODEL the passes that draw the instrument, METERS everything the rack does to its own audio, SCOPE the tube, and REST the browser's own style, layout, raster and compositing, which is usually the largest share of the four. A rack that draws in 4 ms and spends 9 ms measuring its own audio is not slow, it is mis-apportioned, and no single frame rate says so.

<!-- key: tm-fps-led -->
Frames a second sustained over the last window — the reciprocal of the MEAN frame, so a hitch counts against it

<!-- key: tm-frame-led -->
Average frame interval in milliseconds. 16.7 is a 60 Hz frame; 33.3 means every other one is being missed.

### Shortest frame in the window, in ms

<!-- key: tm-min-led -->
Shortest frame in the window, in ms — which is the display's own interval, since nothing renders faster than vsync

### Longest frame in the window, in ms

<!-- key: tm-max-led -->
Longest frame in the window, in ms — the worst single hesitation

<!-- key: tm-late-led -->
Percentage of frames that missed the display's interval by half again or more. This is the number that corresponds to visible hesitation; the frame rate alone does not.

<!-- key: tm-rest-led -->
Milliseconds a frame spent outside the rack's own code: style, layout, raster, compositing and the wasm boundary. Usually the largest of the four, and the one no knob on this panel moves.

<!-- key: tm-model-led -->
Milliseconds a frame spent ISSUING the model's draw calls. A WebGL call queues a command and returns without waiting for pixels, so this reads far below what the model really costs — the GPU and compositor share of it is in REST. Useful for what it does catch: geometry rebuilt every frame instead of cached.

<!-- key: tm-meters-led -->
Milliseconds a frame the rack spent on its own audio: the tap, the loudness, distortion, wow-and-flutter and counter analyzers, and the sequencer clocks. Unlike MODEL and SCOPE this is arithmetic on this thread and the number IS the cost — it is the one of the four to trust absolutely, and the one the analyzers' own controls move.

<!-- key: tm-scope-led -->
Milliseconds a frame spent building the scope's sweep and handing it to the canvas. Like MODEL this is the issuing cost, not the rasterizing one — a canvas stroke records a path and returns — so the tube's raster lands in REST.

### Loudness RATE

<!-- key: lufs-rate -->
How often the loudness readouts latch. This is a DISPLAY rate only — the meter integrates every sample that arrives whatever this says, because an integrated loudness with a block missing is a block missing from the answer. Slower is steadier to read and costs the panel less.

- <!-- key: lufs-rate=100 --> Ten readings a second — as fast as a readout can be followed
- <!-- key: lufs-rate=200 --> Five a second: the default, and about the rate a needle settles at
- <!-- key: lufs-rate=500 --> Twice a second — steadier to read, and a fifth of the writes
- <!-- key: lufs-rate=1000 --> Once a second, for a number being watched rather than chased

### Distortion RATE

<!-- key: thd-rate -->
How often the distortion measurement is made and shown. The analysis window is 341 ms, so measuring faster than about three times a second measures the same audio twice; slower is the same reading for less work.

- <!-- key: thd-rate=200 --> Five a second — the window is 341 ms, so this is as often as there is new audio to measure
- <!-- key: thd-rate=400 --> The default: a fresh window every time, and no audio measured twice
- <!-- key: thd-rate=1000 --> Once a second — the same reading, a fifth of the work
- <!-- key: thd-rate=2000 --> Every two seconds, for a distortion figure being logged rather than tuned

### Wow & Flutter RATE

<!-- key: wf-rate -->
How often the wow-and-flutter measurement is remade. This does not make the analysis cheaper — it makes it rarer. The same lump of work lands on one frame in sixty instead of one in thirty, so this is the control for how OFTEN the rack hesitates, not for how much.

- <!-- key: wf-rate=250 --> Four a second — the most responsive, and the most expensive: this analysis walks the whole window each time
- <!-- key: wf-rate=500 --> The default
- <!-- key: wf-rate=1000 --> Once a second — halves the largest single lump of work in the rack
- <!-- key: wf-rate=2000 --> Every two seconds. A reading that settles over ten seconds does not need remaking faster than this.

### Wow & Flutter WINDOW

<!-- key: wf-win -->
How much audio each wow-and-flutter reading is made over. This is the measurement: ten seconds holds five cycles of the slowest wow, and two seconds cannot see wow at all, only flutter. It is also the cost — the analysis walks the whole window — so unlike RATE, this is the control that makes the work itself smaller.

- <!-- key: wf-win=2 --> Two seconds — flutter only. Too short to see wow at all, and the cheapest by five times.
- <!-- key: wf-win=5 --> Five seconds — two cycles of the slowest wow, and half the work of ten
- <!-- key: wf-win=10 --> Ten seconds: the default, five cycles of the slowest wow
- <!-- key: wf-win=20 --> Twenty seconds — the steadiest reading and twice the work
