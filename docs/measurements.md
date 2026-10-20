# Measurement instruments

The analyzers: what each reads, how it is computed, and how it was checked.

[← README](../README.md)

## Test signals

The reference signals a test record carries, synthesized rather than played
back. They are positions of each generator's waveform knob (Gen 1–4), after
the periodic waves; FREQ does not change them. Route the generator with its
OUT knob or the Mixer.

| | |
|---|---|
| **white noise** | the same noise in both channels: correlation **+1**, the diagonal |
| **pink noise** | equal power per octave, which is the only stimulus a fractional-octave analyzer reads flat. Room measurement is done in this |
| **log sweep** | 20 Hz–20 kHz in 4 s, repeating. Equal time per octave, so the bottom of the band gets as many cycles as the top — the stimulus an impulse response is deconvolved from |
| **1 kHz reference** | the level and distortion reference. At lvl 100 it is 0 dBFS |
| **3150 Hz** | the wow-and-flutter tone, because that is what the test records carry (DIN 45507 / IEC 60386) |
| **left / right only** | which speaker is which |
| **polarity pulse** | a short positive spike with a long shallow recovery, equal areas so it carries no DC. A tone cannot show polarity — it is the same shape upside down |
| **uncorrelated noise** | two independent streams: correlation **0**, a round cloud |
| **out-of-polarity noise** | one stream, one channel inverted: correlation **−1**, and *nothing left* when summed to mono. The mono-compatibility failure, on purpose |

Measured through chaosrack's own displays: the three noises read **r+1.00**,
**r−0.02** and **r−1.00** on the goniometer's correlation meter, and the two
reference tones read **1000.0 Hz** and **3150.4 Hz** on the frequency counter.
The left- and right-only signals read `r --`, which is the meter correctly
saying one channel is dead and there is nothing to correlate.


## Distortion — THD, THD+N, SINAD, ENOB

The measurement a piece of audio equipment is specified by: feed a clean tone
in, and ask how much of what comes back is *not* that tone. A generator's
**1k** wave is the tone; the **Distortion** module makes the measurement.

| | |
|---|---|
| **thd** | the harmonics alone, as a percentage of the fundamental. Noise is not counted, which is what makes it the number a spec sheet quotes and also what makes it flattering |
| **thd+n** | everything that is not the fundamental — harmonics, noise, hum, intermodulation. Always at least THD, and the gap between them is how much of the rubbish is hiss rather than distortion |
| **sinad** | the same ratio in decibels, the way a converter is specified |
| **enob** | the ideal-converter relation run backwards: what a perfect converter would have to be to sound this clean. A perfect *n*-bit converter has a SINAD of 6.02*n* + 1.76 dB |
| **src** | which signal is measured — mix, either channel, mid or side |
| **harm** | how many harmonics are summed. Ones past Nyquist are never counted: they are not there |

It runs every 400 ms rather than every frame. A 16384-point FFT is not free, and
a distortion figure that flickers through three digits is unreadable even when
every digit is right — the measurement is an average over its window, so showing
it faster than the window is long is showing the same audio twice.

**The instrument's own floor is about 0.0015% THD+N** — 96.5 dB SINAD, 17.3
effective bits — measured on a synthesized pure tone. That is the
Blackman-Harris window's leakage getting past the notch, not anything in the
signal, and nothing quieter than it can be read.

It was 0.013% (77.8 dB, 12.6 bits) when the analysis ran through the Hann window
the rest of the app uses. Hann's sidelobes measure −66.7 dB eight bins out and
Blackman-Harris's −96.0, and that 29 dB is the floor almost exactly — the cost
is a main lobe twice as wide, which a distortion measurement can afford because
its harmonics are octaves apart.

Checked against distortion the app generates itself, by putting a second
oscillator on the second harmonic at a known ratio:

| injected | measured |
|---|---|
| 1% | **1.000%** |
| 10% | **10.000%** |
| 50% | **50.000%** |


## RTA — octave bands

The analyzer a room is measured with. A spectrogram shows every bin, which is
right for watching sound move and wrong for asking what a room is doing to it:
linearly spaced bins put nine-tenths of the picture above 2 kHz and squeeze the
bass into a few pixels. The RTA splits the band into fractional octaves — equal
*ratios*, drawn at equal widths, which is a logarithmic frequency axis — and
shows the level in each.

| | |
|---|---|
| **band** | 1/1, 1/3, 1/6 or 1/12 octave. 1/1 is a hi-fi graphic equalizer's ten; 1/3 is what room measurement and ISO use; the finer two find a single narrow resonance |
| **src** | which signal — mix, either channel, mid or side |
| **top** / **rnge** | where the scale sits, in dBFS, and how much of it is shown |
| **avg** | the meter's averaging — quick to rise and slow to fall, as every level meter is |
| **hold** | peak-hold decay in dB/s. What makes the display readable on music, which only excites part of the band at a time |

The band centers are the standard ones — ISO 266 / ANSI S1.11, the base-ten
series — so a reading here is comparable with anybody else's. The familiar
third-octave row comes out as the 31 bands from 20 Hz to 20 kHz that every
acoustics table prints.

**Feed it pink noise** from a generator's pink wave. Fractional-octave bands get wider
in hertz as they go up, so equal power per octave is what reads *flat* — a flat
display on pink noise is the definition of a flat system, and it is the
convention room measurement is done in. White noise rises 3 dB per octave on the
same display, which is the difference between the two stimuli and the reason
pink is the one used.

Measured off the live display's own framebuffer:

| stimulus | bands | spread |
|---|---|---|
| pink, 1/1 octave | 10 | **0.0 dB** |
| pink, 1/3 octave | 31 | **3.4 dB** |
| white, 1/3 octave | 31 | **19.7 dB**, rising |


## Transfer function — magnitude, phase, coherence

Every other measurement here asks about one signal. This one asks about the
*relationship* between two, which is the question a system is actually tuned by:
send a signal into a loudspeaker, a room, a filter or a cable, capture what comes
back, and read what happened in between.

| | |
|---|---|
| **magnitude** | the frequency response in dB. Flat is a system that changed nothing |
| **phase** | how far the output lags the input. A pure **delay** is a phase that falls linearly with frequency, and the slope *is* the delay |
| **coherence** | how much of the output is linearly explained by the input. The number that says whether to believe the other two |
| **dly** | the bulk delay, fitted from the phase slope — what gets dialed into a delay line to line a speaker up |
| **ref** | which channel is the reference. A swap, because half the time the cabling makes it the other way round |
| **band** / **rnge** / **coh** / **avg** | the band smoothing, the magnitude scale, the coherence threshold below which curves are not drawn, and how many windows are averaged |

A dip in the magnitude with the coherence still high is the system; the same dip
with the coherence collapsed is the measurement giving up. The three are drawn
together so the two can be told apart, with magnitude and coherence above the
control panel where they stay visible.

**Averaging is not optional.** From a single window, coherence is exactly 1 at
every frequency — for *any* two signals, including two unrelated noises. It falls
straight out of the definition, and a display built on one window is a row of
perfect scores that means nothing. A result is refused until there are enough
windows for the number to be real.

Coherence does not catch everything, either. A single tone through a perfect wire
reports 0 dB at coherence 1 in every band — correctly, because the wire *is* flat
and the window's leakage really does pass through unchanged. True and worthless:
nothing excited those bands. A separate per-band reference level marks them, and
the delay fit ignores them.

Checked against systems whose answer is known: a gain measures as that gain, and
delays of 16, 48 and 120 samples are recovered from the phase slope to better
than 0.05 ms. Live, through the app's own test signals:

| channels | coherence gate | delay |
|---|---|---|
| identical | curves drawn | **+0.00 ms** |
| independent | curves suppressed | **-- ms** (refused) |
| polarity-inverted | curves drawn | **+0.00 ms** |

That last row is the distinction the display exists to make: a polarity flip is a
perfect linear relationship with constant 180° phase and *no* delay, which a
delay measurement must not confuse with one.


## Loudness — LUFS, loudness range, true peak

The scale everything is delivered against. A peak meter cannot answer the
question, because loudness is not peak: a compressed mix and a dynamic one can
share a peak and be ten decibels apart to listen to.

| | |
|---|---|
| **M** / **S** | momentary (400 ms) and short-term (3 s), both ungated. What it sounds like now, and over the window a person actually judges level across |
| **I** | integrated, over everything since the reset, **gated** — so silence and quiet passages do not drag a programme's number down. This is the one a delivery spec means |
| **LRA** | the loudness range: the 95th percentile of the short-term distribution less the 10th. One number for how dynamic the material is, which a loudness target alone says nothing about |
| **TP** | true peak in dBTP — the peak of the *reconstructed* signal, four-times oversampled |
| **tgt** | a delivery target (−23 EBU R 128, −14 streaming, −16 podcast) with the distance from it beside |

**K-weighting is derived from the analog prototype at whatever rate the audio is
arriving at**, not the 48 kHz coefficients the standard prints — those are only
correct at 48 kHz, and a meter using them at 44.1 would be weighting by a filter
whose corners had quietly moved. A test checks the derivation reproduces the
published numbers exactly at 48 kHz, which is what makes it the same filter
rather than a similar one.

The neatest check of the whole chain: K-weighting's gain at 997 Hz is **+0.691
dB** and the loudness equation's offset is **−0.691 dB**, because the standard
chose the offset to cancel the weighting at its reference frequency. So a 997 Hz
sine of amplitude *A* in both channels reads exactly 20·log₁₀(*A*) LUFS — and
every part of the chain has to be right for that to come out. It is also why
997 Hz rather than 1000 appears throughout BS.1770: at 1 kHz the weighting is
+0.698 dB and the identity misses by seven thousandths of a decibel.

Measured live, through the app's own test signals:

| | M | S | I | LRA | TP |
|---|---|---|---|---|---|
| 1 kHz at 0.5 | −6.0 | −6.0 | −6.0 | 0.0 | −6.0 |
| …then 20 dB quieter for 6 s | −26.0 | −26.0 | **−6.1** | **20.0** | −6.0 |

The second row is the gate and the range demonstrating themselves: the ungated
windows follow the signal down 20 LU, the integrated reading **holds**, and LRA
reports exactly the 20 LU that was applied. The formula predicts −6.014 LUFS for
the first row.

**True peak is measured only where the interpolation kernel fits.** At a buffer's
ends the taps run off the edge, and treating the audio either side as silence is
a step discontinuity — which a bandlimited interpolator answers with exactly the
overshoot the measurement exists to detect. Measured that way, a plain
0.5-amplitude tone with no intersample peak at all read 0.534: the meter
reporting the edge of its own buffer.


## Wow & flutter — speed stability

Every other analyzer here asks about amplitude. This asks whether the **time
axis** is steady, which is what a turntable, a tape deck or a cassette is judged
by. Play a generator's sine at 3150 Hz through the deck, capture it, and read
the frequency modulation.

| | |
|---|---|
| **speed** | the mean frequency error, %. A constant error is a pitch shift — a different fault from a wobble |
| **wow** | the slow 0.5–6 Hz modulation. An off-centre spindle hole is wow at exactly the platter's rate |
| **flutter** | the fast 6–100 Hz kind, from capstans and idlers, which the ear hears as roughness rather than pitch movement |
| **w&f** | the DIN-weighted quasi-peak a specification quotes: the deviation through a filter peaked at 4 Hz, where the ear is most sensitive to pitch movement |
| **nom** | what the tone is supposed to be. 3150 Hz is what the test records carry (DIN 45507, IEC 60386). Set it to 0 to measure wow and flutter without a speed figure — the honest reading when the tone's true frequency is not known |

Demodulated in quadrature rather than by counting zero crossings: the analog
instruments counted crossings, but their resolution is one sample per crossing
and the deviations here are parts in ten thousand.

The carrier is found from the signal rather than assumed. A deck running 2% fast
puts the tone at 3213 Hz, and mixing against 3150 would leave a 63 Hz beat the
demodulator would report as enormous flutter.

**With no deck in the loop it reads zero** — which is the check that the
instrument is not inventing the number:

| | speed | carrier | wow | flutter | w&f |
|---|---|---|---|---|---|
| the generator's own 3150 Hz | +0.000% | 3150.0 Hz | 0.000% | 0.000% | 0.000% |
| …with **nom** set to 3000 | **+5.000%** | 3150.0 Hz | 0.000% | 0.000% | — |
| …with **nom** set to 3200 | **−1.563%** | 3150.0 Hz | 0.000% | 0.000% | — |

That floor took two fixes. Mixing puts an image at twice the carrier, and
whatever survives the filter aliases back into the measurement band on
decimation, where it is indistinguishable from flutter — a single pole rejected
6300 Hz by only 34 dB and read as **0.022% flutter** on a perfectly steady tone,
which is within a factor of two of what a good deck is specified at. Four
cascaded poles reject it by 96 dB. The weighted figure then still read 0.035%,
which was the demodulator's *own* startup: until the filters settle, I and Q are
climbing out of zero and the argument of a vector near the origin is noise, and
the quasi-peak detector holds a peak for a second and a half.


## Waterfall — cumulative spectral decay

The measurement a loudspeaker is characterized by, and the one display here that
could only exist in this app: every other analyzer draws its own flat panel, and
this is a genuine 3-D surface that rides the same vertex pipeline the attractors
do — so it drags, rotates, zooms and takes the gradient like any other model.

Frequency runs left to right (logarithmically), level up, and **time into the
screen**. Each line is the spectrum of what is left of the impulse response from
a moment onwards, so a flat loudspeaker's surface falls away evenly and a
**resonance is a ridge running back into the screen** at one frequency. That is
what it is for: a frequency response cannot tell a resonance from a broad lift,
because they are identical in magnitude and nothing alike in time.

Feed it a generator's **log sweep** (its swp wave), with the sweep as the reference
in one channel and what came back in the other. The impulse response is recovered
by deconvolution — H = Y·conj(X)/(|X|² + ε), inverse-transformed — and the
surface is rebuilt each time a sweep pass completes. Unlike every other audio
mode the picture then **holds still** between passes, which is what makes it
something to rotate and look at rather than something to freeze first.

**A sweep takes four seconds to cross the band, so the measurement needs a whole
pass.** A third of one is 20 Hz to 200 Hz, and the impulse it recovers is a
five-millisecond smear — which is how the delay tests were failing before the
window was made long enough to hold a full pass.

The deconvolution is windowed **rectangularly**, deliberately. Every other
analysis here windows its input to stop a tone leaking across the spectrum; a
deconvolution must not, because a window is a multiplication in time and so a
convolution in frequency — it would smear the very response being recovered.

Checked against systems whose answer is known: a passthrough gives a single spike
at sample 0 of height 1, delays of 1, 17, 240 and 1000 samples put the spike at
exactly those samples, gains scale it, and an exponential decay built to fall
60 dB in a known time is recovered by both the T20 and T30 fits to within 8%.

**One limitation, stated because it is real.** The reverberation time does not
find the noise floor. A Schroeder integral of a *truncated* response always falls
away at its end — the energy remaining after the last sample is zero however loud
the signal was — so a recording that stops before the reverberation has shows a
decay that is partly its own edge. Worse, the obvious check does not catch it: a
T20 and a T30 that disagree are usually taken as the sign that a fit has run into
the floor, and measured against a pure truncation ramp they **agree** to within
6%, both returning about 1.5 s from a signal with no decay whatsoever. The usual
answer is Lundeby's method; it is not here yet.

