# FVF — Harmonic Wobbulator

[← README](../README.md)

The **FVF Wobbulator** mode is a software analog of the
Frequency→Voltage→Frequency converter with balanced modulator designed at
[bunkerofdoom.com](https://bunkerofdoom.com) (hardware built 1984):
it tracks the pitch of the incoming audio, scales/offsets it into a new
carrier frequency, and ring- or AM-modulates that carrier back with the
original signal — the metallic, glitchy timbre that "made guitar/voice sound
very strange." The processed audio is shown on the spectrogram and, with the
**🔊 Listen** switch, played back out. Knobs: gain, offset, fmin, fmax, duty,
mix (dry↔wet), glide, plus waveform (square/pulse/sub-÷2) and modulator
(ring/AM) selectors.

The **🎛 FX** switch is an instant A/B between the raw incoming audio (off) and
the wobbulated signal (on), independent of the `mix` knob; `mix` itself sweeps
dry↔wet continuously.

**Hear it with a microphone** (no setup):
open the page in mic mode (the default when the server was started without
`--audio`; `?audio=mic` asks for it explicitly when the server does capture),
choose **FVF Wobbulator**, turn on **Listen**, and use **headphones** (mic-in
and speaker-out are different devices, so there's no loop; headphones stop
acoustic feedback).

**See/hear it react to *any* app (system audio).** `--audio` captures the
default sink's monitor by default (`--audio-source monitor`), so — exactly like
audioprism — it picks up whatever is playing (VLC, a browser tab, anything)
with no per-app routing:

```
go run github.com/0magnet/chaosrack@master serve --audio
# open http://127.0.0.1:8080/#fvf  → (with headphones) Listen
```

That alone is perfect for the spectrogram and for **Listen over headphones**.
To hear the wobbulated result **out the speakers** you must break the feedback
loop — the speaker output being captured and wobbulated again, one buffer
later, forever. Breaking it means putting the source app and the browser's own
output on different sinks, and there are two ways to ask for that.

**The `route` switch**, in the FVF parameter cells beside FX and Listen, is the
one to reach for: it installs and removes the routing while the server runs, so
A/B-ing normal audio against wobbulated audio is a click rather than a restart.
It appears only on a page served by a server that can actually do it — one
started with `--audio`, on a machine with `pactl` — and only a page on that same
machine can operate it. Unlike every other switch on the panel it reflects the
*machine's* state, not the page's, so it asks the server where it stands when
the panel is built and moves back if a request is refused.

**The `--wobbulate` flag** is the same thing at start-up, for when you know you
want it:

```
go run github.com/0magnet/chaosrack@master serve --audio --wobbulate
```

Either way: a temporary null sink is inserted and made the default, so *every*
app routes into it and can be captured, and the page's own Listen output is
moved back to your real speakers automatically — the server watches for the
browser's playback stream on the null sink and moves it off, so there is no
pavucontrol step (`--wobbulate-apps` controls which app names count as "the
browser"; the app you are wobbulating should be a *different* one, e.g. VLC).

Flow: any app → null sink (silent) → its monitor → server → browser wobbulates
→ real speakers (never captured, so no loop).

The live capture is dropped when the routing changes, on purpose: a capture
resolves "the default sink's monitor" once, when it opens, so one that was
already running would go on recording the sink the audio no longer goes to. The
page reconnects two seconds later against the new default, which is why the
switch needs no reload.

**Reverting** is automatic — turning `route` off, or stopping the server
(Ctrl-C included), restores the previous default sink and removes the null
sink — and non-destructive; it is runtime-only anyway, so a logout or reboot
also clears it. A run that is *killed* rather than asked to stop cannot clean up
after itself, so the next start sweeps away any null sink it left behind, and so
does `scripts/fvf off`. To tap a specific source instead, pass its name, e.g.
`--audio-source fvf_in.monitor`.

**One-command toggle.** `scripts/fvf` drives the whole thing without a browser:

```
scripts/fvf on      # chaosrack serve --audio --wobbulate in the background (FVF_PORT=8080)
scripts/fvf off     # stop it → normal audio restored, stale sinks swept
scripts/fvf status  # and the machine's current default sink
scripts/fvf         # toggle
```


## The hardware it models

The 1984 build used a Teledyne **9400** V/F–F/V converter (Radio Shack
276-1790). The signal path, and how each stage maps onto the DSP:

```
audio in ─▶ level ─▶ hard amp/limiter ─▶ F/V (9400) ─▶ gain(.01–100) ─▶ V/F (9400) ─▶ ┐
   │                 (clip to square:                   + offset(±)     (new freq)    │
   │                  extract zero-cross rate                                         ▼
   │                  = "frequency", drop amplitude)                        ┌─ balanced /
   └──────────────────── original audio ────────────────────────────────────┤  ring modulator ─▶ OUT
                                                                            └─ (xfmrs + germanium
                                                                                diodes, "lowest Vf")
```

1. **Limiter** — amplify until the input clips to a square-ish wave. Amplitude
   is thrown away and only the zero-crossing rate survives: the audio becomes a
   bare *frequency*. `fvf_js.go` does this directly as a zero-crossing count.
2. **F/V** — square wave → DC voltage proportional to frequency (pitch → CV).
3. **Gain + offset** — scale (0.01–100×) and shift the voltage; the `gain` and
   `offset` knobs. The hardware constraint is that the voltage into the V/F must
   stay **positive and never 0**: in the 9400's single-supply V/F mode
   f_out ∝ V_in, so 0 V → 0 Hz → the carrier stops and the ring-mod output goes
   silent (audio × 0). The offset therefore sets a **floor frequency** — a
   minimum standing carrier present even in silence, with the tracked pitch
   riding on top. That is what the `fmin` clamp is for:
   `f_out = clamp(gain·f_in + offset, fmin, fmax)`, `fmin > 0`.
4. **V/F** — the modified voltage becomes a *new* frequency. The 9400 has **two**
   outputs, which are the two carrier waveforms in the mode's selector:
   - **pin 8, "pulse freq out"** — a **fixed ~3 µs pulse** repeating at `f_out`
     regardless of frequency, so duty = `3µs · f_out` *grows with pitch*. Low
     notes give a very short, bright, harmonic-rich pulse that squares up as
     pitch rises. This frequency-dependent harmonic content is much of the
     "harmonic" character, and is what the **pulse** waveform models (the `duty`
     knob generalizes it).
   - **pin 10, "freq ÷2 out"** — flip-flop-divided `f_out/2` at 50 % duty: a
     clean sub-octave square, odd harmonics only. The **sub-÷2** waveform.
5. **Balanced/ring modulator** — transformer + germanium-diode ring modulator
   multiplying the regenerated carrier by the **original** audio. Ring
   modulation produces sum and difference frequencies, hence the metallic,
   bell-like, inharmonic timbre.

[9400 datasheet](https://ww1.microchip.com/downloads/en/DeviceDoc/21483d.pdf)

**The crudeness is the feature.** The clip-and-count pitch tracker locks to the
dominant zero-crossing rate, which on complex, polyphonic or noisy input (voice,
guitar chords) jumps around — octave errors, glitches, wild excursions. That is
the *wobbulator*. Detune the carrier and the ring-mod sidebands go inharmonic.
Modelling the quirks rather than fixing them is the point, so tracking latency
is kept low rather than smoothed away. Family resemblance: ring modulators, Bode
frequency shifters, guitar-synth pitch extraction, EHX Frequency Analyzer.

A tube-era ancestor from the '60s used a **6BN6 gated-beam tube** (a quadrature
FM detector / product mixer), a VR105 regulator, and a **CRT** driven in
quadrature (+180° / +270°) for a vector/Lissajous display of the twisted
waveforms.

**Open questions about the original.** A diode ring modulator is a switching
multiplier: one port is the **carrier** (switching the germanium diodes — hence
"lowest Vf"), the other passes the **signal** linearly. Unresolved: which V/F
output drove the carrier (the bright fixed-width pulse, or the ÷2 square);
which signal drove which port; and the relative drive levels, which set clean
four-quadrant ring modulation against a more gated/AM character. All three are
switchable here and can be tuned by ear.
