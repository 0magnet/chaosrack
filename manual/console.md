# Console

## Console

<!-- key: console -->
Console — model selection, global actions, and every mode/effect switch in one module

<!-- key: console.equation -->
Equation editing and global panel actions. The model itself is chosen on its own category's row — see the rotary at the head of each model bay — rather than from a knob up here that changed what the whole instrument was.

<!-- key: reset-all-btn -->
Reset parameters, colors, view pose, trail, gradient, effect switches (Front/Fill/audio/backdrops), display style, and the signal generator to their defaults (keeps your dock layout + interface size)

<!-- key: normalize-btn -->
Re-center the model to the default (identity) orientation and stop any spin

### Audio

<!-- key: console.audio -->
Audio — the subsystems that are switched on rather than patched in: hardware control over the rack (MIDI). Where every sound goes is the Mixer's, in the routing bay.

### WebMIDI

<!-- key: midi-sw-cell -->
WebMIDI — hardware control: CC 1..N drive the current mode's parameter knobs in order, CC 21..28 the view targets (zoom, pans, spins, rainbow, trail), and any note hops to that note's attractor.

### Solo

<!-- key: gen-solo -->
Solo — the one generator on the rack's signal and the speakers, the others kept as they are and silenced

### Rack

<!-- key: console.rack -->
Rack — the frame itself and the window it is drawn in, rather than anything in the signal path. Nothing here has a knob elsewhere to be beside, which is why it is the one group of switches that stays central.

### Power

<!-- key: power-sw-cell -->
Power — the render loop. Off stops it and clears the canvas, which is what the GPU costs; the panel stays, so every setting is still there to read and to change. A switch again, and back in the Rack group where the rest of the frame's own controls are: it had been folded into the model knob's first detent, which made powering down look like choosing a model called OFF and put the rack's power switch inside the one control that had nothing to do with the rack.

### Rack bay

<!-- key: handles-on-cell -->
Rack bay — draw the 19-inch frame each ROW of modules sits in: rails above and below, an ear each side, and blank panels filling the leftover at the end of the row on the same slot pitch. The modules wrap, and a row is a bay, so a rack that has spilled onto a second row gets a second frame. Handles and panel screws are chosen on the Style module: the inner ring of Size.

<!-- key: show-info-cell -->
Overlay a short description of the current attractor / model on the view

### Toggle browser fullscreen

<!-- key: fullscreen-sw-cell -->
Toggle browser fullscreen — the canvas fills the display; the panel stays available

### Desk

<!-- key: desk-contain-cell -->
Desk — run this app inside a desktop: the desk becomes the environment, its panel takes the bottom of the screen, and the rack floats as a window on it with a task button beside the desk's own. The scene keeps running behind. Open a terminal, the host shell or the file manager from the desk's Applications menu. The OTHER way to have the desk is as a MODEL (Solids → desk), where the whole desktop is drawn into the scene and turns with it.

### Motion

<!-- key: console.motion -->
Motion — whether the model is moving, and what is moving it: the Y spin, a freeze, the physics that gives the figure weight, and Jam, which plays the rack by itself.

<!-- key: auto-rotate-cell -->
Continuously spin the model about the vertical (Y) axis — it folds into the Y spin rate (ω, View)

<!-- key: pause-sw-cell -->
Pause / freeze the animation

### Physics

<!-- key: phys-sw-wrap -->
Physics — give the turtle path weight: it becomes a rigid body in the plane of the screen, inside a room whose walls are the edges of the picture. Reveals the Physics module. Press on the figure to pick it up and throw it; press beside it to turn the view as usual.

### Jam / attract mode

<!-- key: jam-sw-cell -->
Jam / attract mode — the app performs itself: hops to a random attractor every 12–20 s with fresh gentle spin and occasional persist paint. Never touches speaker outputs.

### Lyapunov

<!-- key: mro-* -->
Lyapunov — the largest exponent of the model on screen: how fast two nearby trajectories separate. Positive means chaotic (prediction has a horizon); about zero means periodic or quasi-periodic; negative means the orbit is settling. Flows are per unit time, maps per iterate. Measured on demand, not per frame.

<!-- key: lyap-led -->
Largest Lyapunov exponent. /t = per unit time (flows), /n = per iterate (maps).

<!-- key: lyap-verdict -->
Plain reading of the exponent: chaotic, periodic, settling (converging) or diverged

<!-- key: lyap-remeasure -->
Re-measure now. A run is a few hundred thousand integration steps, so it is done on demand rather than every frame.

### Banner

<!-- key: mro-scopetext -->
Banner — what Fourier Text's beam writes (A–Z, 0–9, dash, space). The harm knob in the bank sets how many harmonics each glyph keeps.

### Banner text

<!-- key: stext-in -->
Banner text — what the beam writes; melt it with the harm knob

### Score

<!-- key: mro-pong -->
Score — each player's points; first past 9 resets the match. The left and right knobs in the bank are the paddle pots.

<!-- key: pong-score-l -->
Left player's score (W/S, left-half touch, or the left pot)

<!-- key: pong-score-r -->
Right player's score (↑/↓, right-half touch, or the right pot)

### Wired

<!-- key: mro-sprottmorph -->
Wired — which two catalog systems the self-programming machine is blended between right now

### Live patch

<!-- key: smorph-led -->
Live patch — the current catalog blend, e.g. D-E 42% (100% = fully the next system); the sys knob parks it, the rate knob self-steps

### Kicks

<!-- key: mro-bounceball -->
Kicks — the machine's re-kicks since the mode started

### Machine re-kicks since the mode started

<!-- key: bounce-kicks -->
Machine re-kicks since the mode started — each is a decayed ball re-launched

### File

<!-- key: mro-stlfile -->
File — the stereolithograph on screen: load one with the load switch, or turn the solid knob in the bank to a built-in one

### Loaded STL

<!-- key: stlfile-led -->
Loaded STL — file name and triangle count (a stand-in cube shows until a file is loaded)

### Desktop style

<!-- key: desk-style-cell -->
Desktop style — four 3-D desktops reproduced over the scene. Flat: ordinary windows. Looking Glass (Sun, 2003): windows lean back in a legible stack; double-click a title bar to turn one over and read its back. Cube (Compiz, 2006): four workspaces on the faces of a cube; the arrow keys spin it. Metisse (2004): shift-drag a title bar to turn a window freely, and it stays live while turned. BumpTop (2009): windows have weight and fall into a pile on top of the rack.

### Desktop style

<!-- key: desk-style -->
Desktop style — how the windows are drawn

<!-- key: rst-desk-style -->
Reset the desktop style

### Program

<!-- key: termanim-cell -->
Program — which terminal program runs on the quad. Twenty-one animations plus the text demos, from tuiwasm; the list is built from what is registered, so it cannot drift from what is actually there.

### Program

<!-- key: termanim-pick -->
Program — which terminal program runs on the quad

### SWEEP

<!-- key: bif-sweep-cell -->
SWEEP — which parameter of the system being swept is the x axis of the diagram; each column integrates the system fresh at that value and plots the maxima of z. The system is the most recent flow mode.

### Swept parameter

<!-- key: bif-sweep -->
Swept parameter — the x axis of the diagram

### DRIVE

<!-- key: bif-drive-cell -->
DRIVE — what puts the system at a parameter value. sweep is the diagram alone, computed left to right. audio keeps the same diagram and points a cursor at it from the live audio envelope, so the branch structure under the music is lit up as it plays. The diagram itself does not move: its x axis means something only because the parameter runs monotonically along it. Needs Audio mod on, which is what computes the envelope.

<!-- key: bif-drive -->
sweep: the diagram alone. audio: the envelope moves a cursor along it.

<!-- key: rst-bif-drive -->
Reset drive (sweep)

### Wave

<!-- key: fvf-wave-cell -->
Wave — the carrier waveform: square, pulse, or a sub-octave square (divided by two)

### Wave

<!-- key: fvf-wave -->
Wave — carrier waveform

<!-- key: rst-fvf-wave -->
Reset wave (pulse)

### Mod

<!-- key: fvf-mod-cell -->
Mod — the modulator topology: ring (four-quadrant) or AM (balanced)

### Mod

<!-- key: fvf-mod -->
Mod — modulator topology

<!-- key: rst-fvf-mod -->
Reset mod (ring)

### Built-in solids

<!-- key: stlfile-builtin-cell -->
Built-in solids — the rack modules and the 19-inch frame at their real dimensions, the geometry as closed solids, and every attractor swept as a tube along its own trajectory. Generated in the browser; the same models are written as .stl files by cmd/stlgen.

### Built-in solid

<!-- key: stlfile-builtin -->
Built-in solid — or a file loaded from disk

- <!-- key: stlfile-builtin=off --> file — the STL loaded with the load switch (a stand-in cube until one is)

### Pass-through

<!-- key: desk-pass -->
Pass-through — where the mouse goes while the Desk MODEL is on screen. Off: dragging turns the model, and Ctrl-drag reaches the desk. On: dragging reaches the desk — moving windows, resizing them, pressing their buttons — and Ctrl-drag turns the model. Control always inverts the switch, so both gestures are available either way. The keyboard needs neither: double-click to type into the focused window, Esc to give it back.

### Restart the match

<!-- key: pong-restart -->
Restart the match — zero both scores and serve fresh

<!-- key: bounce-drop -->
Drop the ball again from the height knob's setting with a fresh drift

<!-- key: stlfile-load -->
Load an .stl file from disk (binary or ASCII)

<!-- key: dock-resize -->
Drag to resize the control panel

<!-- key: dock-controls -->
Dock the controls to an edge of the window, or detach as a floating panel

<!-- key: dock-top -->
Dock the panel to the top edge

<!-- key: dock-bottom -->
Dock the panel to the bottom edge

<!-- key: dock-left -->
Dock the panel as a left sidebar

<!-- key: dock-right -->
Dock the panel as a right sidebar

<!-- key: dock-float -->
Detach the panel as a floating window

<!-- key: dock-footer -->
Dock the panel inline into the host page's footer, below its own content

## Style

<!-- key: style -->
Style — knob face + LED color, and CRT phosphor

### Knob

<!-- key: knobstyle-stack-cell -->
Knob — the face every knob in the rack wears (std / flat / vint / chrome / gold / carbon).

<!-- key: rst-knob-style -->
Reset the knob style

### Knob appearance

<!-- key: knob-style -->
Knob style — knob face appearance (std / flat / vint / chrome / gold / carbon)

- <!-- key: knob-style=std --> std — the standard knob face
- <!-- key: knob-style=flat --> flat — a matte face with no highlight
- <!-- key: knob-style=vint --> vint — a vintage cream face
- <!-- key: knob-style=chrome --> chrome — a polished metal face
- <!-- key: knob-style=gold --> gold — a brushed gold face
- <!-- key: knob-style=carbon --> carbon — a carbon-fiber face

### LED

<!-- key: ledcolor-cell -->
LED — the color of every readout in the rack. Each dot round the knob is one color, in itself; the display under it names the one chosen.

<!-- key: rst-led-color -->
Reset the LED color

### LED color

<!-- key: led-color -->
LED color — readout color for every numeric LED readout

<!-- key: phosphor-cell -->
CRT phosphor for scope traces (Lissajous / Graphic Artist) — sets trace color + afterglow. P31 crisp green … P7 blue→green … P33 long amber.

<!-- key: rst-phosphor -->
Reset the phosphor to off

### Phosphor

<!-- key: phosphor -->
Phosphor — CRT trace color + afterglow for scope modes (P31 crisp green … P7 blue→green … P33 long amber)

## Presets

<!-- key: preset -->
Presets — the current view, saved under a name. A preset holds everything the permalink holds: the model, every knob and color, the effect switches, the parameters and the pose. Recalling one resets to defaults and re-applies it, exactly as the Patchbay's numbered patch memories do; the difference is that these have names, and the address bar does not have to carry them.

### Preset name

<!-- key: preset-name -->
Preset name — what to file the current view under. Saving over a name that already exists replaces it; leaving this empty files the view under the current model's name.

<!-- key: preset-save -->
Save the current view under the name beside it — the same complete state a permalink carries

<!-- key: preset-list -->
Presets saved in this browser

### Recall the selected preset

<!-- key: preset-recall -->
Recall the selected preset — resets to defaults and re-applies the saved view, and updates the address bar so it is immediately shareable

<!-- key: preset-del -->
Delete the selected preset

### Phosphor positions

- <!-- key: phosphor=0 --> no phosphor — the trace keeps the palette's own colors and CRT mode is off
- <!-- key: phosphor=1 --> P31 — the Tektronix standard: bright green, short to medium persistence
- <!-- key: phosphor=2 --> P1 — willemite yellow-green, medium persistence (about 24 ms)
- <!-- key: phosphor=3 --> P2 — yellow-green, long persistence
- <!-- key: phosphor=4 --> P3 — yellow-amber, medium persistence: the classic oscilloscope tube
- <!-- key: phosphor=5 --> P4 — television white, short persistence
- <!-- key: phosphor=6 --> P11 — photographic blue, short persistence: the tube built to expose film
- <!-- key: phosphor=7 --> P7 — two layers: a blue flash that dies fast over a green afterglow that lingers
- <!-- key: phosphor=8 --> P39 — long-persistence green, about 150 ms
- <!-- key: phosphor=9 --> P33 — radar amber: the longest persistence of the set

### LED color positions

- <!-- key: led-color=red --> red — the nixie-adjacent readout: highest contrast on black
- <!-- key: led-color=amber --> amber — the classic seven-segment LED, and the easiest on the eye
- <!-- key: led-color=green --> green — the VFD and early-terminal readout
- <!-- key: led-color=blue --> blue — bright and cold; the newest of the six as a real display
- <!-- key: led-color=cyan --> cyan — the hue the audio-mod readouts used before the LED color was a knob
- <!-- key: led-color=violet --> violet — no display was ever made in it; it is here because it reads well

### Desktop style positions

- <!-- key: desk-style=flat --> Flat — ordinary windows, lying in the plane of the screen
- <!-- key: desk-style=glass --> Looking Glass (Sun, 2003) — windows lean back in a legible stack; double-click a title bar to turn one over and read its back
- <!-- key: desk-style=cube --> Cube (Compiz, 2006) — four workspaces on the faces of a cube; the arrow keys spin it
- <!-- key: desk-style=metisse --> Metisse (2004) — shift-drag a title bar to turn a window freely, and it stays live while turned
- <!-- key: desk-style=bump --> BumpTop (2009) — windows have weight and fall into a pile on top of the rack

### Patch memories

<!-- key: patch-bank-cell -->
Patch memories — eight numbered snapshots of the whole rack. STO, then a number, stores; a number alone recalls.

<!-- key: patch-sto -->
Store mode — press STO, then a slot, to save the current patch there. Plain slot click recalls.

<!-- key: patch-slot -->
Patch memory {n} — click to recall; STO first to store the current patch

### DOCK

<!-- key: dock-label -->
DOCK — drag this label to resize the panel; the arrow buttons choose the dock edge or floating mode

### Show / hide

<!-- key: panel-toggle -->
Show / hide controls (brings them back if the model's 'Front' overlay is hiding them)

## The manual page

The rack's own manual, at /manual: every bay, every module as it stands in the
rack and working, and every control by address.

<!-- key: manual-pop-bay -->
Take this bay out into a window of its own, to keep in view while you read on; closing it puts the bay back here

<!-- key: manual-away -->
This bay is in a window.

<!-- key: manual-model -->
The model, in a window of its own: opening it switches the rack on, and closing it switches the rack off, as the Console's Power switch does
