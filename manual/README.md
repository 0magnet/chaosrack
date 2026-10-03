# The chaosrack manual

Everything the rack says about itself, in one place. A control's tooltip,
the Info window (with the rack switched off) and this manual are the same
text. Read on the rack's own server at **/manual**, it is written by the rack
itself, so every control is listed where it actually is, by its address.

The manual goes in three parts: this page; **the rack, bay by bay**, every
module as it is placed and every control in it; and **the models**, what each
one is and what each of its constants does.

## Using this page

Each bay on this page is the rack's own, drawn whole, and it works: turn a
knob here and the rack answers. **⧉ window** beside a bay's heading takes the
bay out into a window, which you can make as large as you like; arrange the
windows where you want them and they stay put while the text scrolls under
them. Closing a window puts what it held back. **⧉ the model**, at the top of
the side bar, puts the model itself in a window and switches the rack on, as
the Console's Power switch does; closing it switches the rack off.

Each control on a module here carries its address on a small tag, and
holding the pointer over an entry lights the tag of the control it is about.

## Addresses

Every control has an address, **bay.column.row**. It begins the control's
tooltip, and it is how the bays below list them:

- **bay** is the number on the bay's left ear;
- **column** is how far across the bay the control stands, in slots: a bay
  holds twelve modules of the narrowest width side by side, and each of
  those widths is a column, 1 at the left to 12 at the right;
- **row** is which of the bay's three rows of controls it is in, 1 at the
  top to 3 at the bottom.

So 1.3.1 is the top of the third column of bay 1, and the next control to its
right is 1.4.1. A module is addressed by its first column: the Visual bay's
monitor is two slots wide, so it is 1.1 and the bank beside it starts at 1.3.
Two or more controls in one cell — a knob and the readout over it, a column of
switches — are lettered, top to bottom and then left to right: 3.2.1.a,
3.2.1.b. A part that covers several cells — a monitor's screen, a scope's
tube, a pin matrix, the keybed — takes the address of its top left one.
Modules move between bays as the rack is packed into a window, so an address
is where a control is now: it is read off the rack each time this page is
written, never stored.

## The P-unit

Most of the rack is built from one part, the **P-unit** (program unit): a
cell with its legend on a display down the left edge, the setting on a
display over an endless knob ringed with LEDs, a step readout and its small
knob, a reset, and three buttons, + 0 −, down the right edge. It is
programmable: a model bank's P-units are reprogrammed by its MODEL knob, so
the displays say what each one does for the model now playing, and a
selector or a switch is the same part with detents. Click the setting's
display and every position is listed. Every position of a model bank is a
P-unit, and so is every control on the Grid.

## Signal flow

```text
 SOURCES                        MIXER                         WHERE IT GOES
                        a pin where a column
                        crosses a row mixes
                        the one into the other
 CAP L, CAP R ──────────────┐                     ┌──▶ SPK L, SPK R ──▶ the speakers
   the system's audio,      │                     │     (never the capture)
   or the microphone        │   ┌─────────────┐   │
 GEN 1 2 3 4 ───────────────┼──▶│  pin matrix │───┼──▶ RACK L, RACK R ─▶ the rack's signal
 MODEL X Y Z (Model Out) ───┤   └─────────────┘   │         │
 KEYS, DRUMS, MATRIX,       │                     └──▶ MOD A, MOD B ──▶ the Mod matrix only
   SOUNDS (the models' own)─┘                                │
                         ┌───────────────────────────────────┤
                         ▼                  ▼                ▼
                  the meters        the audio models    the Mod matrix ──▶ any knob
                  (Console)         (Audio, Scope rows) (RACK, A, B, HEAD)

 The scopes read RACK L and R, or any one source directly: CAP L/R, GEN 1–4, MODEL X–Z.
```

Every sound on the rack is a column of the **Mixer**, and goes nowhere until
it is pinned. Its rows are what a sound can be for:

- **SPK L, SPK R** — to be **heard**.
- **RACK L, RACK R** — to be **measured**: the rack's signal, which the
  meters, the models drawn from audio, the scopes' RACK inputs and the Mod
  matrix all read.
- **MOD A, MOD B** — to **turn knobs** and nothing else: two sends only the
  Mod matrix reads, so a generator or the model can drive a parameter without
  being heard or changing what the meters show.

A source can be on any of them at once. The capture — what the system is
playing, or the microphone — has no speaker pins at all: the speakers are
already playing it, and playing it back through them would feed it to itself.

The **Mod** matrix beside it turns those signals into movement: each of its
columns is a source (the rack's signal, MOD A or B, or where the model's
trail is now) and each row a knob.

## Writing the manual

The text is the markdown files in the repository's `manual/` directory, one
to a part of the rack. Each entry is a paragraph, or a list item, after a
comment naming its key — the id the panel looks it up by:

```markdown
### Solo

<!-- key: gen-solo -->
Solo — the one generator on the rack's signal and the speakers.

- <!-- key: color-map=5 --> heat — black through red and orange to white
```

A key with `=` in it is one position of a selector. `{name}` in an entry is
filled in by the rack (a generator's number, a model's name). The comments do
not show where markdown is rendered, so the files read on their own as well,
and the tests check that every key the panel asks for is in them.

Serve the rack with `--manual-dir manual` and this page is written from the
files on disk instead of the copy built in, so an edit shows on reload.
