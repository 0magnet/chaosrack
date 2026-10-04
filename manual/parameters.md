# Parameters

## Parameters

<!-- key: params -->
Parameters — the current model's tunable constants (each with knob, LED value, step size, and reset)

### depth

<!-- key: p.bif-depth -->
depth — how much of the swept parameter's range the audio moves the cursor across, centered on wherever that parameter's own knob is set. At 1 the envelope covers the whole axis, which mostly reads as a level meter lying on its side; a third is enough to cross a bifurcation without every drum hit crossing all of them. At 0 the cursor stays on the knob. Live only while DRIVE is on audio.

### latitude lines

<!-- key: p.globe-lat -->
latitude lines — how many parallels circle the globe, or, with par on spiral, how many times the one spiral winds round on its way from pole to pole.

### longitude lines

<!-- key: p.globe-lon -->
longitude lines — how many meridians run from pole to pole.

### parallels

<!-- key: p.globe-par -->
parallels — how the lines of latitude are drawn: as separate rings, or as one continuous spiral from pole to pole.

### direction

<!-- key: p.globe-rev -->
direction — which way the spiral of parallels winds. Only acts with par on spiral; the rings have no direction.

### twist

<!-- key: p.globe-twist -->
twist — turns each meridian as it descends, so the lines of longitude become helices. Zero is the ordinary globe; the sign is which way they lean.

### latitude bands

<!-- key: p.sphere-stacks -->
latitude bands — how many slices the sphere is cut into from pole to pole.

### longitude segments

<!-- key: p.sphere-slices -->
longitude segments — how many wedges the sphere is cut into around its axis.

### major radius

<!-- key: p.torus-R -->
major radius — from the center of the hole to the middle of the tube.

### minor radius

<!-- key: p.torus-r -->
minor radius — the thickness of the tube.

### rings

<!-- key: p.torus-stacks -->
rings — how many segments run around the torus's big circle.

### sides

<!-- key: p.torus-slices -->
sides — how many segments run around the tube.

### roll

<!-- key: p.torus-roll -->
roll — how fast the tube turns about its own core circle, signed; zero holds it still.

### a

<!-- key: p.lissajou-a -->
a — the X frequency. With a, b and c whole numbers the curve closes on itself, and their ratio is its shape.

### b

<!-- key: p.lissajou-b -->
b — the Y frequency.

### c

<!-- key: p.lissajou-c -->
c — the Z frequency, which gives the figure its depth.

### level A

<!-- key: p.ga-la -->
level A — how much of the master oscillator A reaches the vertical deflection.

### level B

<!-- key: p.ga-lb -->
level B — the depth of the B envelope that modulates the ±45° carrier on both axes; what gives the figure its apparent volume.

### level D

<!-- key: p.ga-ld -->
level D — how much of oscillator D reaches the horizontal deflection.

### harmonic B

<!-- key: p.ga-hb -->
harmonic B — B's frequency as a whole multiple of the master A.

### harmonic C

<!-- key: p.ga-hc -->
harmonic C — the carrier's multiple of A. Higher hatches the wireframe more densely.

### harmonic D

<!-- key: p.ga-hd -->
harmonic D — D's multiple of A. At 1 it sets the base rectangle against A.

### waveform A

<!-- key: p.ga-wa -->
waveform A — triangle, or square, which breaks the figure into its hard-edged families. With B, C and D, the article's four switches: sixteen families.

### waveform B

<!-- key: p.ga-wb -->
waveform B — triangle, or square, which breaks the figure up.

### waveform C

<!-- key: p.ga-wc -->
waveform C — triangle, or square, which breaks the figure up.

### waveform D

<!-- key: p.ga-wd -->
waveform D — triangle, or square, which breaks the figure up.

### harmonics

<!-- key: p.stext-harm -->
harmonics — how many Fourier harmonics of each glyph's beam path are kept. Few and the letters melt into loops; many and they sharpen.

### system

<!-- key: p.smorph-sys -->
system — where in the Sprott A…S catalog the machine sits. A fraction is part way between two systems.

### rate

<!-- key: p.smorph-rate -->
rate — how fast the machine steps itself through the catalog, in systems per minute; zero holds it where sys puts it.

### gravity

<!-- key: p.bounce-grav -->
gravity — how hard the ball is pulled down.

### bounce

<!-- key: p.bounce-rest -->
bounce — how much of its energy the ball keeps at each bounce.

### drift

<!-- key: p.bounce-drift -->
drift — the ball's horizontal speed.

### height

<!-- key: p.bounce-height -->
height — where the next Drop releases the ball from, in court units above the floor.

### speed

<!-- key: p.pong-speed -->
speed — how fast the ball travels.

### paddle

<!-- key: p.pong-paddle -->
paddle — the paddles' height.

### skill

<!-- key: p.pong-skill -->
skill — how well the machine player tracks the ball.

### left

<!-- key: p.pong-pad-l -->
left — the left paddle pot. Turn it to take the paddle from the machine; while the machine or the keys drive it, it turns by itself to follow.

### right

<!-- key: p.pong-pad-r -->
right — the right paddle pot. Turn it to take the paddle from the machine; while the machine or the keys drive it, it turns by itself to follow.

### sides

<!-- key: p.polyhedron-p -->
sides — how many sides each face has: 3 triangles, 4 squares, 5 pentagons, 6 hexagons. With meet it names the shape {sides, meet}: from the tetrahedron {3,3}, one more side is the cube {4,3}.

### meet

<!-- key: p.polyhedron-q -->
meet — how many faces meet at each corner. From the tetrahedron {3,3}, one more is the octahedron {3,4} and two more the icosahedron {3,5}. Where the corners' angles make a full turn the faces lie flat (a tiling of the plane); past that they tile the hyperbolic plane, drawn in Poincaré's disk.

### morph

<!-- key: p.polyhedron-morph -->
morph — cuts the solid's corners off, deeper and deeper: at 1 the cuts meet (the rectified solid, the cuboctahedron between the cube and the octahedron) and at 2 it is the dual. + and − step to the next named solid, 0 back to the solid. Acts on a solid, not a tiling.

### operator

<!-- key: p.polyhedron-op -->
operator — the Conway operator applied to the solid (dual, ambo, truncate, kis, expand, bevel). Acts on a solid, not a tiling.

### kis

<!-- key: p.polyhedron-kis -->
kis — a pyramid on every face: out above 0, pressed in below. + raises them, 0 flattens them, − presses them in. Acts on a solid, not a tiling.

### modulus

<!-- key: p.turtle-mod -->
modulus — each term of the sequence is reduced modulo this before it turns the turtle; 0 walks the sequence unreduced.

### sequence

<!-- key: p.turtle-seq -->
sequence — which integer sequence drives the walk.

### multiplier

<!-- key: p.turtle-mul -->
multiplier — multiplies the Fibonacci sequence by this.

### cap

<!-- key: p.turtle-cap -->
cap — how many terms are walked; 0 lets the modulus choose (one full period).

### dimensions

<!-- key: p.turtle-dim -->
dimensions — whether the walk turns in a plane (2) or in space (3).

### tint

<!-- key: p.turtle-tint -->
tint — what the path's color follows.

### trail

<!-- key: p.turtle-trail -->
trail — how much of the walk stays drawn: all of it, a long or short tail, or a comet.

### camera

<!-- key: p.turtle-cam -->
camera — auto fits a figure that closes and locks one that drifts; fit scales it into the box; lock cancels the drift; follow keeps the head in the middle.

### view

<!-- key: p.turtle-view -->
view — which way to face the figure's axis.

### cycle

<!-- key: p.turtle-cycle -->
cycle — seconds between steps to the next modulus; 0 stays on one.

### gravity

<!-- key: p.turtle-grav -->
gravity — pull in world units per second squared; below zero lifts.

### friction

<!-- key: p.turtle-fric -->
friction — how much the figure is slowed where it touches.

### bounce

<!-- key: p.turtle-bounce -->
bounce — how much of its speed the figure keeps off a wall.

### spin

<!-- key: p.turtle-spin -->
spin — how hard the figure is to turn.

### gain

<!-- key: p.xy-gain -->
gain — how far a sample deflects the beam; turn it up for a quiet source, down for one that runs off the edge.

### window

<!-- key: p.xy-win -->
window — how much recent audio is on screen, in milliseconds: the length of the trace, not its shape.

### glow

<!-- key: p.xy-persist -->
glow — phosphor afterglow: how long an old trace fades rather than being cleared each frame. 0 clears every frame.

### lag

<!-- key: p.xy-lag -->
lag — on a mono source, the second axis is the same signal this many milliseconds later, so the figure opens out from the diagonal a mono pair would draw.

### smooth

<!-- key: p.xy-smooth -->
smooth — drawn points per sample: the beam is curved through the samples instead of joined by straight chords. Higher is smoother and costs vertices.

### DFT size

<!-- key: p.spect-dft -->
DFT size — points per transform: larger resolves frequency finer and time coarser.

### overlap

<!-- key: p.spect-ovl -->
overlap — how much each transform overlaps the one before, in percent: more is a smoother scroll for more work.

### window function

<!-- key: p.spect-win -->
window function — the taper each block is shaped by before its transform: Hann is the usual; rectangular is sharpest and leaks the most.

### channel

<!-- key: p.spect-chan -->
channel — which signal is analyzed: the mix of both, or the left or right alone.

### floor

<!-- key: p.spect-min -->
floor — the level, in dB, that maps to the bottom of the color scale; anything quieter is drawn black.

### ceiling

<!-- key: p.spect-max -->
ceiling — the level, in dB, that maps to the top of the color scale; anything louder is drawn at full.

### gain

<!-- key: p.fvf-gain -->
gain — the voltage-to-frequency slope: output pitch = gain × input pitch + offset.

### offset

<!-- key: p.fvf-offset -->
offset — hertz added to every output pitch: a transposition, and a floor.

### lowest frequency

<!-- key: p.fvf-fmin -->
lowest frequency — the carrier never goes below this (a V/F converter cannot reach 0 Hz).

### highest frequency

<!-- key: p.fvf-fmax -->
highest frequency — the carrier's ceiling.

### duty

<!-- key: p.fvf-duty -->
duty — the width of each output pulse, as a fraction of its period: the brightness of the tone.

### mix

<!-- key: p.fvf-mix -->
mix — dry to processed: 0 is the input alone, 1 the converter's output alone.

### glide

<!-- key: p.fvf-glide -->
glide — how smoothly the pitch follows: low is snappy and glitchy, which is the faithful behavior; high glides.

### source

<!-- key: p.rec-src -->
source — what is plotted: the raw audio, a delay embedding of it, or the running attractor's own trajectory.

### window

<!-- key: p.rec-win -->
window — how much history the square covers, in milliseconds (for the trajectory, in the system's own time).

### ε

<!-- key: p.rec-eps -->
ε — the recurrence threshold, as a fraction of the source's scale: two moments closer than this are marked as a recurrence.

### m

<!-- key: p.rec-dim -->
m — the embedding dimension: how many delayed copies make up each point on the embed source.

### band

<!-- key: p.xf-frac -->
band — the smoothing, as a fraction of an octave: 1/1 is broad, 1/12 is fine.

### average

<!-- key: p.xf-avg -->
average — how many windows are averaged: more is steadier and slower to follow a change.

### range

<!-- key: p.xf-range -->
range — dB either side of 0 on the magnitude curve: the vertical scale.

### coherence

<!-- key: p.xf-coh -->
coherence — the minimum coherence, in tenths, below which a band is not trusted and is drawn faded.

### show

<!-- key: p.xf-show -->
show — which curves are drawn: magnitude, phase and coherence together, or one of them alone.

### reference

<!-- key: p.xf-swap -->
ref — which channel is the reference, the signal sent into the system; the other is what came back out of it.

### band

<!-- key: p.rta-frac -->
band — the width of each bar, as a fraction of an octave: 1/1 is a graphic equalizer's ten bands, 1/3 is what room measurement uses, and 1/6 and 1/12 find a single narrow resonance.

### source

<!-- key: p.rta-chan -->
src — which channel is analyzed.

### top

<!-- key: p.rta-top -->
top — the level at the top of the scale, in dBFS.

### range

<!-- key: p.rta-range -->
rnge — how many decibels the scale shows below TOP.

### average

<!-- key: p.rta-avg -->
avg — the meter's averaging: 0 shows each analysis as it lands, for watching a transient; higher is steadier, and the bars always rise faster than they fall.

### hold

<!-- key: p.rta-hold -->
hold — how fast the held peaks fall, in dB per second; 0 holds nothing. Held peaks are what make the display readable on music, which excites part of the band at a time.

### source

<!-- key: p.wfall-src -->
src — which surface is drawn: DCAY is the cumulative spectral decay of an impulse response, measured from a generator's log sweep; LIVE is successive spectra of whatever is playing, stacked into the screen as they age.

### channel

<!-- key: p.wfall-chan -->
chan — which channel the live surface analyzes. The decay surface uses both channels, and REF says which is the reference.

### lines

<!-- key: p.wfall-lines -->
line — how many slices the surface has. With STEP it sets how far back in time the surface reaches.

### step

<!-- key: p.wfall-step -->
step — milliseconds between slices: sixteen lines at 5 ms is the 80 ms a loudspeaker's resonances live in, thirty-two at 40 ms is the length of a bar of music.

### FFT

<!-- key: p.wfall-fft -->
fft — the transform each slice is taken with: shorter resolves time better and the bass worse. A window longer than STEP means neighboring slices see the same audio.

### top

<!-- key: p.wfall-top -->
top — the level at the top of the scale, in dBFS.

### range

<!-- key: p.wfall-range -->
rnge — how many decibels the scale shows below TOP.

### depth

<!-- key: p.wfall-depth -->
dpth — how far back the surface reaches on screen. This is geometry, not time: LINE and STEP set the time.

### reference

<!-- key: p.wfall-swap -->
ref — which channel carries the sweep, for the decay surface; the other is what came back.

### axes

<!-- key: p.xy-basis -->
axes — LR plots left against right; MS turns the display 45° into mid and side, the broadcast orientation, where center content lies along one axis and difference content along the other.

### scale

<!-- key: p.spect-scale -->
scale — the magnitude scale: logarithmic, in dB, or linear. FLOOR and CEILING are read in the same scale.

### axis

<!-- key: p.sect-axis -->
axis — the axis the section plane is perpendicular to.

### position

<!-- key: p.sect-pos -->
pos — where the plane sits along AXIS, as a fraction of the attractor's own reach: 0 is through the middle whatever the system's size.

### direction

<!-- key: p.sect-dir -->
dir — which way through the plane counts as a crossing. One way is the default because a bounded flow that goes up through a plane must come back down, and keeping both superimposes two different sections.

### view

<!-- key: p.sect-view -->
view — PLANE draws the crossings where they are in space, FLAT lays the section out face on, and MAP is the first-return map: each crossing plotted against the next.

### tau

<!-- key: p.takens-tau -->
tau — the delay between the coordinates, in samples at 48 kHz, so one position is the same duration on any source. Too short and the axes are nearly the same sample and the figure collapses onto the diagonal; too long and it folds back on itself. One knob: Takens, Polar and the recurrence plot all read it.

### window

<!-- key: p.takens-win -->
window — how much recent audio is on screen, in milliseconds. The length of the trace, not its shape.

### gain

<!-- key: p.takens-gain -->
gain — world units a full-scale sample maps to. The camera fit tracks this, so the figure stays the same size on screen; it sets the scale the geometry is built at, not how big it looks.

### smooth

<!-- key: p.takens-smooth -->
smooth — Catmull-Rom upsampling, in drawn points per source sample. A straight line between samples draws chords that are an artifact of the drawing; this curves the beam through them instead. Higher is smoother and costs vertices.

### source

<!-- key: p.takens-chan -->
source — which channel of the live pair is reconstructed.

### axes

<!-- key: p.stereo-axes -->
axes — what the three coordinates are. The two 't' positions put TIME on the third axis and draw a goniometer sweeping like a scope trace; the two 'd' positions put a delayed copy there and draw a delay embedding of the pair.

### tau

<!-- key: p.stereo-tau -->
tau — the delay used by the two 'd' axis positions. Inert on the time positions, which have no delayed coordinate.

### window

<!-- key: p.stereo-win -->
window — how much recent audio is on screen, in milliseconds.

### gain

<!-- key: p.stereo-gain -->
gain — the scale the geometry is built at. The camera fit tracks it, so this does NOT change how big the figure looks; vg is the one that does.

### align

<!-- key: p.stereo-align -->
align — an inter-channel delay, signed, in samples at 48 kHz: right is read this many later than left. A spaced pair of microphones or a mis-clocked converter opens the figure into a rotating ellipse; dial the offset out until it collapses back onto the diagonal and the knob reads how far apart they were.

### width

<!-- key: p.stereo-width -->
width — mid/side width on the DRAWN figure only: below 1 the difference content shrinks toward mono, above 1 it spreads. Nothing is written back to the audio, so the correlation meter goes on reading the source as it is and this asks 'what would widening do'.

### vertical gain

<!-- key: p.stereo-vg -->
vertical gain — scales the two signal axes against the frame, the way a scope's vertical knob does. The camera fit ignores it, so a loud passage can be turned down to fit or a quiet one driven off the top.

### trigger

<!-- key: p.stereo-trig -->
trigger — what fixes the START of the window. OFF ends it at the newest sample, so a steady tone is redrawn at a different phase each frame and the figure slides. RISE and FALL start it where the signal crosses the level, so successive frames begin at the same phase and a periodic signal stands still. LOCK is the one for music: it matches the SHAPE of the last frame instead of looking for an edge, so nothing has to cross a level and what holds still is the whole waveform rather than one point on it — which is what works on chords and speech, where an edge trigger cannot. Nothing to lock to and it free-runs rather than blanking, which is the AUTO behavior of a bench scope.

### trigger source

<!-- key: p.stereo-tsrc -->
trigger source — which signal the trigger watches. It need not be what is drawn: locking to MID holds the whole figure, and locking to one channel is how to hold a figure whose other channel is the busy one.

### trigger coupling

<!-- key: p.stereo-tcpl -->
trigger coupling — what reaches the trigger. DC passes everything, LF reject high-passes so bass and offset stop dragging the crossing around, HF reject low-passes so hiss and cymbals stop producing crossings of their own. This is the "which frequency do I trigger on" control, spelled the way a scope spells it.

### run mode

<!-- key: p.stereo-trun -->
run mode — what happens when NOTHING triggers, which is the whole question on music, where a lock comes and goes. AUTO draws anyway so the display is never blank. NORMAL holds the last triggered frame, so a figure that did hold stays up to be read instead of dissolving the moment the signal changes. SINGLE catches the next trigger and freezes; move this knob to re-arm.

### holdoff, milliseconds

<!-- key: p.stereo-hold -->
holdoff, milliseconds — the minimum quiet before an edge counts. Zero takes whichever edge is newest, so a waveform with several crossings per pattern locks to a different one each frame. Set near the length of a bar and a loop stands still; near a period and single cycles do. This is the control that locks onto a PATTERN rather than a cycle inside one.

### graticule

<!-- key: p.stereo-grat -->
graticule — the reference lines a goniometer is read against. Which lines carry the meaning follows the AXES dial: on the L/R positions the DIAGONALS do (x=y is in phase, x=-y is the content that vanishes when summed to mono) and on mid/side the AXES do (side=0 is mono, mid=0 is entirely out of phase). Scaled with gain and vg, so the lines stay with the trace.

### trigger position

<!-- key: p.stereo-tpos -->
trigger position — where the trigger point sits in the window, 0 at the left edge and 1 at the right. Past zero the window holds audio from BEFORE the edge, which is how to see what led up to a transient rather than only what followed it. A scope calls this the horizontal position; it costs that much more older audio to keep.

### noise reject

<!-- key: p.stereo-hyst -->
noise reject — how far past the level the signal must go before a crossing counts. Zero triggers on every dither across the level, which on program material means the figure flickers between two phases a sample apart. A few hundredths is usually enough.

### level

<!-- key: p.stereo-lvl -->
level — where the trigger looks for its crossing, in units of full scale. 0 is the zero crossing and is what to use unless the signal has an offset or the interesting edge is part way up it.

### span

<!-- key: p.stereo-span -->
span — timebase. Stretches the TIME axis only, so the trace can sweep across a wide window instead of sitting in a small square. Inert on the two 'd' positions, where all three axes are signal and stretching one would be a distortion.

### map

<!-- key: p.polar-map -->
map — how sample magnitude becomes radius: tanh and soft squash gently, dB is logarithmic, unit discards magnitude and keeps direction only, drawing on the unit sphere.

### drive

<!-- key: p.polar-drive -->
drive — how hard the signal is pushed into the map before it squashes. Low leaves quiet material near the center; high pushes everything out toward the surface.

### window

<!-- key: p.polar-win -->
window — how much recent audio is on screen, in milliseconds.

### gain

<!-- key: p.polar-gain -->
gain — the scale the geometry is built at; the camera fit tracks it.

### source

<!-- key: p.polar-chan -->
source — which channel of the live pair is reconstructed.

## Model Out

The model as a sound: its own equations run at audio rate, so its x, y and z
are three signals the Mixer can put on the speakers and the rack's signal.
These two sit at the top of the bank's last column for every model that has a
sound.

### SPD

<!-- key: p.mo-spd -->
Model Out SPD — how fast the model runs as a signal: integrator steps per sample, in semitones. 0 is one step a sample; every 12 up is twice as fast, an octave higher. The pitch is the system's own.

### LVL

<!-- key: p.mo-lvl -->
Model Out LVL — the level of the model's x, y and z, as the Mixer takes them (0–100).

### SPEED

<!-- key: speed-cell -->
SPEED — the model's time base, at 1.12.3 below Model Out. For a flow it sets integration sub-steps per drawn vertex above ×1, and scales dt below ×1; for an animated model (Lissajous, turtle, pong, morph) it sets the playback rate. Dark for models with no time base of their own (geometry, maps, analysis displays).

<!-- key: slider-value-speed -->
Speed value, ×0.01 to ×100 — type or scroll

<!-- key: speed-slider -->
Speed — model time-base multiplier (logarithmic, ×0.01 to ×100)

<!-- key: rst-speed -->
Reset speed (×1)

### Stereo AXES positions

- <!-- key: p.stereo-axes=0 --> L/R delay embedding — L, R, L(t−τ)
- <!-- key: p.stereo-axes=1 --> L/R goniometer — L, R, time sweep
- <!-- key: p.stereo-axes=2 --> mid/side delay embedding — M, S, M(t−τ)
- <!-- key: p.stereo-axes=3 --> mid/side goniometer — M, S, time sweep

### Stereo TRIG positions

- <!-- key: p.stereo-trig=0 --> off — the window ends at the newest sample and the figure slides
- <!-- key: p.stereo-trig=1 --> rising — start where the signal crosses the level going up
- <!-- key: p.stereo-trig=2 --> falling — start where it crosses going down
- <!-- key: p.stereo-trig=3 --> lock — match the shape of the last frame, with no level or edge involved

### Stereo trigger SRC positions

- <!-- key: p.stereo-tsrc=0 --> mid — the sum of the pair, which is what both axes are built from
- <!-- key: p.stereo-tsrc=1 --> left — lock to the left channel alone
- <!-- key: p.stereo-tsrc=2 --> right — lock to the right channel alone

### Stereo trigger CPL positions

- <!-- key: p.stereo-tcpl=0 --> DC — the whole signal reaches the trigger
- <!-- key: p.stereo-tcpl=1 --> LF reject — high-passed, so bass and offset stop dragging the crossing
- <!-- key: p.stereo-tcpl=2 --> HF reject — low-passed, so hiss and cymbals stop triggering on themselves

### Stereo trigger RUN positions

- <!-- key: p.stereo-trun=0 --> auto — draw anyway when nothing triggers, so the display never goes blank
- <!-- key: p.stereo-trun=1 --> normal — hold the last triggered frame until the next trigger
- <!-- key: p.stereo-trun=2 --> single — catch the next trigger and freeze; move this knob to re-arm

### Stereo GRAT positions

- <!-- key: p.stereo-grat=0 --> off — no reference lines
- <!-- key: p.stereo-grat=1 --> axes — the two coordinate axes only
- <!-- key: p.stereo-grat=2 --> full — axes, diagonals and the unit box

### Model switches

#### ITER (Custom)

<!-- key: sw.custom.iter -->
iterate — read the expressions as a discrete MAP (x = f(x,y,z)) instead of as derivatives to integrate (x += dt·f). No dt, no path between iterates, so it draws as points. Type 1 - 1.4x^2 + y and 0.3x for Henon.

#### SURF (Custom)

<!-- key: sw.custom.surf -->
surface — read the first expression as F(x, y, z) and draw the surface where it is zero, by its contours: slices across each axis, traced where each meets it. Try x^2 + y^2 + z^2 - 1 (a sphere), max(abs(x), abs(y), abs(z)) - 0.6 (a cube), or sin(3x)cos(3y) + sin(3y)cos(3z) + sin(3z)cos(3x) (a gyroid). Edit a Polyhedron's equation to start from a solid.

#### 4D W (Custom)

<!-- key: sw.custom.4d -->
4D — add a fourth state variable w with its own dw/dt equation (hidden from the 3D plot, fed back through the others). Flows only: a map or a surface has no fourth state.

#### PASS (Desk)

<!-- key: sw.desk.pass -->
Pass-through — where the mouse goes while the Desk MODEL is on screen. Off: dragging turns the model, and Ctrl-drag reaches the desk. On: dragging reaches the desk — moving windows, resizing them, pressing their buttons — and Ctrl-drag turns the model. Control always inverts the switch, so both gestures are available either way. The keyboard needs neither: double-click to type into the focused window, Esc to give it back.

#### RSET (Pong)

<!-- key: sw.pong.rset -->
Restart — zero both scores and serve fresh (springs back)

#### DROP (Bounce)

<!-- key: sw.bounceball.drop -->
Drop — release the ball again from the height knob's setting with a fresh drift (springs back)

#### LOAD (STL file)

<!-- key: sw.stlfile.load -->
Load — open an .stl file from disk, binary or ASCII (springs back)

#### FX (FVF)

<!-- key: sw.fvf.fx -->
FX — on: wobbulated (processed) audio; off: the raw incoming audio straight through (instant A/B, independent of the MIX knob; affects both sound and spectrogram)

#### LSTN (FVF)

<!-- key: sw.fvf.lstn -->
Listen — play the wobbulated audio out the speakers (mic: use headphones; music: see the null-sink setup)

#### ROUT (FVF)

<!-- key: sw.fvf.rout -->
Route — send ALL system audio through a temporary null sink on the server's machine, so the wobbulated result can be played out the speakers without being captured and wobbulated again. Turn it on, play something in any app, then turn on Listen. Off restores the previous default sink; so does stopping the server. Only a page on the same machine can switch it.

#### TRND (Recurrence)

<!-- key: sw.recurrence.trnd -->
Trend — show RR, DET and LAM over the last {seconds} seconds on this screen, newest at the right, one column per measurement (about six a second), with vertical rules every 10 s. Off, the screen shows the model as it does for every other one. THE THREE PANES DO NOT SHARE A SCALE; what they share is the time axis. RR is drawn as its square root, because RR is a lit fraction of a SQUARE and the root of an area is the fraction of the side — the shaded band is the 1–5% the plot is readable in, so turn ε until the trace sits in it. DET and LAM are drawn as themselves, 0 at the bottom and 1 at the top: DET climbing is structure appearing, DET falling off is noise or a change of regime. A break in a trace is never a value — it is a stretch with no measurement behind it.

#### RT60 readout (Waterfall)

<!-- key: ro.rt60 -->
Reverberation time of the room, in seconds — how long a sound takes to fall 60 dB after it stops. Taken from the same impulse response the surface is, by Schroeder backward integration: the decay curve is the energy REMAINING after each moment, which turns a noisy decay into a smooth one without averaging repeated measurements. Measured as T20 and extrapolated — the straight part between -5 dB and -25 dB, because the first few decibels are direct sound and the last of a real decay is in the noise floor. Blank on the live surface, which has no impulse to decay from, and blank until a sweep has been measured.

### Waterfall FFT positions

- <!-- key: p.wfall-fft=0 --> 1024 — 21 ms window, 47 Hz bins: the sharpest in time and the blindest in the bass
- <!-- key: p.wfall-fft=1 --> 2048 — 43 ms window, 23 Hz bins: the decay surface's default
- <!-- key: p.wfall-fft=2 --> 4096 — 85 ms window, 12 Hz bins: the live surface's default
- <!-- key: p.wfall-fft=3 --> 8192 — 171 ms window, 5.9 Hz bins: separates low notes, smears anything quick

### Channel positions (every CHAN knob)

- <!-- key: tap-chan=0 --> mix — the two channels summed: what a mono meter would read
- <!-- key: tap-chan=1 --> left — the left channel alone
- <!-- key: tap-chan=2 --> right — the right channel alone
- <!-- key: tap-chan=3 --> mid — the sum, halved: what both channels agree on, and what a mono listener hears
- <!-- key: tap-chan=4 --> side — the difference, halved: what the two channels disagree about, which is the stereo width itself

### Equation

<!-- key: equation -->
Equation — the editable system: one derivative expression per state variable; commits on Enter/blur

<!-- key: equation.map -->
Equation — the editable system: one expression per state variable giving its NEXT value (a discrete map); commits on Enter/blur

<!-- key: equation.surface -->
Equation — the editable surface: F(x, y, z), drawn where it is zero; commits on Enter/blur

<!-- key: equation.running -->
Equation — the running model's system, one derivative per state variable. Edit a line to make it your own (Custom).

### Section (Poincaré)

<!-- key: sect-module -->
Where the Poincaré section's plane sits, and which way through it counts. AXIS and POS place it — POS as a fraction of the attractor's own reach along that axis, so 0 is through the middle whatever the system's size. DIR one way is the default: a bounded flow that goes up through a plane must come back down through it, so counting both superimposes two different sections. The crossings draw in gold where they physically are; Analysis → Poincaré Section is the same section as a picture of its own, with the return map.

### Every parameter's knob

`{label}` is the parameter's name; `{min}` and `{max}` are its range.

<!-- key: param-slider -->
{label} — attractor parameter (range {min} … {max})

<!-- key: param-step -->
Step size for {label} — how much one knob step changes the value

<!-- key: param-step-knob -->
Step size for {label} — turn for ten times finer or coarser

### Globe LAT positions

- <!-- key: p.globe-lat=0 --> spiral, clockwise — the parallels drawn as one continuous spiral from pole to pole
- <!-- key: p.globe-lat=1 --> rings — the parallels drawn as separate rings of latitude
- <!-- key: p.globe-lat=2 --> spiral, counterclockwise — the same spiral wound the other way round

### Recurrence readouts

<!-- key: ro.rr -->
RR — recurrence rate, the percentage of the square that is lit: the number to turn ε by, and 1–5% is the readable range.

<!-- key: ro.det -->
DET — determinism, the percentage of lit points lying on diagonal lines, which is what separates a system from noise: an orbit reads near 100, white noise near 0. The line of identity is left out: every point recurs with itself, and counting that would give noise a confident score for nothing.

<!-- key: ro.lam -->
LAM — laminarity, the percentage on vertical lines: states the system sat in rather than passed through, so high LAM against lower DET is intermittency.

### Takens readouts

<!-- key: ro.meas -->
Measured embedding, in milliseconds: τ is the first minimum of the signal's average mutual information, written into the τ knob; m is the false-nearest-neighbor dimension. m greater than 3 means the trail on screen is a projection of a higher-dimensional reconstruction. This runs by itself once the mode has enough audio, and again when the source changes — but never over a τ you have set yourself. The button measures again on demand.

<!-- key: takens-measure -->
Measure the embedding from the audio in the buffer and set τ from it. Once, on demand — this mode deliberately does not re-tune itself per frame, because a knob that moves with the music makes the figure move with it too.

### Bifurcation readouts

<!-- key: ro.cur -->
Where the audio envelope currently puts the swept parameter — the cursor's position on the diagram's x axis. "sweep" means the audio drive is off; "mod off" means it is selected but Audio mod is not on, so there is no envelope to follow.

### Transfer readouts

<!-- key: ro.dly -->
Bulk delay between the two channels, fitted from the slope of the phase — a pure delay is a phase that falls linearly with frequency, and the slope is the delay. This is the number a system-tuning rig is reached for: it is what gets dialed into a delay line to line a loudspeaker up with the rest of the system. Fitted only across bands the stimulus actually reached and whose coherence clears the COH knob, and on the UNWRAPPED phase — a real delay turns through 360° many times across the band, and a slope fitted to the wrapped curve is a slope fitted to a sawtooth.

### Lyapunov readout

### Spare readout

<!-- key: ro.spare -->
Spare readout — nothing is assigned to this position.

### dt

<!-- key: dt-help -->
dt — the integration time step: how far the system advances on each step. Smaller follows the flow more faithfully and moves along it more slowly; too large and the solver leaves the attractor or blows up.

#### λ (Lyapunov readout)

<!-- key: ro.lyap -->
Largest Lyapunov exponent, measured live from a pair of trajectories started a hair apart: how fast two nearby states of THIS system, at these coefficients, separate. Positive is chaos — prediction has a horizon of roughly 1/λ — and the bigger it is the shorter that horizon. About zero is a limit cycle or a torus. Negative is settling to a fixed point. Per unit of MODEL time, not per second: it does not change when the browser is busy, and it does change with dt and with Speed, because the thing being integrated changes with them. "λ --" means not enough model time has been averaged yet for the number to mean anything; it clears itself after a second or so. Analysis → lyap is the same quantity measured at length on demand, to three decimals.

#### r (XY correlation)

<!-- key: ro.corr -->
Correlation between the two channels over the displayed window, as a goniometer's correlation meter reads it: +1.00 means the channels are identical and the figure is the diagonal line, 0 means they are unrelated and the figure is a round cloud, −1.00 means one is the other's polarity inverted — and that is the content that disappears if the mix is summed to mono. Measured on L and R whatever the axes knob is set to, because it is a property of the channels rather than of the way they are drawn. "mono" means the source has only one channel, so there is no stereo relationship to read. "r --" means silence, or one dead channel: nothing to correlate.

<!-- key: bif-sweep-cell.live -->
SWEEP {mode} — which of its parameters is the x axis. The system is the most recent flow mode: switch to an attractor, tune it, then come back.

### The equation's lines

`{v}` is the state variable a line is for and `{mode}` the running model.

<!-- key: equation.readonly -->
Read-only: this line follows the solid's knobs in the bank.

<!-- key: equation.solid -->
The solid as an equation: F = 0 on its surface, negative inside. Edit it to make your own: the rack switches to Custom, drawing your surface.

<!-- key: equation.no-w -->
d{v}/dt — {mode} has three states, so there is no fourth equation.

<!-- key: equation.not-system -->
d{v}/dt — {mode} is not a system of equations the editor can show: it is built, measured or played rather than integrated.

<!-- key: equation.line -->
d{v}/dt for {mode}. Edit it to make your own: the rack switches to Custom, seeded with this system and your change.

### Physics (Turtle Path)

<!-- key: phys-module -->
The figure as a rigid body in the plane of the screen, inside a room whose walls are the edges of the picture. GRAV pulls either way up; FRIC is how much the surfaces bite; BOUNCE is how much of the speed a wall gives back; SPIN is how readily it turns.

### Unassigned switch

<!-- key: sw.unassigned -->
Unassigned — {mode} has no switch in this position.

### Resets

<!-- key: reset -->
Reset {label}

<!-- key: reset-step -->
Reset the step size for {label} to {step}

<!-- key: bank-rst-none -->
Nothing to reset: this model has no parameter at this position

<!-- key: bank-step-none -->
No step size to reset: a setting has positions, not a resolution
