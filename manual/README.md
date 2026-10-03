# chaosrack Operator's Manual

This manual describes the chaosrack front panel: its bays, the modules
mounted in them, and the function of each control. It is the same text as
the panel's tooltips and the Info window. Served by the instrument at
**/manual**, it is generated from the panel as currently configured, so each
control is listed at its present address.

The manual is in three parts:

1. **Introduction**: conventions, addressing and signal flow (this section).
2. **The rack, bay by bay**: each bay as mounted, with every module and
   control in it.
3. **Models**: each model and the function of each of its parameters.

## Using this page

Each bay is shown as installed, and its controls are live: operating a
control here operates the instrument. **⧉ window** beside a bay's heading
detaches the bay into a resizable window, which stays in place while the
text scrolls beneath it; closing the window returns the bay to the page.
**⧉ the model**, at the top of the side bar, opens the display in a window
and powers the rack on, as the Console's Power switch does; closing it powers
the rack off.

Each control carries an address tag. Pointing at an entry in the text
highlights the tag of the control it describes.

## Addresses

Every control has an address of the form **bay.column.row**. The address
begins the control's tooltip and indexes the bay listings below.

- **bay**: the number on the bay's left ear.
- **column**: the slot the control occupies, counted from the bay's left
  edge. A bay is twelve slots wide, one slot being the narrowest module
  width, so columns run 1 to 12.
- **row**: the bay's control row, 1 (top) to 3 (bottom).

Examples: 1.3.1 is row 1 of column 3 in bay 1, and the control to its right
is 1.4.1. A module is addressed by its first column: the Visual monitor
occupies two slots and is 1.1, so the bank beside it begins at 1.3.

Where two or more controls share a cell (a knob and its readout, a column
of switches), each is given a letter, top to bottom and then left to right:
3.2.1.a, 3.2.1.b. A part spanning several cells (a monitor screen, a scope
tube, a pin matrix, the keybed) takes the address of its top-left cell.

Modules are repositioned when the rack is fitted to a window, so addresses
describe the current layout. They are read from the panel each time this
page is generated and are not stored.

## The P-unit

Most panel positions use one standard control, the **P-unit** (program
unit). Each P-unit has:

- a legend display along its left edge;
- a setting display above an endless encoder with an LED ring (click the
  setting display to list every position);
- a step readout with its own trim knob, and a reset;
- three buttons, **+ 0 −**, along its right edge.

A P-unit is programmable. In a model bank, the MODEL selector assigns each
P-unit to a parameter of the selected model, and the displays show the
current assignment. A selector or a switch is the same unit operating in
detented steps. Every position of a model bank, the Grid and the Display
module is a P-unit.

## Signal flow

```text
 SOURCES                        MIXER                         DESTINATIONS
                        a pin at a column/row
                        intersection routes the
                        source to that bus
 CAP L, CAP R ──────────────┐                     ┌──▶ SPK L, SPK R ──▶ speakers
   system audio             │                     │     (capture excluded)
   or microphone            │   ┌─────────────┐   │
 GEN 1 2 3 4 ───────────────┼──▶│  pin matrix │───┼──▶ RACK L, RACK R ─▶ rack bus
 MODEL X Y Z (Model Out) ───┤   └─────────────┘   │         │
 KEYS, DRUMS, MATRIX,       │                     └──▶ MOD A, MOD B ──▶ Mod matrix only
   SOUNDS (model voices) ───┘                                │
                         ┌───────────────────────────────────┤
                         ▼                  ▼                ▼
                     meters           audio models      Mod matrix ──▶ any knob
                    (Console)      (Audio, Scope rows)  (RACK, A, B, HEAD)

 Scope inputs: RACK L/R, or any single source: CAP L/R, GEN 1–4, MODEL X–Z.
```

Every audio source is a column of the **Mixer** and is routed nowhere until
pinned. The rows are buses:

- **SPK L, SPK R**: monitor output to the speakers.
- **RACK L, RACK R**: the rack bus, read by the meters, the audio-driven
  models, the scopes' RACK inputs and the Mod matrix.
- **MOD A, MOD B**: modulation sends, read only by the Mod matrix. A
  generator or the model can drive a parameter through them without being
  heard or appearing on the meters.

A source may be pinned to any number of buses. The capture inputs (system
audio or microphone) have no speaker pins: the speakers already carry that
signal, and routing it back to them would form a feedback loop.

The **Mod** matrix converts signals into control movement. Its columns are
sources (the rack bus, MOD A or B, or the position of the model's trail
head); its rows are destination parameters.

## Maintaining this manual

The text is held in the markdown files in the repository's `manual/`
directory, one per area of the panel. Each entry is a paragraph or list item
preceded by a comment giving its key, the identifier the panel looks it up
by:

```markdown
### Solo

<!-- key: gen-solo -->
Solo: routes only this generator to the rack bus and the speakers.

- <!-- key: color-map=5 --> heat: black through red and orange to white
```

A key containing `=` names one position of a selector. `{name}` in an entry
is substituted by the panel (a generator number, a model name). The comments
are not rendered, so the files remain readable as plain markdown, and the
tests verify that every key the panel requests is present.

Run the server with `--manual-dir manual` to generate this page from the
files on disk rather than the built-in copy; edits then appear on reload.
