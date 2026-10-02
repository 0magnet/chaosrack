# Routing

Where every signal on the rack goes. Two modules share the routing bay: the
**Mixer**, which says where each *sound* goes, and the **Mod** matrix, which
says which *knobs* the rack's signal turns. The Mixer makes the signal the Mod
matrix reads, so they sit side by side.

The signal flow — what every source can be pinned to, and what reads each
row — is drawn at the front of the manual: [README](README.md#signal-flow).

## Mixer

<!-- key: mixer -->
Mixer — every sound on the rack, and where it goes. Each column is a source (the capture, the generators, the model's x, y and z, the instruments); each row is a destination: SPK L and R are the speakers, RACK L and R the rack's own signal, which the meters, the audio models, the scopes and the Mod matrix read, and MOD A and B two sends only the Mod matrix reads, for a source that should turn a knob without being heard or measured. A pin where they cross puts the source there: press it to pin at full level, press again to take it away, and turn the wheel over it, or LVL below, to set its gain; a negative gain inverts it. Several pins on a row are mixed. The capture has no speaker pins: it is the system's own audio, already playing.

### The axes

- <!-- key: mx-from --> FROM — the columns: where a signal comes from
- <!-- key: mx-to --> TO — the rows: where it goes

### The matrix

<!-- key: mixer-matrix -->
The pins: a column for each source, a row for each place it can go. Press a pin to put the source there at full level, press it again to take it away, turn the wheel over it to set its gain. A dashed pin is one that cannot be made.

### The columns

- <!-- key: mix-group=cap --> CAP — the capture: what the system is playing (with chaosrack --audio), or the microphone. Measured, never played: it has no speaker pins.
- <!-- key: mix-group=gen --> GEN — the four generators, as their knobs set them.
- <!-- key: mix-group=model --> MODEL — Model Out: the running model's x, y and z, its own equations run at audio rate. The model's SPD and LVL are at the top of its bank's last column.
- <!-- key: mix-group=inst --> INST — the instruments: the keys, the drums, the tone matrix, and the sounds a model makes itself (Pong's and Bounce's blips, the FVF wobbulator's Listen).

- <!-- key: mix-src=cl --> CAP L — the capture's left channel
- <!-- key: mix-src=cr --> CAP R — the capture's right channel
- <!-- key: mix-src=g1 --> GEN 1 — the first generator
- <!-- key: mix-src=g2 --> GEN 2 — the second generator
- <!-- key: mix-src=g3 --> GEN 3 — the third generator
- <!-- key: mix-src=g4 --> GEN 4 — the fourth generator
- <!-- key: mix-src=mx --> MODEL X — the model's x, from Model Out
- <!-- key: mix-src=my --> MODEL Y — the model's y, from Model Out
- <!-- key: mix-src=mz --> MODEL Z — the model's z, from Model Out
- <!-- key: mix-src=ky --> KEYS — the keyboard, as the Synth bay voices it
- <!-- key: mix-src=dr --> DRUMS — the Matrix's rhythm section
- <!-- key: mix-src=tm --> MATRIX — the tone matrix's pings
- <!-- key: mix-src=fx --> SOUNDS — the running model's own sounds: Pong, Bounce, the FVF wobbulator

### The rows

- <!-- key: mix-row=sl --> SPK L — the left speaker
- <!-- key: mix-row=sr --> SPK R — the right speaker
- <!-- key: mix-row=rl --> RACK L — the left of the rack's signal: what the meters, the audio models, the scopes' RACK L and the Mod matrix's L read
- <!-- key: mix-row=rr --> RACK R — the right of the rack's signal
- <!-- key: mix-row=ma --> MOD A — a send to the Mod matrix's A column and nothing else: a source here turns knobs without being heard and without the meters seeing it
- <!-- key: mix-row=mb --> MOD B — the Mod matrix's B column, as MOD A is its A

### A pin

`{src}` and `{row}` are the column and the row it joins; `{gain}` is its gain.

<!-- key: mixer-pin -->
{src} → {row}, gain {gain} — press to pin it at full level or take it away; the wheel sets the gain, and below 0 inverts it

<!-- key: mixer-pin.none -->
{src} cannot go to {row}: the capture is the system's own audio, already on the speakers, and playing it back would feed it to itself

### PIN

<!-- key: mixer-sel -->
PIN — the pin LVL is setting: the last one pressed or turned, as source>row

### LVL

<!-- key: mixer-lvl -->
LVL — the selected pin's gain, −1 to 1: 1 is the source as it is, 0 takes the pin away, and below 0 inverts it — GEN 1 at 1 on RACK L and −1 on RACK R is L−R, and nothing in a mono sum

### Generators on the Mixer

`{gen}` is the generator's number.

<!-- key: gen-autopin -->
Gen {gen} is pinned to the Mixer: one side of the speakers and of the rack's signal. The Mixer moves it, or takes it away.

<!-- key: gen-solo-btn -->
solo: lit, Gen {gen} is the only generator heard and on the rack's signal; the others keep their pins and their settings

## Mod

<!-- key: mod-matrix -->
Mod — audio modulation: the rack's signal turning the rack's knobs. Each column is a source: RACK — the rack's signal, as the Mixer makes it, both sides summed (st) or one (L, R) — SEND, the Mixer's MOD A and B, which only modulation reads — or HEAD, where the model is drawn now (x, y, z, and r, its distance from the center). Each row is a control: the running model's parameters, then the view's. A pin routes the column to the row; another pin in the same row moves the route; a lit pin pressed again takes it away. DEST, DPTH and EQ under it are the selected row's route: which control, how deep, and which frequency bands drive it.

### The matrix

<!-- key: mod-grid -->
The routes: a column for each source, a row for each control, in blocks of nine. One route to a row: press a pin to route its column to the row, another pin in the row to move it, a lit one to take it away; the wheel over a lit pin sets its depth.

### The columns

- <!-- key: mod-group=rack --> RACK — the rack's signal, as the Mixer's RACK rows make it
- <!-- key: mod-group=send --> SEND — the Mixer's MOD A and B rows: whatever is pinned there, and nowhere else need it be
- <!-- key: mod-group=head --> HEAD — the head of the model's trail, where it is drawn now: the model steering itself. Not Model Out, which is the model as sound, on the Mixer.

- <!-- key: mod-src=st --> st — both sides of the rack's signal summed drive the control
- <!-- key: mod-src=L --> L — RACK L alone drives the control
- <!-- key: mod-src=R --> R — RACK R alone drives the control
- <!-- key: mod-src=A --> A — the Mixer's MOD A send drives the control
- <!-- key: mod-src=B --> B — the Mixer's MOD B send drives the control
- <!-- key: mod-src=x --> head x — the attractor's own current x drives the control, so the system feeds back into itself. Small depths drift the shape; large ones make a different system, which is the point. Smoothed, because a loop that answers within a frame of its own output is an oscillator at the frame rate.
- <!-- key: mod-src=y --> head y — as head x, on the y coordinate
- <!-- key: mod-src=z --> head z — as head x, on the z coordinate. The classic one to try: a Lorenz whose rho follows its own z.
- <!-- key: mod-src=r --> head r — the attractor's distance from the origin drives the control: a source that does not care which way the orbit went, only how far out it is

### A row

`{control}` is the control the row is for, and `{src}` a column.

<!-- key: mod-row -->
{control} — select this control: DEST, DPTH and EQ show its route

<!-- key: mod-row.none -->
No parameter here for this model

<!-- key: mod-pin -->
{src} → {control} — press to route, again to take it away; the wheel sets its depth

### DEST

<!-- key: mod-dest -->
DEST — which control the route below is for. The matrix's row names say the same; this knob steps through them.

### DPTH

<!-- key: mod-depth -->
DPTH — how strongly the source drives the selected control; minus inverts it, 0 is no route at all. About 1.5 overdrives.

<!-- key: mod-depth.field -->
Depth of the selected route — type or scroll

### EQ

<!-- key: mod-eq -->
EQ — which frequency bands, low to high, drive the selected control. Drag across the bars to paint them.

<!-- key: mod-eq.strip -->
EQ for {control} — drag to pick which frequency bands (low→high) drive the {control} parameter
