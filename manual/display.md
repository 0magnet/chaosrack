# Display

## Layers · Colors

<!-- key: layers -->
Layers · Colors — how the picture is made up and colored. BEHIND draws a second picture filling the canvas behind the model and SKIN paints one onto its surface; BG is the color behind it all. SRC is what the color follows, MAP how a value becomes a color, PERIOD and SHIFT the window on the map, TRAIL how much of the trail stays lit, and START, MID and END the gradient's own colors. The switches: FILL makes the spectrogram or FVF plane the whole screen face-on, INVERT reverses the gradient, HELD stops the color range refitting itself.

### Backdrop

<!-- key: bg-cell -->
Backdrop — what is drawn behind the model, filling the canvas: the scrolling spectrogram, the XY oscilloscope, a live terminal, or the whole desk. Water is the exception: it is a LENS, drawn through the finished frame rather than behind it — drag on the canvas to make waves. One at a time, or off.

<!-- key: rst-bg-visual -->
Reset the backdrop to off

### Backdrop

<!-- key: bg-visual -->
Backdrop — what is drawn behind the model

- <!-- key: bg-visual=off --> off — nothing behind the model; the background color alone
- <!-- key: bg-visual=spectrogram --> spectrogram — the scrolling spectrogram of the live audio fills the canvas behind the model
- <!-- key: bg-visual=xy --> xy scope — the stereo XY oscilloscope (a goniometer) behind the model
- <!-- key: bg-visual=terminal --> terminal — a live terminal, running a real shell, behind the model
- <!-- key: bg-visual=termanim --> animation — a terminal animation plays behind the model
- <!-- key: bg-visual=desk --> desk — the whole 3-D desktop, windows and all, behind the model
- <!-- key: bg-visual=water --> water (lens) — the exception: water is drawn THROUGH the finished frame rather than behind it; drag on the canvas to make waves

### Skin

<!-- key: skin-cell -->
Skin — what is painted ON the model's own surface, wrapped round it, for the models that have a surface to take it: the live spectrogram, a terminal, or the whole desk. Off leaves the wireframe. The same three pictures the Behind knob draws in the background; this puts one on the object instead.

<!-- key: rst-skin-visual -->
Reset the skin to off

### Skin

<!-- key: skin-visual -->
Skin — what is painted on the model's surface

- <!-- key: skin-visual=off --> off — the model keeps its wireframe; nothing is painted on it
- <!-- key: skin-visual=spectrogram --> spectrogram — the live spectrogram is wrapped round the model's surface
- <!-- key: skin-visual=terminal --> terminal — a live terminal is wrapped round the model's surface
- <!-- key: skin-visual=desk --> desk — the whole 3-D desktop is wrapped round the model's surface

### Background color behind the model

<!-- key: grp-cbg -->
Background color behind the model — swatch or Hue / Level knob

<!-- key: color-bg -->
Background color

<!-- key: rst-color-bg -->
Reset background color

<!-- key: grp-cstart -->
Gradient START color (low end of the source axis) — pick with the swatch or dial it with the Hue (outer) / Level (inner) knob

<!-- key: color-base -->
Gradient start color

<!-- key: rst-color-base -->
Reset start color

<!-- key: grp-cmid -->
Gradient MIDDLE color (3-color palette only) — swatch or Hue / Level knob

<!-- key: color-mid -->
Gradient middle color

<!-- key: rst-color-mid -->
Reset middle color

<!-- key: grp-cend -->
Gradient END color (high end of the source axis) — swatch or Hue / Level knob

<!-- key: color-top -->
Gradient end color

<!-- key: rst-color-top -->
Reset end color

### Source

<!-- key: src-cell -->
Source — what the color follows. OFF is a flat trace in the start swatch: the color follows nothing, which is what used to be called the mono palette and is a statement about the SOURCE rather than about the map. X / Y / Z follow a coordinate of the figure; on the three delay embeddings those axes are one signal at three lags, so they color along different directions of the same figure rather than showing three different quantities. TRAIL follows age along the window. AUDIO paints the short-time spectral CENTROID along the trail — which frequency each moment sits at. LEVEL paints the short-time LEVEL instead, which is the quantity the spectrogram backdrop is painting, so with one of the colormap palettes behind it the figure and the backdrop say the same thing in the same language; AUDIO deliberately does not, and a figure colored by centroid over a spectrogram is not supposed to match it. Only the Takens, stereo and polar embeddings have a trail that is a time axis, so elsewhere both are one flat tint. Displays with a value of their own — the spectrogram, the RTA, the transfer function — do not consult this knob at all, and it dims in those modes.

<!-- key: rst-gradient-source -->
Reset the color source

### Source

<!-- key: gradient-source -->
Source — what the color follows.

- <!-- key: gradient-source=5 --> OFF — a flat trace in the start swatch; the color follows nothing at all
- <!-- key: gradient-source=0 --> X — the color follows the figure's X coordinate
- <!-- key: gradient-source=1 --> Y — the color follows the figure's Y coordinate
- <!-- key: gradient-source=2 --> Z — the color follows the figure's Z coordinate
- <!-- key: gradient-source=3 --> TRAIL — the color follows age along the trail, newest to oldest
- <!-- key: gradient-source=4 --> AUDIO — the color follows the short-time spectral centroid of the sound: which frequency the moment sits at, not how loud it is
- <!-- key: gradient-source=6 --> LEVEL — the color follows the short-time level, which is the quantity the spectrogram backdrop paints. Put a colormap palette behind it (heat, turbo, viridis, magma) and the figure and the backdrop agree: loud is the hot end in both
- <!-- key: gradient-source=11 --> dB — LEVEL on a decibel scale, −60 dB to full scale, FIXED: half way up the ramp is always −30 dB, so two moments can be compared. Linear level spends most of its range on the loudest few percent
- <!-- key: gradient-source=7 --> CORRELATION — how much L and R agree, at each point of the sweep. Fixed scale: the bottom of the ramp is out of phase (what vanishes when summed to mono), the middle uncorrelated, the top identical. A correlation meter says something in here is out of phase; this says WHICH PART
- <!-- key: gradient-source=8 --> SIDE — the level of (L−R)/2, the width of the image moment by moment. Auto-ranged: it answers which parts are wider than which. Turn WIDE and watch it respond
- <!-- key: gradient-source=9 --> BALANCE — which side the energy is on, hard left to hard right, FIXED with centered at the middle of the ramp. Meant for a diverging palette, where an image drifting off center shows as a color drift
- <!-- key: gradient-source=13 --> POSITION — the ANGLE of the (L,R) vector: hue IS the stereo position of the sample rather than a quantity computed about it. On a hue palette the figure's arms take a color per direction
- <!-- key: gradient-source=10 --> FLUX — how much the spectrum CHANGED from the previous slice, rectified so only increases count. Steady tones go flat and transients flash: onsets painted onto the trace
- <!-- key: gradient-source=12 --> PITCH — the centroid folded into one octave, so the same note is the same color in any register. Fixed, and least like the others on a hue palette: a melody becomes a sequence of hues that repeats when it does

### Map

<!-- key: map-cell -->
Map — how a value becomes a color, for every display in the rack including the spectrogram. 2 and 3 mix the Palette module's own swatches; HUE is a raw hue sweep; heat, blue, gray, turbo, viridis and magma are the published colormaps. Turbo and viridis are the even ones — equal steps in the value look like equal steps in color, which a hue sweep does not give you. This is the only colormap control there is: the spectrogram had a second one naming the same six maps, and two knobs that had to be kept in step by hand were one knob too many.

<!-- key: rst-gradient-colors -->
Reset the color map

### Map

<!-- key: gradient-colors -->
Map — how a value becomes a color.

- <!-- key: gradient-colors=2 --> 2-color — a straight mix between the Palette module's start and end swatches
- <!-- key: gradient-colors=3 --> 3-color — the start, middle and end swatches mixed in turn
- <!-- key: gradient-colors=4 --> hue sweep — a raw sweep round the hue circle: vivid, and not perceptually even
- <!-- key: gradient-colors=5 --> heat — black through red and orange to white
- <!-- key: gradient-colors=6 --> blue — the cool counterpart of heat
- <!-- key: gradient-colors=7 --> gray — luminance alone: the map for reading shape rather than value
- <!-- key: gradient-colors=8 --> turbo — an even rainbow; equal steps in the value look like equal steps in color
- <!-- key: gradient-colors=9 --> viridis — even, and readable with color-blindness
- <!-- key: gradient-colors=10 --> magma — even, black through purple and red to cream

### Palette period

<!-- key: grp-rainbow -->
Palette period — how many times the palette is crossed across the trail (low = a narrow slice of it, which the shift knob then sweeps). Drives the rainbow AND the six colormaps: the question is the same one either way.

### Palette period value

<!-- key: slider-value-rfreq -->
Palette period value — type or scroll

### Palette period

<!-- key: rainbow-freq -->
Palette period — palette crossings across the trail

<!-- key: rst-rfreq -->
Reset palette period

### Palette shift

<!-- key: grp-pshift -->
Palette shift — where the window sits on the colormap. Turn the period down to a slice and this sweeps that slice along heat / blue / gray / turbo / viridis / magma; route it from audio in the Mod module and the sound moves the figure THROUGH the map instead of only tinting it. Past either end the map turns back rather than wrapping — turbo's two ends are different colors, and joining them puts a hard seam across the figure — so ±1 is the reversed map, not a dead stop.

### Palette shift value

<!-- key: slider-value-pshift -->
Palette shift value — type or scroll

### Palette shift

<!-- key: palette-shift -->
Palette shift — where the colormap window starts

<!-- key: rst-pshift -->
Reset palette shift

### P-unit buttons in Layers · Colors

The module's switches are buttons on the P-units they concern.

- <!-- key: trio.bg-visual=0 --> F (BEHIND): fill — spectrogram or FVF backdrop full screen, face-on. Applies only to those two backdrops.
- <!-- key: trio.bg-visual=1 --> B (BEHIND): back — the Colors module edits the backdrop's colors instead of the model's. Applies only to a flat backdrop.
- <!-- key: trio.gradient-colors=0 --> I (MAP): invert — reverse the palette direction (start and end swapped)
- <!-- key: trio.gradient-source=0 --> H (SRC): held — the color range stops refitting to the source

### Fill

<!-- key: spect-fill-cell -->
Fill the screen with the spectrogram / FVF display (face-on) instead of the rotatable plane

<!-- key: gradient-reverse-cell -->
Reverse the color-gradient direction (swap start and end)

### Held

<!-- key: color-lock-sw-cell -->
Held — stop the color range refitting itself. Auto stretches the ramp to what is in view, which keeps a quiet passage visible and means the same color says different things a minute apart; held freezes it, so color is comparable between moments. No effect on sources already on a fixed scale (dB, corr, bal, pos, pitch).

### Back

<!-- key: edit-back-cell -->
Back — point the Colors module at the BACKDROP instead of the model: its map, its start, mid and end, its period, each kept apart from the model's. The MODEL knob and the bank show the backdrop's own controls while it is on, and choose among the backdrops. With src OFF a terminal, animation, desk or scope keeps its own colors; any other src redraws it through the map by brightness. Off, the panel is on the model again. Dimmed while nothing is BEHIND.

### Color range

<!-- key: color-lock -->
Color range — auto-ranging or held

- <!-- key: color-lock=0 --> AUTO — the ramp refits itself to what is in view
- <!-- key: color-lock=1 --> HELD — the ramp stays where it is, so color is comparable between moments

## Spectro

<!-- key: spectro -->
Spectro — the spectrogram's own controls, for when it is a LAYER rather than the model: painted on a surface as the skin, or filling the canvas behind the model as the backdrop. As the model its controls are in Parameters, like every other model's; in either of the other two roles Parameters is showing something else, and these had nowhere to be at all.

## Record

<!-- key: record -->
Record — a monitor showing exactly what will be captured, and the transport that captures it. The picture is the recorded area, not the whole canvas, so what you see here is what lands in the file.

### Monitor

<!-- key: rec-preview-cell -->
Monitor — the recorded area, live. Letterboxed, so the shape of the picture is the shape of the region. Timecode and status are burned into the picture the way a field monitor overlays them.

### Monitor

<!-- key: rec-mon-on-cell -->
Monitor — power to this module's screen. The picture is a copy out of the model's own drawing buffer every frame, which is not free; the rack shows every module in a bay now, so a screen you are not watching needs a switch of its own rather than relying on the module being put away. Off, the glass goes dark and the module costs nothing.

### Format

<!-- key: rec-format -->
Format — WebM video (MediaRecorder; small, and plays anywhere a browser does) or animated GIF (256 colors, larger per second, plays anywhere at all). The GIF's palette is built from the clip itself, so gradients do not band.

### Format

<!-- key: rec-gif-sw -->
Format — webm records video; gif records an animated GIF, bigger and with fewer colors, but it plays anywhere an image does.

### Area

<!-- key: rec-area -->
Area — the whole canvas, or a region you drag on it. Switching to region takes the canvas's drag until you have chosen an area, then gives it back; the monitor shows what you picked.

<!-- key: rec-region-sw -->
What gets recorded: the full canvas, or a region of it. On region, drag on the canvas to choose the area — that drag picks the region instead of turning the model, and the camera comes back as soon as you let go. The dashed outline stays to show the choice; switch back to full to clear it.

### Record

<!-- key: rec-btn -->
Record — starts capturing the area on the monitor. Press again, or Stop, to end the take and save the file.

### Stop

<!-- key: rec-stop-btn -->
Stop — ends the take and saves. A GIF is encoded when it stops, which takes a moment for a long clip.

### Still

<!-- key: screenshot-btn -->
Still — saves a PNG of what is on the monitor, at full resolution. Honors the area switch, so with a region set it captures the region and not the whole canvas.

### Media

<!-- key: rec-meter-fill-cell -->
Media — how much of a GIF's frame budget the take has used. A GIF is held in memory until it is encoded, so it has a hard ceiling and stops there on its own; WebM streams to disk as it goes and has none.

### Last take

<!-- key: rec-log-cell -->
Last take — what the previous recording came out as. A GIF is encoded when the take ends, so its size is not known until then; this is where it appears.

## View

<!-- key: view -->
View — orientation: per-axis angle knobs and continuous spin rates

### X axis

<!-- key: knob-x-cell -->
X axis — angle knob, spin rate, horizontal position

### X rotation angle

<!-- key: knob-x -->
X rotation angle — drag or turn to tilt about X

### X rotation angle in degrees

<!-- key: led-x -->
X rotation angle in degrees — drag the model or turn the outer ring to change

### X spin rate

<!-- key: rotation-controls-x -->
X spin rate — continuous rotation about X

### X spin rate value

<!-- key: slider-value-x -->
X spin rate value — type or scroll

<!-- key: rst-rx -->
Reset X angle + spin rate

### Y axis

<!-- key: knob-y-cell -->
Y axis — angle knob, spin rate, vertical position

### Y rotation angle

<!-- key: knob-y -->
Y rotation angle — drag or turn to tilt about Y

### Y rotation angle in degrees

<!-- key: led-y -->
Y rotation angle in degrees — drag the model or turn the outer ring to change

### Y spin rate

<!-- key: rotation-controls-y -->
Y spin rate — continuous rotation about Y

### Y spin rate value

<!-- key: slider-value-y -->
Y spin rate value — type or scroll

<!-- key: rst-ry -->
Reset Y angle + spin rate

### Z axis

<!-- key: knob-z-cell -->
Z axis — angle knob, spin rate, zoom (depth)

### Z rotation angle

<!-- key: knob-z -->
Z rotation angle — drag near the rim to roll about Z

### Z rotation angle in degrees

<!-- key: led-z -->
Z rotation angle in degrees — drag near the rim or turn the outer ring to change

### Z spin rate

<!-- key: rotation-controls-z -->
Z spin rate — continuous rotation about Z

### Z spin rate value

<!-- key: slider-value-z -->
Z spin rate value — type or scroll

<!-- key: rst-rz -->
Reset Z angle + spin rate

### X position

<!-- key: slider-value-panx-cell -->
X position — slides the model sideways. The knob turns once round, its middle the center: half a turn either way is the edge of its travel, and the readout is how far it has turned, in degrees, like the angle knob beside it.

<!-- key: slider-value-panx -->
X position, in degrees of the knob's turn — type or scroll

### X position

<!-- key: pan-x -->
X position — slide the model horizontally

<!-- key: rst-panx -->
Reset X position

### Y position

<!-- key: slider-value-pany-cell -->
Y position — slides the model up and down. The knob turns once round, its middle the center: half a turn either way is the edge of its travel, and the readout is how far it has turned, in degrees, like the angle knob beside it.

<!-- key: slider-value-pany -->
Y position, in degrees of the knob's turn — type or scroll

### Y position

<!-- key: pan-y -->
Y position — slide the model vertically

<!-- key: rst-pany -->
Reset Y position

<!-- key: slider-value-zoom-cell -->
Zoom, and Fore nested on top of it. ZOOM is the camera's distance, on a knob that turns once round like the position knobs, its readout in degrees of that turn. FORE is where the model sits relative to the rack, as a moving partition rather than a switch. Fully back and all of it is behind the controls; fully forward and all of it is in front; anywhere between and the panel cuts through it, near half in front and far half behind. The cut is a plane parallel to the screen, so it stays put as you turn the model. A flat picture — the spectrogram, the terminal — has no depth to cut, so it moves whole. The reset puts both back.

### Zoom, in degrees of the knob's turn

<!-- key: slider-value-zoom -->
Zoom, in degrees of the knob's turn — type or scroll

### Zoom

<!-- key: camera-zoom -->
Zoom — camera distance to the model

### Fore

<!-- key: slider-value-fore -->
Fore — -1 is entirely behind the rack, +1 entirely in front, 0 cuts through the middle

### Fore

<!-- key: model-fore -->
Fore — slide the model through the rack

<!-- key: rst-zoom -->
Reset zoom and Fore

## Grid

<!-- key: grid -->
Grid — how many views of the model are drawn, what differs between them, and which of them the knobs are pointed at. It sits in the DISPLAY bay beside View and Position because it is a question about the picture, not about the signal: Trace is how one trajectory is drawn, View is where the camera is, and this is how many cameras there are.

### Grid

<!-- key: view-n-stack-cell -->
Grid — how many views of the model are drawn, as a square (2 is side by side). A grid is not that many instruments configured by hand: pick what varies with the Sweep dial and the cells draw that parameter's range, which is a contact sheet of one setting rather than sixteen panels to fill in.

- <!-- key: view-n=0 --> 1 — a single view
- <!-- key: view-n=1 --> 2 — side by side
- <!-- key: view-n=2 --> 4 — two by two
- <!-- key: view-n=3 --> 9 — three by three
- <!-- key: view-n=4 --> 16 — four by four

### Grid buttons

- <!-- key: trio.view-n=0 --> T: tile — the cells side by side, each in its own part of the screen (lit while tiled)
- <!-- key: trio.view-n=1 --> O: overlay — every cell drawn over the whole screen, one on top of another; each cell after the first in a color of its own, unless the sweep is of the coloring

### Overlay

<!-- key: grid-ovl -->
Overlay — the grid's cells drawn one over another on the whole screen instead of side by side. Cells after the first are drawn each in a single color of its own (the second green, then further hues) so they can be told apart, except under a csrc or cmap sweep, where the coloring is what differs. With a start sweep of two cells this is the twin-trajectory display: the model and a copy started ε away, separating at the attractor's own rate. The O button on GRID.

### Sweep

<!-- key: sweep-p-stack-cell -->
Sweep — which parameter varies across the grid, and the from and to knobs beside this one say over how much of its range. This is what makes a grid of sixteen usable: rather than sixteen panels to fill in, one dial says what differs and the cells draw that parameter's space, like a contact sheet. The list is the CURRENT model's own parameters, so it changes with the mode; a model that integrates its trajectory offers no parameter sweep, because the cells share one integrator and a swept grid would smear one trajectory across them. A flow offers start instead: each cell integrates its own trajectory, started ε from the model's own. csrc and cmap sweep the COLORING instead of a number — the same figure read nine different ways, or in nine palettes — which is the case a grid is most worth having for.

- <!-- key: sweep-p=0 --> none — every cell the same

### Sweep buttons

The same three on Sweep down.

- <!-- key: trio.sweep-p=0 --> the next parameter in the list
- <!-- key: trio.sweep-p=1 --> none — stop sweeping in this direction
- <!-- key: trio.sweep-p=2 --> the parameter before it in the list

### Sweep down

<!-- key: sweep2-p-stack-cell -->
Sweep down — a SECOND parameter, varying down the grid while the first varies across it. This is what the second direction of a grid is for: with one sweep a three by three of nine delays wasted its rows, and asking for nine palettes meant giving up the nine delays. Set both and each column is one value of across, each row one value of down, and the cell where they meet is the pair — nine delays read in three palettes, say. Set only this one and it behaves exactly like the sweep beside it. The two cannot name the same parameter: rows and columns varying the same thing is not a comparison.

- <!-- key: sweep2-p=0 --> none — the grid varies in one direction only

### Sweep from

<!-- key: sweep-lo-cell -->
Sweep from — where in the swept parameter's own range the first cell sits, as a fraction: 0 is the parameter's minimum. Turn both this and to inward and the contact sheet covers a slice of the range in detail rather than all of it coarsely, which is how a sweep is used once you know roughly where the interesting part is. For a start sweep the fraction sets ε: 0 is ε = 0, the model's own trajectory, and above 0 the scale is logarithmic, 10⁻⁹ to 10⁻¹.

### Sweep start value

<!-- key: slider-value-swlo -->
Sweep start value — type or scroll

<!-- key: sweep-lo -->
Sweep start, as a fraction of the parameter's range

<!-- key: rst-swlo -->
Reset sweep start

### Sweep to

<!-- key: sweep-hi-cell -->
Sweep to — where in the swept parameter's own range the last cell sits, as a fraction: 1 is the parameter's maximum. Below the from knob and the sweep runs backwards, which is worth having when the interesting end should be read first.

### Sweep end value

<!-- key: slider-value-swhi -->
Sweep end value — type or scroll

<!-- key: sweep-hi -->
Sweep end, as a fraction of the parameter's range

<!-- key: rst-swhi -->
Reset sweep end

### Focus

<!-- key: focus-n-cell -->
Focus — which cell of the grid the knobs drive while Link is off. Its positions follow the grid, so a sixteen-cell sheet has sixteen of them. Moving it rebuilds the parameter rows against that cell's own instance, which is what makes a knob write to the view you are looking at rather than to the one behind it.

- <!-- key: focus-n=0 --> A — the first cell

### Link

<!-- key: link-sw-cell -->
Link — every cell of the grid on one set of knobs: turn one and every view follows, which is what to use for comparing colorings or camera angles of one setting. It is the 0 button beside Focus. Off, each cell keeps its own parameters and the knobs drive the one Focus is on — which is what to use for comparing settings of the instrument.

### Focus buttons

- <!-- key: trio.focus-n=0 --> the next cell — unlinks, and the knobs drive that cell alone
- <!-- key: trio.focus-n=1 --> Link — every cell on one set of knobs (lit while linked)
- <!-- key: trio.focus-n=2 --> the cell before — unlinks, and the knobs drive that cell alone

## Display

<!-- key: display -->
Display — trace rendering: line width, point decimation, trail length and mode, and the overlays drawn on the trace (Poincaré section, graticule). Three P-units; the switches are on their buttons. The twin trajectory is now the Grid's start sweep with Overlay on.

### LINE

<!-- key: line-cell -->
LINE — trace line width, 1 to 10 pixels. Buttons: S Poincaré section, G graticule; each is an independent switch.

<!-- key: slider-value-line -->
Line width value — type or scroll

<!-- key: line-width -->
Line width — trace thickness in pixels

<!-- key: rst-line -->
Reset line width

### LINE buttons

- <!-- key: trio.line-width=0 --> S: Poincaré section overlay
- <!-- key: trio.line-width=1 --> G: graticule behind a scope trace

### POINTS

<!-- key: points-cell -->
POINTS — point decimation. At 0 the trace is a solid line. Any other setting draws the trace as that many points, evenly spaced along it; a setting at or above the trail's vertex count is equivalent to a solid line. Useful where the figure folds over itself and a solid line obscures its structure.

<!-- key: slider-value-dash -->
Point count — type or scroll; 0 is the solid line

<!-- key: dash-duty -->
Point count — the number of points the trace is drawn as; 0 draws it solid

<!-- key: rst-dash -->
Reset point count (0, solid line)

### POINTS buttons

- <!-- key: trio.dash-duty=0 --> dots: draw every vertex as a point
- <!-- key: trio.dash-duty=1 --> line: solid line (dots off, point count 0)

### Dots

<!-- key: use-points -->
Dots — draw every vertex of the trace as a point instead of a connected line

### TRAIL

<!-- key: trail-controls -->
TRAIL — trail length, in vertices, and trail mode. The buttons select the mode: persist, scan or ring.

<!-- key: slider-value-trail -->
Trail length value (vertices) — type or scroll

<!-- key: trail-slider -->
Trail length — number of most recent vertices drawn

<!-- key: rst-trail -->
Reset trail length

### TRAIL buttons

- <!-- key: trio.trail-slider=0 --> persist: the frame is not cleared; the trace accumulates until the mode is changed
- <!-- key: trio.trail-slider=1 --> scan: the whole trail is recomputed and redrawn each frame (default)
- <!-- key: trio.trail-slider=2 --> ring: only the trail head is integrated each frame and the trail is its history, so parameter changes take effect from the head onward

### Persist

<!-- key: persist-trail-cell -->
Persist — the frame is not cleared; the trace accumulates. TRAIL's + button.

### Ring

<!-- key: ring-sw-cell -->
Ring — beam mode: only the advancing head is integrated each frame and the trail is its history. Parameter and audio changes bend the path from the head forward instead of reshaping the whole curve, and long trails cost little. TRAIL's − button.

### Poincaré section

<!-- key: sect-sw-cell -->
Poincaré section — samples the trajectory where it crosses a plane in one direction and draws the accumulated intersections in gold. Each crossing is interpolated between the two samples that straddle the plane. Enabling it reveals the Section module, which sets the plane and direction. Analysis → Poincaré Section shows the same section as a separate display, with the first-return map.

### Graticule

<!-- key: scope-grat-cell -->
Graticule — the scale behind a scope trace: 8 × 10 divisions, heavier center axes, minor ticks at fifths of a division, and the 0/10/90/100 % rise-time markers. On by default. Drawn only for the scope models and for any model using the CRT color source.

### Grid SWEEP positions

- <!-- key: grid-sweep=none --> none — every cell the same
- <!-- key: grid-sweep=src --> color source — a different reading of the same figure per cell
- <!-- key: grid-sweep=map --> color map — the same reading in a different palette per cell
- <!-- key: grid-sweep=start --> start — each cell a trajectory of its own, started ε from the model's (from and to set ε); flows only

### Swept mark

<!-- key: grid-swept -->
Swept {dir} — this parameter's value comes from where each cell sits in the grid, not from this knob. The from and to knobs beside the Sweep dials say which part of its range the cells cover.

### Link mark

<!-- key: grid-linked -->
Linked — every cell uses view A's setting of this control, even though the views are unlinked. Click to give each cell its own again.

<!-- key: grid-unlinked -->
Unlinked — each cell has its own setting of this control. Click to pin every cell to view A's, so this one knob drives them all while the rest stay independent.

### Hue

<!-- key: hue-knob -->
Hue — turn all the way around the spectrum

### Color knobs

Every color on the panel is set by a stacked pair: Hue round the outside,
Level inside it. `{name}` is the color the knob sets.

<!-- key: color-knob -->
{name} knob — outer ring = Hue (rainbow scale), inner ring = Level (black → color → white)

<!-- key: color-knob.hue -->
{name} — Hue: turn all the way around the spectrum

<!-- key: color-knob.level -->
{name} — Level (black → color → white)

### Every knob's printed scale

<!-- key: knob-quarter -->
{deg}° — a quarter-turn mark on the angle scale

<!-- key: knob-min -->
{v} — the lowest this knob goes; turned fully counter-clockwise

<!-- key: knob-max -->
{v} — the highest this knob goes; turned fully clockwise

### Fine trim

<!-- key: knob-fine -->
Fine trim — {control}

<!-- key: knob-fine.plain -->
fine trim

### Endless knobs

<!-- key: knob-endless -->
An endless knob: {what}. Each turn lights the ring in the next LED color, sweeping over the last.

## Lattice

<!-- key: lattice -->
Lattice — any model drawn in depth by terminals. Each sheet of the stack is a real terminal running \[lattice\](https://github.com/0magnet/lattice), and each draws its own slice of the volume: with the Lattice model, its solid; with L lit on ROWS, the model chosen, whatever it is. The model's trail, mesh or dots are the figure every program is handed, and a voxel is lit where the figure passes through it, written as the way the figure runs across the sheet. The model keeps its own knobs in the bank, so it is played as ever; these six are how the lattice draws it. Pictures and terminals, which are flat already, are not drawn in it.

### ROWS

<!-- key: lattice-n-cell -->
ROWS — each terminal's height in rows, which is its resolution: the programs read their terminal's size, as programs do, and draw that many voxels across. A family has as many sheets as a terminal has rows, so this is the volume's resolution in all three dimensions: more rows is a terminal zoomed out to smaller characters, and a finer volume. 3 × rows terminals of 2 × rows columns; finer costs more. Button: L draws the model chosen in the lattice.

<!-- key: slider-value-latrows -->
Rows — type or scroll

<!-- key: lattice-n -->
Rows — each terminal's height, and the volume's resolution

<!-- key: rst-lattice-n -->
Reset rows (12)

### ROWS button

- <!-- key: trio.lattice-n=0 --> L: the model chosen, drawn by the lattice instead of on its own

<!-- key: lat-on -->
L — the model chosen, drawn by the lattice

### STYLE

<!-- key: lattice-style-cell -->
STYLE — how each terminal writes what crosses its sheet.

<!-- key: lattice-style -->
Style — how each terminal writes what crosses its sheet

- <!-- key: lattice-style=0 --> lines — the line a surface makes across the sheet, or the way a figure runs across it, ─ │ ╱ ╲, and · where it lies along the sheet or runs through it
- <!-- key: lattice-style=1 --> ascii — the same in - | / \ and .
- <!-- key: lattice-style=2 --> shade — how squarely it crosses the sheet, .:-=+*# from lying in it to standing across it
- <!-- key: lattice-style=3 --> solid — every voxel it passes through, filled

<!-- key: rst-lattice-style -->
Reset style (lines)

### LOOK

<!-- key: lattice-look-cell -->
LOOK — how the sheets are put together on screen.

<!-- key: lattice-look -->
Look — how the sheets are put together on screen

- <!-- key: lattice-look=0 --> glass — as a desktop composites translucent terminal windows: each sheet's default background at OPAC, its text and any background color a program sets opaque, every piece laid over what is behind it, back to front
- <!-- key: lattice-look=1 --> light — a volumetric display rather than terminals: no backgrounds, each character added to what is behind it as light, and each family of sheets faded as it turns edge on

<!-- key: rst-lattice-look -->
Reset look (glass)

### OPAC

<!-- key: lattice-opac-cell -->
OPAC — every terminal's background opacity, in percent: the setting a translucent terminal emulator has (kitty's background_opacity, alacritty's window.opacity), never something a program asks for. It applies to the default background, which is black; text stays opaque. At 0 only the text is there; higher, each sheet darkens what is behind it, so the far side is dimmer than the near.

<!-- key: slider-value-latopac -->
Opacity — type or scroll, in percent

<!-- key: lattice-opac -->
Opacity — every terminal's background, in percent

<!-- key: rst-lattice-opac -->
Reset opacity (10%)

### STACKS

<!-- key: lattice-stacks-cell -->
STACKS — which families of sheets there are. Fewer families are fewer terminals and less to draw: a family that is not shown does not run its programs either.

<!-- key: lattice-stacks -->
Stacks — which families of sheets there are

- <!-- key: lattice-stacks=0 --> xyz — all three, so the volume has depth from every side
- <!-- key: lattice-stacks=1 --> auto — all three while they face you, a family dropped as it turns within about 12° of edge on, where it shows next to nothing; the family facing you most is always kept
- <!-- key: lattice-stacks=2 --> xz — the two upright families: seen from above or below, you look along their edges
- <!-- key: lattice-stacks=3 --> z — the front sheets alone: a stack of terminals, seen best from the front or behind

<!-- key: rst-lattice-stacks -->
Reset stacks (xyz)

### TURN

<!-- key: lattice-turn-cell -->
TURN — what turning the view turns. The programs are told the way any program is told anything, on their input: each is sent a pose, the view's turn or its inverse, which lattice applies to what it draws.

<!-- key: lattice-turn -->
Turn — what turning the view turns

- <!-- key: lattice-turn=0 --> both — the stack and what is in it turn together
- <!-- key: lattice-turn=1 --> lattice — the stack turns and what is in it holds still: it moves through the grid as the grid turns round it, every terminal redrawing as it goes
- <!-- key: lattice-turn=2 --> solid — the stack stays square to the screen and what is in it turns inside

<!-- key: rst-lattice-turn -->
Reset turn (both)
