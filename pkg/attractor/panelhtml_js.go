//go:build js && wasm

package attractor

import (
	_ "embed"
)

// ── Controls panel HTML ──────────────────────────────────────────────────────

//go:embed panel.css
var panelCSS string

// controlsBody is the panel markup; the stylesheet lives in panel.css
// (embedded above) and is prepended as a <style> block at inject time.
const controlsBody = `
<div class="modules">
<div class="sect console"><div class="sect-hdr" title="Console — model selection, global actions, and every mode/effect switch in one module">Console</div>
<div class="row toprow swrow swsecs">
  <div class="swsec consec"><div class="swsec-hdr" title="Model selection and global panel actions">Model</div>
  <span class="grp modelsel-col" data-no-drag id="model-sel-grp" title="Model selector — outer knob picks the category, inner knob the model">
  <span class="u-lbl mlbl">Model</span>
  <span id="modelknob-holder"></span>
  <select id="mode-select" class="selwin" style="display:none"></select>
  <select id="cat-select" class="selwin csel-cat" data-no-drag title="Model category — the optgroup the model knob is browsing"></select>
  <select id="model-select" class="selwin csel-model" data-no-drag title="Model within the selected category"></select>
  </span>
  <span class="grp" id="dock-controls" title="Dock the controls to an edge of the window, or detach as a floating panel"><span class="dock-lbl">DOCK</span><button class="ctrl-btn dockb" id="dock-top" title="Dock the panel to the top edge">↑</button><button class="ctrl-btn dockb" id="dock-bottom" title="Dock the panel to the bottom edge">↓</button><button class="ctrl-btn dockb" id="dock-left" title="Dock the panel as a left sidebar">←</button><button class="ctrl-btn dockb" id="dock-right" title="Dock the panel as a right sidebar">→</button><button class="ctrl-btn dockb" id="dock-float" title="Detach the panel as a floating window">⧉</button><button class="ctrl-btn dockb" id="dock-footer" title="Dock the panel inline into the host page's footer, below its own content">▣</button></span>
  <div class="console-btns">
  <span class="grp btn-row"><button class="pushbtn" id="reset-all-btn" title="Reset parameters, colors, view pose, trail, gradient, effect switches (Front/Fill/audio/backdrops), display style, and the signal generator to their defaults (keeps your dock layout + interface size)"></button><span class="btn-lbl">Reset All</span></span>
  <span class="grp btn-row"><button class="pushbtn" id="normalize-btn" title="Re-center the model to the default (identity) orientation and stop any spin"></button><span class="btn-lbl">Normalize</span></span>
  <span class="grp btn-row"><button class="pushbtn" id="screenshot-btn" title="Save a PNG screenshot of the current view"></button><span class="btn-lbl">Screenshot</span></span>
  </div>
  </div>
  <div class="swsec"><div class="swsec-hdr" title="Setup — equation editing and video capture">Setup</div>
    <label class="grp" style="cursor:pointer;" title="Load the current attractor's equations into the editable Custom mode; toggle off to return to it"><input type="checkbox" class="sw" id="edit-eq-sw"> Edit eqn</label>
    <label class="grp" style="cursor:pointer;" title="Record the canvas to a .webm video (MediaRecorder) — flip off to stop and download. Video only; capture system audio externally if needed."><input type="checkbox" class="sw" id="rec-sw"> Record</label>
    <span id="extra-nav"></span>
  </div>
  <div class="swsec"><div class="swsec-hdr" title="Motion — spin, pause, and the autonomous jam performer">Motion</div>
    <label class="grp" style="cursor:pointer;" title="Continuously spin the model about the vertical (Y) axis — folds into the Y spin-rate knob"><input type="checkbox" class="sw" id="auto-rotate" checked> Auto-rotate</label>
    <label class="grp" style="cursor:pointer;" title="Pause / freeze the animation"><input type="checkbox" class="sw" id="pause-sw"> Pause</label>
    <label class="grp" style="cursor:pointer;" title="Jam / attract mode — the app performs itself: hops to a random attractor every 12–20 s with fresh gentle spin and occasional persist paint. Never touches speaker outputs."><input type="checkbox" class="sw" id="jam-sw"> Jam</label>
  </div>
  <div class="swsec"><div class="swsec-hdr" title="Trace — how the trajectory is drawn (points, persist, ring beam, twin, section, gradient direction)">Trace</div>
    <label class="grp" style="cursor:pointer;" title="Draw the trajectory as discrete points instead of a connected line"><input type="checkbox" class="sw" id="use-points"> Points</label>
    <label class="grp" style="cursor:pointer;" title="Draw the model in front of the controls (controls stay clickable through it)"><input type="checkbox" class="sw" id="model-front"> Front</label>
    <label class="grp" style="cursor:pointer;" title="Fill the screen with the spectrogram / FVF display (face-on) instead of the rotatable plane"><input type="checkbox" class="sw" id="spect-fill"> Fill</label>
    <label class="grp" style="cursor:pointer;" title="Keep the entire trail on screen (never clear old points) — accumulates the full attractor"><input type="checkbox" class="sw" id="persist-trail"> Persist</label>
    <label class="grp" style="cursor:pointer;" title="Ring trail — scope-style beam: only the advancing head integrates each frame (the trail is its history), so knob/audio changes bend the path from the head forward instead of reshaping the whole curve, and long trails cost almost nothing. Off = classic scan (whole curve recomputed and reshaped every frame)."><input type="checkbox" class="sw" id="ring-sw"> Ring</label>
    <label class="grp" style="cursor:pointer;" title="Twin trajectories — a second copy of the flow starts ε apart (green) and the two visibly separate at the attractor's own Lyapunov rate; the λ readout is a live largest-Lyapunov-exponent estimate (positive = chaotic). Both copies use the same integrator, so what separates them is the dynamics."><input type="checkbox" class="sw" id="twin-sw"> Twin <span class="led" id="twin-lambda" title="Live largest Lyapunov exponent estimate — positive means chaos (e-folding rate of the separation per time unit)"></span></label>
    <label class="grp" style="cursor:pointer;" title="Poincaré section — sample the trajectory only where it pierces the z plane through the attractor's center (positive-going crossings) and draw the accumulated intersections in gold: the flow's sheets collapse into the section's fractal scatter."><input type="checkbox" class="sw" id="sect-sw"> Sect</label>
    <label class="grp" style="cursor:pointer;" title="Reverse the color-gradient direction (swap start and end)"><input type="checkbox" class="sw" id="gradient-reverse"> Invert</label>
  </div>
  <div class="swsec"><div class="swsec-hdr" title="Audio — modulation, sources (test tone / signal gen), MIDI, meters, and audio backdrops">Audio</div>
    <label class="grp" style="cursor:pointer;" title="Enable audio-reactive modulation — reveals the MOD + EQ modules for routing audio features to each control"><input type="checkbox" class="sw" id="audio-mod"> Audio mod</label>
    <label class="grp" style="cursor:pointer;" title="Play a sweeping test tone (captured back via the server) to exercise the audio modulation"><input type="checkbox" class="sw" id="test-tone"> Test tone</label>
    <label class="grp" style="cursor:pointer;" title="WebMIDI — hardware control: CC 1..N drive the current mode's parameter knobs in order, CC 21..28 the view targets (zoom, pans, spins, rainbow, trail), and any note hops to that note's attractor."><input type="checkbox" class="sw" id="midi-sw"> MIDI</label>
    <label class="grp" style="cursor:pointer;" title="Show / hide the top-left audio feature meters (amp / bass / mid / treble / cntr / beat) while Audio mod is on"><input type="checkbox" class="sw" id="show-meters" checked> Meters</label>
    <label class="grp" style="cursor:pointer;" title="Signal generator — a built-in client-side audio source (three X/Y/Z oscillators). Drives audio modulation, the spectrogram and the xy scope with no server or microphone, and can play over the speakers. Controls are in the Generator module."><input type="checkbox" class="sw" id="fg-on"> Signal gen</label>
    <label class="grp" style="cursor:pointer;" title="Paint the live audio spectrogram onto the current surface model (sphere / globe / torus)"><input type="checkbox" class="sw" id="spectro-skin"> Spectro-skin</label>
    <label class="grp" style="cursor:pointer;" title="Backdrop: scrolling spectrogram behind the model"><input type="checkbox" class="sw" id="bg-spectro"> Spectro bg</label>
    <label class="grp" style="cursor:pointer;" title="Backdrop: XY oscilloscope behind the model"><input type="checkbox" class="sw" id="bg-xy"> XY bg</label>
    <select id="bg-visual" style="display:none"><option value="">off</option><option value="spectrogram">spectrogram</option><option value="xy">xy scope</option></select>
  </div>
  <div class="swsec"><div class="swsec-hdr" title="Window — overlays and optional modules (info, fullscreen, template, patchbay)">Window</div>
    <label class="grp" style="cursor:pointer;" title="Overlay a short description of the current attractor / model on the view"><input type="checkbox" class="sw" id="show-info"> Info</label>
    <label class="grp" style="cursor:pointer;" title="Toggle browser fullscreen — the canvas fills the display; the panel stays available"><input type="checkbox" class="sw" id="fullscreen-sw"> Fullscreen</label>
    <label class="grp" style="cursor:pointer;" title="Show the Template module — a labeled legend of every named slot a module can have (header, label, readout, knob, ring, inner, fine, dial, reset). Hover a slot for the Go struct field it maps to."><input type="checkbox" class="sw" id="tpl-on"> Template</label>
    <label class="grp" style="cursor:pointer;" title="Show the Patchbay module — an EMS-Synthi-style pin matrix routing audio energy (stereo/L/R) to any parameter or view control, plus an 8-slot patch memory bank (STO + slot stores, slot recalls)."><input type="checkbox" class="sw" id="patch-on"> Patchbay</label>
    <label class="grp" style="cursor:pointer;" title="Show the Counter module — a NAND-gate-style frequency counter: counts trigger crossings of the live audio source over a gate window and shows cycles per second on a DSEG readout."><input type="checkbox" class="sw" id="counter-on"> Counter</label>
    <label class="grp" style="cursor:pointer;" title="Show the Keys module — a playable polyphonic piano keyboard, up to the full 88 keys, with voice and speaker routing; the computer keyboard plays the labeled octaves (Z row + Q row)."><input type="checkbox" class="sw" id="keys-on"> Keys</label>
  </div>
</div>
</div>
<div class="sect"><div class="sect-hdr" title="Parameters — the current model's tunable constants (each with knob, LED value, step size, and reset)">Parameters</div><div id="params" class="row"></div></div>
<div class="sect" id="pong-module" style="display:none"><div class="sect-hdr" title="Scoreboard — Scope Pong's front panel: each player's score over their paddle pot (turn it to seize the paddle from the machine; it spins by itself while the machine plays, like a motorized pot), plus a restart button">Scoreboard</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">left</span><span class="led demo-led" id="pong-score-l" title="Left player's score (W/S, left-half touch, or the pot below) — first past 9 resets the match">0</span></span><input type="range" id="pong-pad-l" min="-1" max="1" step="0.01" value="0" title="Left paddle pot — turn to take the left paddle from the machine; tracks the paddle while the machine or keys drive it" style="display:none"><span class="grp vmbay"><span id="pong-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">right</span><span class="led demo-led" id="pong-score-r" title="Right player's score (↑/↓, right-half touch, or the pot below) — first past 9 resets the match">0</span></span><input type="range" id="pong-pad-r" min="-1" max="1" step="0.01" value="0" title="Right paddle pot — turn to take the right paddle from the machine; tracks the paddle while the machine or keys drive it" style="display:none"><span class="grp vmbay"><span id="pong-rstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp btn-row demo-btn"><button class="pushbtn" id="pong-restart" title="Restart the match — zero both scores and serve fresh"></button><span class="btn-lbl">Restart</span></span></span>
</div></div>
<div class="sect" id="stext-module" style="display:none"><div class="sect-hdr" title="Banner — Fourier Text's input: what the harmonic character generator writes (A–Z, 0–9, dash, space). The harm knob in Parameters sets how many harmonics each glyph keeps.">Banner</div>
<div class="row keysflex stext-row">
  <span class="pcell axcol vmcell gen-cell stext-cell"><span class="punit-top"><span class="plabel">text</span></span><input type="text" id="stext-in" class="stext-in" maxlength="24" data-no-drag title="Banner text — what the beam writes; melt it with the harm knob in Parameters"></span>
</div></div>
<div class="sect" id="smorph-module" style="display:none"><div class="sect-hdr" title="Patch — the self-programming analog computer's wiring readout: which two catalog systems the machine is blended between right now">Patch</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">wired</span></span><span class="grp vmbay"><span class="led demo-led smorph-led" id="smorph-led" title="Live patch — the current catalog blend, e.g. D-E 42% (100% = fully the next system); the sys knob parks it, the rate knob self-steps">D</span></span></span>
</div></div>
<div class="sect" id="bounce-module" style="display:none"><div class="sect-hdr" title="Launcher — Bouncing Ball's front panel: the drop-height pot (the analog demo's initial-condition setting), the machine's re-kick count, and a Drop button">Launcher</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">height</span><input type="number" class="numin gen-led" id="bounce-height-led" title="Drop height (0.2–1.0, court units above the floor) — where the next Drop releases the ball" min="0.2" max="1" step="0.05" value="0.90"></span><input type="range" id="bounce-height" min="0.2" max="1" step="0.05" value="0.9" title="Drop height pot — the initial condition the next Drop (or mode entry) releases the ball from" style="display:none"><span class="grp vmbay"><span id="bounce-hstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">kicks</span><span class="led demo-led" id="bounce-kicks" title="Machine re-kicks since the mode started — each is a decayed ball re-launched">0</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp btn-row demo-btn"><button class="pushbtn" id="bounce-drop" title="Drop the ball again from the height pot's setting with a fresh drift"></button><span class="btn-lbl">Drop</span></span></span>
</div></div>
<div class="sect"><div class="sect-hdr" title="Colors — gradient source axis, palette size, rainbow period, and trail length">Colors</div>
<div class="row vmrow">
  <span class="pcell axcol" id="src-cell" title="Outer ring = gradient source (which axis the color follows); inner ring = number of colors. Overridden while a Phosphor is selected."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">src</span><span id="gradient-stack"></span></span></span><select id="gradient-source" title="Gradient source — which axis (X / Y / Z / trail) the trace color follows" style="display:none">
    <option value="0">X</option>
    <option value="1">Y</option>
    <option value="2" selected>Z</option>
    <option value="3">trail</option>
  </select><select id="gradient-colors" title="Palette — number of gradient colors (mono / 2-color / 3-color / rainbow)" style="display:none">
    <option value="1">mono</option>
    <option value="2" selected>2-color</option>
    <option value="3">3-color</option>
    <option value="4">rainbow</option>
  </select></span>
  <span class="pcell axcol vmcell" id="grp-rainbow" title="Rainbow period — how much of the spectrum spans the trail at once (low = a narrow flowing slice)"><span class="punit-top"><span class="plabel">period</span><input type="number" id="slider-value-rfreq" class="numin" title="Rainbow period value — type or scroll" min="0.05" max="20" step="0.05" value="1"></span><input type="range" id="rainbow-freq" min="0.05" max="20" value="1" step="0.05" title="Rainbow period — spectrum span across the trail"><button class="rst" id="rst-rfreq" title="Reset rainbow period">↺</button></span>
  <span class="pcell axcol vmcell" id="trail-controls" title="Trail length — how many recent points stay lit"><span class="punit-top"><span class="plabel">Trail</span><input type="number" id="slider-value-trail" class="numin" title="Trail length value (points) — type or scroll" min="1000" max="500000" step="1000" value="20000"></span><input type="range" id="trail-slider" min="1000" max="500000" value="20000" step="1000" title="Trail length — how many recent points stay lit"><button class="rst" id="rst-trail" title="Reset trail length">↺</button></span>
</div></div>
<div class="sect"><div class="sect-hdr" title="Palette — the gradient's start / middle / end colors and the background">Palette</div>
<div class="row vmrow two-col">
  <span class="pcell axcol vmcell pal-cell" id="grp-cstart" title="Gradient START color (low end of the source axis) — pick with the swatch or dial it with the Hue (outer) / Level (inner) knob"><span class="punit-top"><span class="plabel" id="lbl-cstart">start</span><span class="cswatch"><input type="color" id="color-base" value="#ff0000" title="Gradient start color"><button class="rst" id="rst-color-base" title="Reset start color">↺</button></span></span><span class="ckholder" id="ck-color-base"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cmid" title="Gradient MIDDLE color (3-color palette only) — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">mid</span><span class="cswatch"><input type="color" id="color-mid" value="#00ff00" title="Gradient middle color"><button class="rst" id="rst-color-mid" title="Reset middle color">↺</button></span></span><span class="ckholder" id="ck-color-mid"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cend" title="Gradient END color (high end of the source axis) — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">end</span><span class="cswatch"><input type="color" id="color-top" value="#0000ff" title="Gradient end color"><button class="rst" id="rst-color-top" title="Reset end color">↺</button></span></span><span class="ckholder" id="ck-color-top"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cbg" title="Background color behind the model — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">bg</span><span class="cswatch"><input type="color" id="color-bg" value="#000000" title="Background color"><button class="rst" id="rst-color-bg" title="Reset background color">↺</button></span></span><span class="ckholder" id="ck-color-bg"></span></span>
</div></div>
<div class="sect"><div class="sect-hdr" title="View — orientation: per-axis angle knobs and continuous spin rates">View</div>
<div class="row vmrow vmaxrow">
  <span class="pcell axcol axrot" data-no-drag title="X axis — angle knob, spin rate, horizontal position">
    <span class="plabel">X</span>
    <span class="grp"><div class="knob" id="knob-x" title="X rotation angle — drag or turn to tilt about X"><i class="knob-ptr" id="knobptr-x"></i></div><span class="led" id="led-x" title="X rotation angle in degrees — drag the model or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-x" min="-1" max="1" value="0" step="0.1" title="X spin rate — continuous rotation about X"><input type="number" id="slider-value-x" class="numin" title="X spin rate value — type or scroll" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rx" title="Reset X angle + spin rate">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag title="Y axis — angle knob, spin rate, vertical position">
    <span class="plabel">Y</span>
    <span class="grp"><div class="knob" id="knob-y" title="Y rotation angle — drag or turn to tilt about Y"><i class="knob-ptr" id="knobptr-y"></i></div><span class="led" id="led-y" title="Y rotation angle in degrees — drag the model or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-y" min="-1" max="1" value="0" step="0.1" title="Y spin rate — continuous rotation about Y"><input type="number" id="slider-value-y" class="numin" title="Y spin rate value — type or scroll" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-ry" title="Reset Y angle + spin rate">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag title="Z axis — angle knob, spin rate, zoom (depth)">
    <span class="plabel">Z</span>
    <span class="grp"><div class="knob" id="knob-z" title="Z rotation angle — drag near the rim to roll about Z"><i class="knob-ptr" id="knobptr-z"></i></div><span class="led" id="led-z" title="Z rotation angle in degrees — drag near the rim or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-z" min="-1" max="1" value="0" step="0.1" title="Z spin rate — continuous rotation about Z"><input type="number" id="slider-value-z" class="numin" title="Z spin rate value — type or scroll" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rz" title="Reset Z angle + spin rate">↺</button></span>
  </span>
</div>
</div>
<div class="sect"><div class="sect-hdr" title="Position — slide the model horizontally / vertically and zoom the camera">Position</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">X</span><input type="number" id="slider-value-panx" class="numin" title="X position value — type or scroll" min="-8" max="8" step="1" value="0"></span><input type="range" id="pan-x" min="-8" max="8" value="0" step="1" title="X position — slide the model horizontally"><button class="rst" id="rst-panx" title="Reset X position">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Y</span><input type="number" id="slider-value-pany" class="numin" title="Y position value — type or scroll" min="-8" max="8" step="1" value="0"></span><input type="range" id="pan-y" min="-8" max="8" value="0" step="1" title="Y position — slide the model vertically"><button class="rst" id="rst-pany" title="Reset Y position">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Zoom</span><input type="number" id="slider-value-zoom" class="numin" title="Zoom value — type or scroll" min="-95" max="95" step="1" value="0"></span><input type="range" id="camera-zoom" min="-95" max="95" value="0" step="1" title="Zoom — camera distance to the model"><button class="rst" id="rst-zoom" title="Reset zoom">↺</button></span>
</div>
</div>
<div class="sect"><div class="sect-hdr" title="Display — animation speed, line width, and the knob step / fine multipliers">Display</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Speed</span><input type="number" id="slider-value-speed" class="numin" title="Speed value ×0.01–×100 — type or scroll" min="0.01" max="100" step="0.01" value="1"></span><input type="range" id="speed-slider" min="-2" max="2" value="0" step="0.1" title="Speed — animation rate (sub-steps / dt scale)"><button class="rst" id="rst-speed" title="Reset speed">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Line</span><input type="number" id="slider-value-line" class="numin" title="Line width value — type or scroll" min="1" max="10" step="1" value="1"></span><input type="range" id="line-width" min="1" max="10" value="1" step="1" title="Line width — trace thickness"><button class="rst" id="rst-line" title="Reset line width">↺</button></span>
  <span class="pcell axcol vmcell sf-cell" id="stepfine-grp" title="Outer ring = Step× (coarse step), inner ring = Fine× (fraction of a step)"><span class="sf-hdr"><span class="sf-row"><span class="plabel">step</span><span class="led sf-led" id="step-led" title="current Step×">1</span></span></span><span class="grp vmbay"><span id="stepfine-stack"></span></span><span class="sf-ftr"><span class="sf-row"><span class="plabel">fine</span><span class="led sf-led" id="fine-led" title="current Fine×">.1</span></span></span><select id="step-ratio" title="Step multiplier — scales every knob’s coarse step (outer ring)" style="display:none"><option value="0.25">0.25</option><option value="0.5">0.5</option><option value="1" selected>1</option><option value="2">2</option><option value="5">5</option></select><select id="fine-ratio" title="Fine multiplier — the fine-trim disc’s fraction of a step (inner knob)" style="display:none"><option value="1">1</option><option value="0.1" selected>0.1</option><option value="0.01">0.01</option><option value="0.001">0.001</option></select></span>
</div></div>
<div class="sect"><div class="sect-hdr" title="Style — interface size, knob face + LED color, and CRT phosphor">Style</div>
<div class="row vmrow">
  <span class="pcell axcol" title="Size of every knob (S / M / L / XL) — scales the whole control interface."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Size</span><span id="size-stack"></span></span></span><select id="knob-size" title="Interface size — scales every knob and readout (S / M / L / XL)" style="display:none"><option value="0.85">S</option><option value="1" selected>M</option><option value="1.3">L</option><option value="1.7">XL</option></select></span>
  <span class="pcell axcol" title="Outer ring = knob appearance (std / flat / vint / chrome / gold / carbon); inner ring = LED readout color."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Knob</span><span id="knobstyle-stack"></span></span></span><select id="knob-style" title="Knob appearance — face finish (outer ring: std / flat / vint / chrome / gold / carbon)" style="display:none"><option value="std" selected>std</option><option value="flat">flat</option><option value="vint">vint</option><option value="chrome">chrome</option><option value="gold">gold</option><option value="carbon">carbon</option></select><select id="led-color" title="LED color — readout hue for every LED display (inner knob)" style="display:none"></select></span>
  <span class="pcell axcol" id="phosphor-cell" title="CRT phosphor for scope traces (Lissajous / Graphic Artist) — sets trace color + afterglow. P31 crisp green … P7 blue→green … P33 long amber."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Phosphor</span><span id="phosphor-stack"></span></span></span><select id="phosphor" title="Phosphor — CRT trace color and afterglow for scope modes" style="display:none"></select></span>
</div>
</div>
<div class="sect gen-osc" id="gen-x-module"><div class="sect-hdr" title="Oscillator X — the xy scope's horizontal axis. freq knob (log, equal turn per octave), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen X</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-x-led" title="Gen X frequency in Hz — type a value or turn the knob (log scale, one turn per octave)" min="27.5" max="28160" step="1" value="200"></span><input type="range" id="gen-x-freq" min="0" max="120" step="1" value="34" style="display:none"><span class="grp vmbay"><span id="gen-x-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-x-lvl-led" title="Gen X level (0–100)" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-x-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-x-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-x-ostack"></span></span><select id="gen-x-out" title="X output — speaker channel routing (outer ring: off / L / R / L+R)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-x-wave" title="X waveform — oscillator shape (inner knob: sine / tri / sqr / saw / noise)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option><option value="4">noise</option></select></span>
</div></div>
<div class="sect gen-osc" id="gen-y-module"><div class="sect-hdr" title="Oscillator Y — the xy scope's vertical axis. freq knob (log), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen Y</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-y-led" title="Gen Y frequency in Hz — type a value or turn the knob (log scale, one turn per octave)" min="27.5" max="28160" step="1" value="300"></span><input type="range" id="gen-y-freq" min="0" max="120" step="1" value="41" style="display:none"><span class="grp vmbay"><span id="gen-y-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-y-lvl-led" title="Gen Y level (0–100)" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-y-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-y-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-y-ostack"></span></span><select id="gen-y-out" title="Y output — speaker channel routing (outer ring: off / L / R / L+R)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-y-wave" title="Y waveform — oscillator shape (inner knob: sine / tri / sqr / saw / noise)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option><option value="4">noise</option></select></span>
</div></div>
<div class="sect gen-osc" id="gen-z-module"><div class="sect-hdr" title="Oscillator Z — a third generator (audio / modulation). freq knob (log), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen Z</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-z-led" title="Gen Z frequency in Hz — type a value or turn the knob (log scale, one turn per octave)" min="27.5" max="28160" step="1" value="150"></span><input type="range" id="gen-z-freq" min="0" max="120" step="1" value="29" style="display:none"><span class="grp vmbay"><span id="gen-z-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-z-lvl-led" title="Gen Z level (0–100)" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-z-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-z-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-z-ostack"></span></span><select id="gen-z-out" title="Z output — speaker channel routing (outer ring: off / L / R / L+R)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-z-wave" title="Z waveform — oscillator shape (inner knob: sine / tri / sqr / saw / noise)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option><option value="4">noise</option></select></span>
</div></div>
<div class="sect gen-osc" id="gen-env-module"><div class="sect-hdr" title="Envelope — the Complex Sound Generator's shaper for the signal generator's speaker output: in RPT mode it cycles attack → decay continuously, a shaped tremolo over whatever Gen X/Y/Z are routed to the speakers. OFF passes the generators through untouched. Analysis paths (scope, spectrogram, meters) stay unshaped.">Envelope</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">atk</span><input type="number" class="numin gen-led" id="gen-env-atk-led" title="Envelope attack time in milliseconds — the rise from silence to full level" min="1" max="2000" step="1" value="10"></span><input type="range" id="gen-env-atk" min="1" max="2000" step="1" value="10" style="display:none"><span class="grp vmbay"><span id="gen-env-astack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">dcy</span><input type="number" class="numin gen-led" id="gen-env-dcy-led" title="Envelope decay time in milliseconds — the fall from full level back to silence" min="1" max="5000" step="1" value="300"></span><input type="range" id="gen-env-dcy" min="1" max="5000" step="1" value="300" style="display:none"><span class="grp vmbay"><span id="gen-env-dstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">mode</span></span><span class="grp vmbay"><span id="gen-env-mstack"></span></span><select id="gen-env-mode" title="Envelope mode — OFF: generators pass through untouched · RPT: attack → decay repeats continuously (shaped tremolo)" style="display:none"><option value="off" selected>off</option><option value="rpt">rpt</option></select></span>
</div></div>
<div class="sect gen-osc" id="sonify-module"><div class="sect-hdr" title="Model Out — HEAR the attractor. Inner knob picks how: FLOW integrates the attractor's own equations at audio rate, so the pitch is the system's natural orbital frequency (chaos chirps, periodic windows lock to tones, parameter changes are audible) and RATE transposes it (A4 = ×1); SCAN traces the drawn trail as one waveform period at exactly RATE Hz (a stable, playable tone). MAP picks which two coordinates drive L/R (CAM = the screen's x/y, so rotating the model changes the sound; off = silent). LVL is output level.">Model Out</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">rate</span><input type="number" class="numin gen-led" id="sonify-led" title="Model Out RATE — FLOW: transposition in Hz (440 = ×1) · SCAN: trail sweeps per second" min="27.5" max="28160" step="1" value="110"></span><input type="range" id="sonify-freq" min="0" max="120" step="1" value="24" style="display:none"><span class="grp vmbay"><span id="sonify-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="sonify-lvl-led" title="Model Out level (0–100)" min="0" max="100" step="1" value="60"></span><input type="range" id="sonify-lvl" min="0" max="100" step="1" value="60" style="display:none"><span class="grp vmbay"><span id="sonify-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">map</span></span><span class="grp vmbay"><span id="sonify-mstack"></span></span><select id="sonify-map" title="Model Out mapping (outer ring) — which two model coordinates drive L/R (off = silent)" style="display:none"><option value="off" selected>off</option><option value="cam">CAM</option><option value="xy">XY</option><option value="xz">XZ</option><option value="yz">YZ</option></select><select id="sonify-mode" title="Model Out mode (inner knob) — FLOW: audify the dynamics (pitch emergent, RATE transposes) · SCAN: trail as wavetable (RATE = exact Hz)" style="display:none"><option value="flow" selected>FLOW</option><option value="scan">SCAN</option></select></span>
</div></div>
<div class="sect gen-osc" id="counter-module" style="display:none"><div class="sect-hdr" title="Counter — a frequency counter for the rack, in the spirit of the glensstuff.com NAND-gate counter: it counts trigger crossings of the live audio source over the gate window and shows cycles per second, the way the discrete-logic original did. Feed it the mic, the ws stream, or the signal generator.">Counter</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><span class="led counter-led" id="counter-led" title="Measured frequency in Hz — cycles counted over the last closed gate window">-----.-</span></span><span class="grp vmbay"><span class="counter-gate" id="counter-gate" title="Gate lamp — flashes each time the gate window closes and the readout updates"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">gate</span></span><span class="grp vmbay"><span id="counter-gstack"></span></span><select id="counter-gatesel" title="Gate time in seconds — how long the counter counts before updating (longer gate = finer resolution, slower updates)" style="display:none"><option value="0.1">0.1</option><option value="0.5">0.5</option><option value="1" selected>1</option><option value="2">2</option></select></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">trig</span><input type="number" class="numin gen-led" id="counter-trig-led" title="Trigger level (% of full scale) — the hysteresis threshold a cycle must swing across to count; raise it to reject noise" min="0" max="30" step="1" value="4"></span><input type="range" id="counter-trig" min="0" max="30" step="1" value="4" style="display:none"><span class="grp vmbay"><span id="counter-tstack"></span></span></span>
</div></div>
<div class="sect gen-osc keys-sect" id="keys-module" style="display:none"><div class="sect-hdr" title="Keys — a playable polyphonic keyboard: click or drag the keybed (glissando works), or play the computer keyboard on the labeled keys (Z row = lower octave, Q row = upper). The range ring sets the key count up to the full 88; voice and speaker routing live on the out knob.">Keys</div>
<div class="row keysflex">
<div class="vmrow keysctl">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">range</span><span class="led" id="keys-size-led" title="Keyboard size — how many piano keys are on the bed right now">49</span></span><span class="grp vmbay"><span id="keys-rstack"></span></span><select id="keys-span" title="Keyboard range — key count (outer ring: 13 / 25 / 37 / 49 / 61 / 85, or 88 = the full piano, A0–C8)" style="display:none"><option value="1">13</option><option value="2">25</option><option value="3">37</option><option value="4" selected>49</option><option value="5">61</option><option value="7">85</option><option value="88">88</option></select><select id="keys-base" title="Starting octave — the C the keybed begins on (inner knob; ignored at 88 keys, which always spans A0–C8)" style="display:none"><option value="1">C1</option><option value="2" selected>C2</option><option value="3">C3</option><option value="4">C4</option><option value="5">C5</option></select></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="keys-lvl-led" title="Keys output level (0–100)" min="0" max="100" step="1" value="80"></span><input type="range" id="keys-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="keys-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="keys-ostack"></span></span><select id="keys-out" title="Keys output — speaker channel routing (outer ring: off / L / R / L+R)" style="display:none"><option value="off">off</option><option value="l">L</option><option value="r">R</option><option value="both" selected>L+R</option></select><select id="keys-wave" title="Keys voice — oscillator shape (inner knob: sine / tri / sqr / saw / noise)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option><option value="4">noise</option></select></span>
</div>
<div class="keys-bed" id="keys-bed" data-no-drag title="Piano keybed — click or hold-and-slide to play; the letter-labeled keys map to your computer keyboard"></div>
</div></div>
</div>
<div id="runtime" style="color:#555;font-size:11px;padding-left:40px;"></div>
`

// ── main ─────────────────────────────────────────────────────────────────────

// ledColorDefs is the ORDERED single source for the LED-readout color options.
// It drives the LED-color knob's detents (in sweep order — amber sits between
// red and green), the hidden <select>, the CSS-var color map, and the dial
// dots. Add a color here and it appears everywhere.
var ledColorDefs = []struct{ name, col, glow, bg, bd string }{
	{"red", "#ff3b30", "#ff2a20", "#170000", "#3a0000"},
	{"amber", "#ffb000", "#d08000", "#171000", "#3a2a00"},
	{"green", "#35e06a", "#1c9a44", "#001709", "#0a3a1a"},
	{"blue", "#3bb0ff", "#1c7ad0", "#001017", "#0a2a3a"},
	{"cyan", "#2ce0e0", "#159a9a", "#001515", "#0a3838"},   // the old audio-mod readout hue
	{"violet", "#b06cff", "#8040d0", "#12001f", "#2a0a3a"}, // new
}

// styleKnobRot staggers the knob-style label ring by this many degrees so its
// labels sit between the LED-color dots on the inner ring; the style knob's
// pointer is offset by the same amount so it still points at its labels.
const styleKnobRot = 30.0
