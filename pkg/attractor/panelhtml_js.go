//go:build js && wasm

package attractor

import (
	_ "embed" // for the //go:embed directives below
)

// ── Controls panel HTML ──────────────────────────────────────────────────────

//go:embed panel.css
var panelCSS string

// controlsBody is the panel markup; the stylesheet lives in panel.css
// (embedded above) and is prepended as a <style> block at inject time.
const controlsBody = `
<div class="modules">
<div class="sect console"><div class="sect-hdr" data-doc="console">Console</div>
<div class="row toprow swrow swsecs">
  <div class="swsec consec"><div class="swsec-hdr" data-doc="console.equation">Model</div>
  <!-- The single source of truth for which model is running. It has no face
       of its own any more: the two concentric knobs that used to drive it
       sat on the Console, so the one control the whole rack is about was
       nowhere near the controls it governed, and the rack had two model
       selectors to keep in step with each other. The category rows are the
       face now; this is the wire they all drive. -->
  <select id="mode-select" style="display:none"></select>
  <span id="extra-nav"></span>
  <div class="console-btns">
  <span class="grp btn-row"><button class="pushbtn" id="reset-all-btn" data-doc="reset-all-btn"></button><span class="btn-lbl">Reset All</span></span>
  <span class="grp btn-row"><button class="pushbtn" id="normalize-btn" data-doc="normalize-btn"></button><span class="btn-lbl">Normalize</span></span>
  </div>
  </div>
  <div class="swsec"><div class="swsec-hdr" data-doc="console.audio">Audio</div>
    <label class="grp" style="cursor:pointer;" data-doc="midi-sw-cell"><input type="checkbox" class="sw" id="midi-sw"> MIDI</label>
    <select id="gen-solo" data-doc="gen-solo" style="display:none"><option value="" selected>none</option><option value="x">1</option><option value="y">2</option><option value="z">3</option><option value="v">4</option></select>
  </div>
  <div class="swsec"><div class="swsec-hdr" data-doc="console.rack">Rack</div>
    <label class="grp" style="cursor:pointer;" data-doc="power-sw-cell"><input type="checkbox" class="sw" id="power-sw" checked> Power</label>
    <label class="grp" style="cursor:pointer;" data-doc="handles-on-cell"><input type="checkbox" class="sw" id="handles-on"> Rack bay</label>
    <label class="grp" style="cursor:pointer;" data-doc="show-info-cell"><input type="checkbox" class="sw" id="show-info"> Info</label>
    <label class="grp" style="cursor:pointer;" data-doc="fullscreen-sw-cell"><input type="checkbox" class="sw" id="fullscreen-sw"> Fullscreen</label>
    <label class="grp" style="cursor:pointer;" data-doc="desk-contain-cell"><input type="checkbox" class="sw" id="desk-contain"> Desk</label>
  </div>
  <div class="swsec" id="motion-sec"><div class="swsec-hdr" data-doc="console.motion">Motion</div>
    <label class="grp" style="cursor:pointer;" data-doc="auto-rotate-cell"><input type="checkbox" class="sw" id="auto-rotate" checked> Auto-rot</label>
    <label class="grp" style="cursor:pointer;" data-doc="pause-sw-cell"><input type="checkbox" class="sw" id="pause-sw"> Pause</label>
    <label class="grp" id="phys-sw-wrap" style="cursor:pointer;" data-doc="phys-sw-wrap"><input type="checkbox" class="sw" id="phys-sw"> Physics</label>
    <label class="grp" style="cursor:pointer;" data-doc="jam-sw-cell"><input type="checkbox" class="sw" id="jam-sw"> Jam</label>
  </div>
</div>
</div>
<div class="sect" id="params-module"><div class="sect-hdr" data-doc="params">Parameters</div><div id="params" class="row"></div></div>
<div class="sect" id="layers-module"><div class="sect-hdr" data-doc="layers">Layers · Colors</div>
<!-- P-units (rackbank_js.go), made by buildPUnitModule, and the four color
     knobs, which are a part of their own (hue and level). The switches the
     P-units' buttons set are kept below, hidden. -->
<div class="punit-grid catgrid" id="layers-bank">
  <div class="punit" id="bg-cell" data-param="bg-visual" data-doc="bg-cell" style="grid-row:1;grid-column:1"><span class="punit-top"><span class="plabel">behind</span></span><select id="bg-visual" data-doc="bg-visual" style="display:none"><option value="" data-doc="bg-visual=off">off</option><option value="spectrogram" data-doc="bg-visual=spectrogram">spectrogram</option><option value="xy" data-doc="bg-visual=xy">xy scope</option><option value="terminal" data-doc="bg-visual=terminal">terminal</option><option value="termanim" data-doc="bg-visual=termanim">animation</option><option value="desk" data-doc="bg-visual=desk">desk</option><option value="water" data-doc="bg-visual=water">water (lens)</option></select><button class="rst" id="rst-bg-visual" data-doc="rst-bg-visual">↺</button></div>
  <div class="punit" id="skin-cell" data-param="skin-visual" data-doc="skin-cell" style="grid-row:2;grid-column:1"><span class="punit-top"><span class="plabel">skin</span></span><select id="skin-visual" data-doc="skin-visual" style="display:none"><option value="" data-doc="skin-visual=off">off</option><option value="spectrogram" data-doc="skin-visual=spectrogram">spectrogram</option><option value="terminal" data-doc="skin-visual=terminal">terminal</option><option value="desk" data-doc="skin-visual=desk">desk</option></select><button class="rst" id="rst-skin-visual" data-doc="rst-skin-visual">↺</button></div>
  <span class="pcell axcol vmcell pal-cell" id="grp-cbg" style="grid-row:3;grid-column:1" data-doc="grp-cbg"><span class="punit-top"><span class="plabel">bg</span><span class="cswatch"><input type="color" id="color-bg" value="#000000" data-doc="color-bg"><button class="rst" id="rst-color-bg" data-doc="rst-color-bg">↺</button></span></span><span class="ckholder" id="ck-color-bg"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cstart" style="grid-row:1;grid-column:2" data-doc="grp-cstart"><span class="punit-top"><span class="plabel" id="lbl-cstart">start</span><span class="cswatch"><input type="color" id="color-base" value="#ff0000" data-doc="color-base"><button class="rst" id="rst-color-base" data-doc="rst-color-base">↺</button></span></span><span class="ckholder" id="ck-color-base"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cmid" style="grid-row:2;grid-column:2" data-doc="grp-cmid"><span class="punit-top"><span class="plabel">mid</span><span class="cswatch"><input type="color" id="color-mid" value="#00ff00" data-doc="color-mid"><button class="rst" id="rst-color-mid" data-doc="rst-color-mid">↺</button></span></span><span class="ckholder" id="ck-color-mid"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cend" style="grid-row:3;grid-column:2" data-doc="grp-cend"><span class="punit-top"><span class="plabel">end</span><span class="cswatch"><input type="color" id="color-top" value="#0000ff" data-doc="color-top"><button class="rst" id="rst-color-top" data-doc="rst-color-top">↺</button></span></span><span class="ckholder" id="ck-color-top"></span></span>
  <div class="punit" id="src-cell" data-param="gradient-source" data-doc="src-cell" style="grid-row:1;grid-column:3"><span class="punit-top"><span class="plabel">src</span></span><select id="gradient-source" data-doc="gradient-source" style="display:none">
    <option value="5" data-doc="gradient-source=5">off</option>
    <option value="0" data-doc="gradient-source=0">X</option>
    <option value="1" data-doc="gradient-source=1">Y</option>
    <option value="2" selected data-doc="gradient-source=2">Z</option>
    <option value="3" data-doc="gradient-source=3">trail</option><option value="4" data-doc="gradient-source=4">audio</option><option value="6" data-doc="gradient-source=6">level</option><option value="11" data-doc="gradient-source=11">dB</option><option value="7" data-doc="gradient-source=7">corr</option><option value="8" data-doc="gradient-source=8">side</option><option value="9" data-doc="gradient-source=9">bal</option><option value="13" data-doc="gradient-source=13">pos</option><option value="10" data-doc="gradient-source=10">flux</option><option value="12" data-doc="gradient-source=12">pitch</option>
  </select><button class="rst" id="rst-gradient-source" data-doc="rst-gradient-source">↺</button></div>
  <div class="punit" id="map-cell" data-param="gradient-colors" data-doc="map-cell" style="grid-row:2;grid-column:3"><span class="punit-top"><span class="plabel">map</span></span><select id="gradient-colors" data-doc="gradient-colors" style="display:none">
    <option value="2" selected data-doc="gradient-colors=2">2-color</option>
    <option value="3" data-doc="gradient-colors=3">3-color</option>
    <option value="4" data-doc="gradient-colors=4">hue sweep</option><option value="5" data-doc="gradient-colors=5">heat</option><option value="6" data-doc="gradient-colors=6">blue</option><option value="7" data-doc="gradient-colors=7">gray</option><option value="8" data-doc="gradient-colors=8">turbo</option><option value="9" data-doc="gradient-colors=9">viridis</option><option value="10" data-doc="gradient-colors=10">magma</option>
  </select><button class="rst" id="rst-gradient-colors" data-doc="rst-gradient-colors">↺</button></div>
  <div class="punit" id="grp-rainbow" data-param="rainbow-freq" data-stops data-doc="grp-rainbow" style="grid-row:3;grid-column:3"><span class="punit-top"><span class="u-lbl">period</span><input type="text" inputmode="decimal" id="slider-value-rfreq" class="numin u-val" data-doc="slider-value-rfreq" value="1.00"></span><input type="range" id="rainbow-freq" min="0.05" max="20" value="1" step="0.05" data-doc="rainbow-freq"><button class="rst" id="rst-rfreq" data-doc="rst-rfreq">↺</button></div>
  <div class="punit" id="grp-pshift" data-param="palette-shift" data-stops data-doc="grp-pshift" style="grid-row:1;grid-column:4"><span class="punit-top"><span class="u-lbl">shift</span><input type="text" inputmode="decimal" id="slider-value-pshift" class="numin u-val" data-doc="slider-value-pshift" value="0.00"></span><input type="range" id="palette-shift" min="-1" max="1" value="0" step="0.01" data-doc="palette-shift"><button class="rst" id="rst-pshift" data-doc="rst-pshift">↺</button></div>
</div>
<span hidden><input type="checkbox" class="sw" id="spect-fill" data-doc="spect-fill-cell"><input type="checkbox" class="sw" id="gradient-reverse" data-doc="gradient-reverse-cell"><input type="checkbox" class="sw" id="color-lock-sw" data-doc="color-lock-sw-cell"><input type="checkbox" class="sw" id="edit-back" data-doc="edit-back-cell"><select id="color-lock" data-doc="color-lock" style="display:none">
    <option value="0" selected data-doc="color-lock=0">auto</option>
    <option value="1" data-doc="color-lock=1">held</option>
    </select></span>
</div>
<div id="model-parts" style="display:none">
<div class="punit" id="speed-cell" data-param="speed-slider" data-stops data-doc="speed-cell"><span class="punit-top"><span class="u-lbl">speed</span><input type="text" inputmode="decimal" id="slider-value-speed" class="numin u-val" data-doc="slider-value-speed" value="1.00"></span><input type="range" id="speed-slider" min="-2" max="2" value="0" step="0.1" data-doc="speed-slider"><button class="rst" id="rst-speed" data-doc="rst-speed">↺</button></div>
<span class="mro" data-for="*" data-doc="mro-*"><span class="mro-lbl">lyap</span><span class="mrodmd" data-chars="8" id="lyap-led" data-doc="lyap-led">--.--</span><span class="mrodmd" data-chars="8" id="lyap-verdict" data-doc="lyap-verdict"></span><button class="rst" id="lyap-remeasure" data-doc="lyap-remeasure">↻</button></span>
<span class="mro" data-for="scopetext" data-doc="mro-scopetext"><span class="mro-lbl">text</span><input type="text" id="stext-in" class="stext-in" maxlength="24" data-no-drag data-doc="stext-in"></span>
<span class="mro" data-for="pong" data-doc="mro-pong"><span class="mro-lbl">score</span><span class="mrodmd" data-chars="4" id="pong-score-l" data-doc="pong-score-l">0</span><span class="mro-sep">:</span><span class="mrodmd" data-chars="4" id="pong-score-r" data-doc="pong-score-r">0</span></span>
<span class="mro" data-for="sprottmorph" data-doc="mro-sprottmorph"><span class="mro-lbl">wired</span><span class="mrodmd" data-chars="8" id="smorph-led" data-doc="smorph-led">D</span></span>
<span class="mro" data-for="bounceball" data-doc="mro-bounceball"><span class="mro-lbl">kicks</span><span class="mrodmd" data-chars="4" id="bounce-kicks" data-doc="bounce-kicks">0</span></span>
<span class="mro" data-for="stlfile" data-doc="mro-stlfile"><span class="mro-lbl">file</span><span class="mrodmd" data-chars="8" id="stlfile-led" data-doc="stlfile-led">none</span></span>
<div class="punit" id="desk-style-cell" data-bank-of="desk" data-doc="desk-style-cell"><span class="punit-top"><span class="plabel">style</span></span><select id="desk-style" data-doc="desk-style" style="display:none"></select><button class="rst" id="rst-desk-style" data-doc="rst-desk-style">↺</button></div>
<div class="punit" id="termanim-cell" data-bank-of="termanim" data-doc="termanim-cell"><span class="punit-top"><span class="plabel">prog</span></span><select id="termanim-pick" data-doc="termanim-pick" style="display:none"></select></div>
<div class="punit" id="bif-sweep-cell" data-bank-of="bifurcation" data-doc="bif-sweep-cell"><span class="punit-top"><span class="plabel">sweep</span></span><select id="bif-sweep" data-doc="bif-sweep" style="display:none"></select></div>
<div class="punit" id="bif-drive-cell" data-bank-of="bifurcation" data-doc="bif-drive-cell"><span class="punit-top"><span class="plabel">drive</span></span><select id="bif-drive" data-doc="bif-drive" style="display:none"><option value="0" selected>sweep</option><option value="1">audio</option></select><button class="rst" id="rst-bif-drive" data-doc="rst-bif-drive">↺</button></div>
<div class="punit" id="fvf-wave-cell" data-bank-of="fvf" data-doc="fvf-wave-cell"><span class="punit-top"><span class="plabel">wave</span></span><select id="fvf-wave" data-doc="fvf-wave" style="display:none"><option value="0">square</option><option value="1" selected>pulse</option><option value="2">sub/2</option></select><button class="rst" id="rst-fvf-wave" data-doc="rst-fvf-wave">↺</button></div>
<div class="punit" id="fvf-mod-cell" data-bank-of="fvf" data-doc="fvf-mod-cell"><span class="punit-top"><span class="plabel">mod</span></span><select id="fvf-mod" data-doc="fvf-mod" style="display:none"><option value="0" selected>ring</option><option value="1">AM</option></select><button class="rst" id="rst-fvf-mod" data-doc="rst-fvf-mod">↺</button></div>
<div class="punit" id="stlfile-builtin-cell" data-bank-of="stlfile" data-doc="stlfile-builtin-cell"><span class="punit-top"><span class="plabel">solid</span></span><select id="stlfile-builtin" data-doc="stlfile-builtin" style="display:none"><option value="" data-doc="stlfile-builtin=off">file</option></select></div>
<input type="checkbox" class="sw" id="desk-pass" data-doc="desk-pass">
<button class="pushbtn" id="pong-restart" data-doc="pong-restart"></button>
<button class="pushbtn" id="bounce-drop" data-doc="bounce-drop"></button>
<button class="pushbtn" id="stlfile-load" data-doc="stlfile-load"></button>
</div>
<div class="sect" id="spectro-module" style="display:none"><div class="sect-hdr" data-doc="spectro">Spectro</div>
<div id="spectro-params" class="row"></div></div>
<div class="sect" id="record-module"><div class="sect-hdr" data-doc="record">Record</div>
<div class="recgrid">
  <div class="rec-screen-unit" data-no-drag>
    <span class="monbezel" data-doc="rec-preview-cell"><canvas id="rec-preview" width="244" height="230"></canvas><span id="rec-tally" class="rec-tally"></span></span>
  <label class="grp" style="cursor:pointer;" data-doc="rec-mon-on-cell"><input type="checkbox" class="sw" id="rec-mon-on" checked> Monitor</label>
  </div>
  <div class="rec-controls">
    <div class="rec-unit" data-doc="rec-format">
      <span class="u-lbl">format</span>
      <span class="rec-swrow" data-doc="rec-gif-sw"><span class="rec-legend">webm</span><input type="checkbox" class="sw" id="rec-gif-sw"><span class="rec-legend">gif</span></span>
    </div>
    <div class="rec-unit" data-doc="rec-area">
      <span class="u-lbl">area</span>
      <span class="rec-swrow" data-doc="rec-region-sw"><span class="rec-legend">full</span><input type="checkbox" class="sw" id="rec-region-sw"><span class="rec-legend">region</span></span>
    </div>
    <div class="rec-unit">
      <span class="u-lbl">transport</span>
      <span class="grp rec-btns">
        <button class="recbtn recbtn-rec" id="rec-btn" data-doc="rec-btn"></button>
        <button class="recbtn recbtn-stop" id="rec-stop-btn" data-doc="rec-stop-btn"></button>
        <button class="recbtn recbtn-still" id="screenshot-btn" data-doc="screenshot-btn"></button>
      </span>
    </div>
    <div class="rec-unit" data-doc="rec-meter-fill-cell">
      <span class="u-lbl">media</span>
      <span class="rec-meter"><span class="rec-meter-fill" id="rec-meter-fill"></span></span>
      <span class="rec-counter" id="rec-status">--:--:--</span>
    </div>
    <div class="rec-unit rec-log" data-doc="rec-log-cell">
      <span class="u-lbl">last take</span>
      <span class="rec-logline" id="rec-log">no takes yet</span>
    </div>
  </div>
</div>
<input type="checkbox" id="rec-sw" class="rec-hidden-state">
</div>
<div class="sect"><div class="sect-hdr" data-doc="view">View</div>
<div class="row vmrow vmaxrow">
  <span class="pcell axcol axrot" data-no-drag data-doc="knob-x-cell">
    <span class="plabel">X</span>
    <span class="grp"><span class="knob" id="knob-x" data-doc="knob-x"><i class="knob-ptr" id="knobptr-x"></i></span><span class="led" id="led-x" data-doc="led-x">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-x" min="-1" max="1" value="0" step="0.1" data-doc="rotation-controls-x"><input type="number" id="slider-value-x" class="numin" data-doc="slider-value-x" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rx" data-doc="rst-rx">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag data-doc="knob-y-cell">
    <span class="plabel">Y</span>
    <span class="grp"><span class="knob" id="knob-y" data-doc="knob-y"><i class="knob-ptr" id="knobptr-y"></i></span><span class="led" id="led-y" data-doc="led-y">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-y" min="-1" max="1" value="0" step="0.1" data-doc="rotation-controls-y"><input type="number" id="slider-value-y" class="numin" data-doc="slider-value-y" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-ry" data-doc="rst-ry">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag data-doc="knob-z-cell">
    <span class="plabel">Z</span>
    <span class="grp"><span class="knob" id="knob-z" data-doc="knob-z"><i class="knob-ptr" id="knobptr-z"></i></span><span class="led" id="led-z" data-doc="led-z">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-z" min="-1" max="1" value="0" step="0.1" data-doc="rotation-controls-z"><input type="number" id="slider-value-z" class="numin" data-doc="slider-value-z" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rz" data-doc="rst-rz">↺</button></span>
  </span>
  <span class="pcell axcol vmcell" data-no-drag data-doc="slider-value-panx-cell"><span class="punit-top"><span class="plabel">X</span><input type="number" id="slider-value-panx" class="numin" data-doc="slider-value-panx" min="-180" max="180" step="5" value="0"></span><input type="range" id="pan-x" min="-8" max="8" value="0" step="0.2222222222222222" data-doc="pan-x"><button class="rst" id="rst-panx" data-doc="rst-panx">↺</button></span>
  <span class="pcell axcol vmcell" data-no-drag data-doc="slider-value-pany-cell"><span class="punit-top"><span class="plabel">Y</span><input type="number" id="slider-value-pany" class="numin" data-doc="slider-value-pany" min="-180" max="180" step="5" value="0"></span><input type="range" id="pan-y" min="-8" max="8" value="0" step="0.2222222222222222" data-doc="pan-y"><button class="rst" id="rst-pany" data-doc="rst-pany">↺</button></span>
  <span class="pcell axcol vmcell" data-no-drag data-doc="slider-value-zoom-cell"><span class="punit-top"><span class="plabel">Zoom</span><input type="number" id="slider-value-zoom" class="numin" data-doc="slider-value-zoom" min="-180" max="180" step="0.5" value="0"></span><input type="range" id="camera-zoom" min="-95" max="95" value="0" step="0.25" data-doc="camera-zoom"><input type="number" id="slider-value-fore" class="numin" data-doc="slider-value-fore" min="-1" max="1" step="0.05" value="-1"><input type="range" id="model-fore" min="-1" max="1" value="-1" step="0.05" data-doc="model-fore"><button class="rst" id="rst-zoom" data-doc="rst-zoom">↺</button></span>
</div>
</div>
<div class="sect" id="grid-module"><div class="sect-hdr" data-doc="grid">Grid</div>
<!-- Six P-units (rackbank_js.go), the part every position of a model bank is:
     built from this by buildGridBank. Each selector's knob is mounted in its
     .knobhold, which the dial's own rebuild refills. -->
<div class="punit-grid catgrid" id="grid-bank">
  <div class="punit" id="view-n-cell" data-param="view-n" data-doc="view-n-stack-cell" style="grid-row:1;grid-column:1"><span class="punit-top"><span class="plabel">grid</span></span><select id="view-n" style="display:none">
      <option value="0" selected data-doc="view-n=0">1</option>
      <option value="1" data-doc="view-n=1">2</option>
      <option value="2" data-doc="view-n=2">4</option>
      <option value="3" data-doc="view-n=3">9</option>
      <option value="4" data-doc="view-n=4">16</option>
    </select><span class="knobhold" id="view-n-stack"></span><input type="checkbox" id="grid-ovl" hidden data-doc="grid-ovl"></div>
  <div class="punit" id="sweep-p-cell" data-param="sweep-p" data-doc="sweep-p-stack-cell" style="grid-row:2;grid-column:1"><span class="punit-top"><span class="plabel">sweep</span></span><select id="sweep-p" style="display:none">
      <!-- Filled by buildSweepDial: the targets are the CURRENT model's own parameters, so this list changes with the mode. -->
      <option value="0" selected data-doc="sweep-p=0">—</option>
    </select><span class="knobhold" id="sweep-p-stack"></span></div>
  <div class="punit" id="sweep2-p-cell" data-param="sweep2-p" data-doc="sweep2-p-stack-cell" style="grid-row:3;grid-column:1"><span class="punit-top"><span class="plabel">down</span></span><select id="sweep2-p" style="display:none">
      <!-- Filled by buildSweepDial from the same per-mode target lists as sweep-p. -->
      <option value="0" selected data-doc="sweep2-p=0">—</option>
    </select><span class="knobhold" id="sweep2-p-stack"></span></div>
  <div class="punit" id="sweep-lo-cell" data-param="sweep-lo" data-stops data-doc="sweep-lo-cell" style="grid-row:1;grid-column:2"><span class="punit-top"><span class="u-lbl">from</span><input type="text" inputmode="decimal" id="slider-value-swlo" class="numin u-val" data-doc="slider-value-swlo" value="0.00"></span><input type="range" id="sweep-lo" min="0" max="1" value="0" step="0.01" data-doc="sweep-lo"><button class="rst" id="rst-swlo" data-doc="rst-swlo">↺</button></div>
  <div class="punit" id="sweep-hi-cell" data-param="sweep-hi" data-stops data-doc="sweep-hi-cell" style="grid-row:2;grid-column:2"><span class="punit-top"><span class="u-lbl">to</span><input type="text" inputmode="decimal" id="slider-value-swhi" class="numin u-val" data-doc="slider-value-swhi" value="1.00"></span><input type="range" id="sweep-hi" min="0" max="1" value="1" step="0.01" data-doc="sweep-hi"><button class="rst" id="rst-swhi" data-doc="rst-swhi">↺</button></div>
  <div class="punit" id="focus-n-cell" data-param="focus-n" data-doc="focus-n-cell" style="grid-row:3;grid-column:2"><span class="punit-top"><span class="plabel">focus</span></span><select id="focus-n" style="display:none">
      <!-- Filled by buildFocusDial: one position per cell of the CURRENT grid. -->
      <option value="0" selected data-doc="focus-n=0">A</option>
    </select><span class="knobhold" id="focus-n-stack"></span><input type="checkbox" id="link-sw" checked hidden data-doc="link-sw-cell"></div>
</div>
</div>
<div class="sect" id="display-module"><div class="sect-hdr" data-doc="display">Display</div>
<!-- P-units (rackbank_js.go), made by buildPUnitModule. The switches their
     buttons set are kept below, hidden: each is still one control to the
     permalink and Reset All, and its buttons light from it. -->
<div class="punit-grid catgrid" id="display-bank">
  <div class="punit" id="line-cell" data-param="line-width" data-stops data-doc="line-cell" style="grid-row:1;grid-column:1"><span class="punit-top"><span class="u-lbl">line</span><input type="text" inputmode="decimal" id="slider-value-line" class="numin u-val" data-doc="slider-value-line" value="1"></span><input type="range" id="line-width" min="1" max="10" value="1" step="1" data-doc="line-width"><button class="rst" id="rst-line" data-doc="rst-line">↺</button></div>
  <div class="punit" id="points-cell" data-param="dash-duty" data-stops data-doc="points-cell" style="grid-row:2;grid-column:1"><span class="punit-top"><span class="u-lbl">points</span><input type="text" inputmode="decimal" id="slider-value-dash" class="numin u-val" data-doc="slider-value-dash" value="0"></span><input type="range" id="dash-duty" min="0" max="4000" value="0" step="10" data-doc="dash-duty"><button class="rst" id="rst-dash" data-doc="rst-dash">↺</button></div>
  <div class="punit" id="trail-controls" data-param="trail-slider" data-stops data-doc="trail-controls" style="grid-row:3;grid-column:1"><span class="punit-top"><span class="u-lbl">trail</span><input type="text" inputmode="decimal" id="slider-value-trail" class="numin u-val" data-doc="slider-value-trail" value="20000"></span><input type="range" id="trail-slider" min="1000" max="500000" value="20000" step="1000" data-doc="trail-slider"><button class="rst" id="rst-trail" data-doc="rst-trail">↺</button></div>
</div>
<span hidden><input type="checkbox" class="sw" id="use-points" data-doc="use-points"><input type="checkbox" class="sw" id="persist-trail" data-doc="persist-trail-cell"><input type="checkbox" class="sw" id="ring-sw" data-doc="ring-sw-cell"><input type="checkbox" class="sw" id="sect-sw" data-doc="sect-sw-cell"><input type="checkbox" class="sw" id="scope-grat" checked data-doc="scope-grat-cell"></span>
</div>
<div class="sect"><div class="sect-hdr" data-doc="style">Style</div>
<div class="row vmrow">
  <span class="pcell axcol rstcell" data-doc="knobstyle-stack-cell"><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Knob</span><span id="knobstyle-stack"></span></span></span><button class="rst" id="rst-knob-style" data-doc="rst-knob-style">↺</button><select id="knob-style" data-doc="knob-style" style="display:none"><option value="std" selected data-doc="knob-style=std">std</option><option value="flat" data-doc="knob-style=flat">flat</option><option value="vint" data-doc="knob-style=vint">vint</option><option value="chrome" data-doc="knob-style=chrome">chrome</option><option value="gold" data-doc="knob-style=gold">gold</option><option value="carbon" data-doc="knob-style=carbon">carbon</option></select></span>
  <span class="pcell axcol rstcell" id="ledcolor-cell" data-doc="ledcolor-cell"><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">LED</span><span id="ledcolor-stack"></span></span></span><button class="rst" id="rst-led-color" data-doc="rst-led-color">↺</button><select id="led-color" data-doc="led-color" style="display:none"></select></span>
  <span class="pcell axcol rstcell" id="phosphor-cell" data-doc="phosphor-cell"><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Phosphor</span><span id="phosphor-stack"></span></span></span><button class="rst" id="rst-phosphor" data-doc="rst-phosphor">↺</button><select id="phosphor" data-doc="phosphor" style="display:none"></select></span>
</div>
</div>
<!-- The scope is two modules at the head of the GENERATORS bay. The first is
     the tube, in the top row of its two slots with the SOURCE buttons at its
     right, and under it the four knobs that set how the beam looks and where
     it sits: SCALE ILLUM round INTENSITY on one shaft, FOCUS, and the two
     POSITIONs, in markup order column by column. The second is the ranges
     and the trigger, one column of three. -->
<div class="sect" id="scope-module"><div class="sect-hdr" data-doc="scope">Scope 1</div>
<div class="row vmrow scoperow">
  <span class="scope-tube" data-doc="scope-screen-cell"><canvas id="scope-screen" width="563" height="384"></canvas><span class="scope-glass"></span></span>
  <span class="scope-side"><span class="scope-mini" data-cap="H" data-doc="rst-scope-hpos-cell"><button class="rst" id="rst-scope-hpos" data-doc="rst-scope-hpos">↺</button><input type="range" id="scope-hpos" min="-5" max="5" step="0.05" value="0" style="display:none"><span id="scope-hpos-stack"></span></span><span class="scope-chan-slot" id="scope-chan-slot"></span><span class="scope-mini" data-cap="V" data-doc="rst-scope-vpos-cell"><button class="rst" id="rst-scope-vpos" data-doc="rst-scope-vpos">↺</button><input type="range" id="scope-vpos" min="-4" max="4" step="0.05" value="0" style="display:none"><span id="scope-vpos-stack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell sc-beam" data-doc="scope-intens-led-cell"><span class="punit-top"><span class="plabel">inten</span><input type="number" class="numin" id="scope-intens-led" data-doc="scope-intens-led" min="0" max="1" step="0.01" value="0.6"></span><input type="range" id="scope-illum" min="-0.2" max="1" step="0.01" value="0.5" data-doc="scope-illum" style="display:none"><input type="range" id="scope-intens" min="0" max="1" step="0.01" value="0.6" data-doc="scope-intens" style="display:none"><input type="range" id="scope-focus" min="0" max="1" step="0.01" value="0.5" data-doc="scope-focus" style="display:none"><span class="grp vmbay"><span id="scope-intens-stack"></span></span><button class="rst" id="rst-scope-intens" data-doc="rst-scope-intens">↺</button></span>
  <span class="pcell axcol vmcell gen-cell scopetrio sc-trig" id="scope-trig-cell" data-doc="scope-trig-cell"><span class="punit-top"><span class="plabel">trig</span><input type="number" class="numin" id="scope-trig-led" data-doc="scope-trig-led" min="-1" max="1" step="0.01" value="0"></span><input type="range" id="scope-trig" min="-1" max="1" step="0.01" value="0" style="display:none"><span class="grp vmbay"><span id="scope-trig-stack"></span></span><button class="rst" id="rst-scope-trig" data-doc="rst-scope-trig">↺</button></span>
  <span class="pcell axcol vmcell gen-cell sc-volts" data-doc="scope-volts-read-cell"><span class="punit-top"><span class="plabel">volts</span><span id="scope-volts-read"></span></span><select id="scope-volts" style="display:none"></select><input type="range" id="scope-in1" min="0" max="10" step="1" value="0" data-doc="scope-in1" style="display:none"><input type="range" id="scope-in2" min="0" max="10" step="1" value="1" data-doc="scope-in2" style="display:none"><span class="grp vmbay"><span id="scope-volts-stack"></span></span><button class="rst" id="rst-scope-volts" data-doc="rst-scope-volts">↺</button></span>
  <span class="pcell axcol vmcell gen-cell sc-time" data-doc="scope-time-read-cell"><span class="punit-top"><span class="plabel">time</span><span id="scope-time-read"></span></span><select id="scope-time" style="display:none"></select><span class="grp vmbay"><span id="scope-time-stack"></span></span><button class="rst" id="rst-scope-time" data-doc="rst-scope-time">↺</button></span>
</div></div>
<div class="sect gen-osc" id="gen-v-module"><div class="sect-hdr" data-doc="gen-v">Gen 4</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin" id="gen-v-led" data-doc="gen-v-led" min="27.5" max="28160" step="1" value="247"></span><input type="range" id="gen-v-freq" min="0" max="120" step="1" value="38" style="display:none"><span class="grp vmbay"><span id="gen-v-fstack"></span></span><button class="rst" id="rst-gen-v-freq" data-doc="rst-gen-v-freq">↺</button></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin" id="gen-v-lvl-led" data-doc="gen-v-lvl-led" min="0" max="100" step="1" value="0"></span><input type="range" id="gen-v-lvl" min="0" max="100" step="1" value="0" style="display:none"><input type="range" id="gen-v-atk" min="1" max="2000" step="1" value="10" data-doc="gen-v-atk" style="display:none"><input type="range" id="gen-v-dcy" min="1" max="5000" step="1" value="300" data-doc="gen-v-dcy" style="display:none"><select id="gen-v-env" data-doc="gen-v-env" style="display:none"><option value="off" selected>off</option><option value="rpt">rpt</option></select><span class="grp vmbay"><span id="gen-v-lstack"></span></span><button class="rst" id="rst-gen-v-lvl" data-doc="rst-gen-v-lvl">↺</button></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-v-ostack"></span></span><select id="gen-v-wave" data-endless data-doc="gen-v-wave" style="display:none"></select></span>
</div></div>
<div class="sect gen-osc" id="gen-x-module"><div class="sect-hdr" data-doc="gen-x">Gen 1</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin" id="gen-x-led" data-doc="gen-x-led" min="27.5" max="28160" step="1" value="200"></span><input type="range" id="gen-x-freq" min="0" max="120" step="1" value="34" style="display:none"><span class="grp vmbay"><span id="gen-x-fstack"></span></span><button class="rst" id="rst-gen-x-freq" data-doc="rst-gen-x-freq">↺</button></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin" id="gen-x-lvl-led" data-doc="gen-x-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-x-lvl" min="0" max="100" step="1" value="80" style="display:none"><input type="range" id="gen-x-atk" min="1" max="2000" step="1" value="10" data-doc="gen-x-atk" style="display:none"><input type="range" id="gen-x-dcy" min="1" max="5000" step="1" value="300" data-doc="gen-x-dcy" style="display:none"><select id="gen-x-env" data-doc="gen-x-env" style="display:none"><option value="off" selected>off</option><option value="rpt">rpt</option></select><span class="grp vmbay"><span id="gen-x-lstack"></span></span><button class="rst" id="rst-gen-x-lvl" data-doc="rst-gen-x-lvl">↺</button></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-x-ostack"></span></span><select id="gen-x-wave" data-endless data-doc="gen-x-wave" style="display:none"></select></span>
</div></div>
<div class="sect gen-osc" id="gen-y-module"><div class="sect-hdr" data-doc="gen-y">Gen 2</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin" id="gen-y-led" data-doc="gen-y-led" min="27.5" max="28160" step="1" value="300"></span><input type="range" id="gen-y-freq" min="0" max="120" step="1" value="41" style="display:none"><span class="grp vmbay"><span id="gen-y-fstack"></span></span><button class="rst" id="rst-gen-y-freq" data-doc="rst-gen-y-freq">↺</button></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin" id="gen-y-lvl-led" data-doc="gen-y-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-y-lvl" min="0" max="100" step="1" value="80" style="display:none"><input type="range" id="gen-y-atk" min="1" max="2000" step="1" value="10" data-doc="gen-y-atk" style="display:none"><input type="range" id="gen-y-dcy" min="1" max="5000" step="1" value="300" data-doc="gen-y-dcy" style="display:none"><select id="gen-y-env" data-doc="gen-y-env" style="display:none"><option value="off" selected>off</option><option value="rpt">rpt</option></select><span class="grp vmbay"><span id="gen-y-lstack"></span></span><button class="rst" id="rst-gen-y-lvl" data-doc="rst-gen-y-lvl">↺</button></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-y-ostack"></span></span><select id="gen-y-wave" data-endless data-doc="gen-y-wave" style="display:none"></select></span>
</div></div>
<div class="sect gen-osc" id="gen-z-module"><div class="sect-hdr" data-doc="gen-z">Gen 3</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin" id="gen-z-led" data-doc="gen-z-led" min="27.5" max="28160" step="1" value="150"></span><input type="range" id="gen-z-freq" min="0" max="120" step="1" value="29" style="display:none"><span class="grp vmbay"><span id="gen-z-fstack"></span></span><button class="rst" id="rst-gen-z-freq" data-doc="rst-gen-z-freq">↺</button></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin" id="gen-z-lvl-led" data-doc="gen-z-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-z-lvl" min="0" max="100" step="1" value="80" style="display:none"><input type="range" id="gen-z-atk" min="1" max="2000" step="1" value="10" data-doc="gen-z-atk" style="display:none"><input type="range" id="gen-z-dcy" min="1" max="5000" step="1" value="300" data-doc="gen-z-dcy" style="display:none"><select id="gen-z-env" data-doc="gen-z-env" style="display:none"><option value="off" selected>off</option><option value="rpt">rpt</option></select><span class="grp vmbay"><span id="gen-z-lstack"></span></span><button class="rst" id="rst-gen-z-lvl" data-doc="rst-gen-z-lvl">↺</button></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-z-ostack"></span></span><select id="gen-z-wave" data-endless data-doc="gen-z-wave" style="display:none"></select></span>
</div></div>
<div class="sect gen-osc" id="preset-module"><div class="sect-hdr" data-doc="preset">Presets</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">name</span></span><input type="text" id="preset-name" class="stext-in preset-in" maxlength="24" data-no-drag data-doc="preset-name"><span class="grp btn-row demo-btn"><button class="pushbtn" id="preset-save" data-doc="preset-save"></button><span class="btn-lbl">Save</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">saved</span></span><span class="grp vmbay"><select id="preset-list" class="stl-builtin preset-list" data-no-drag data-doc="preset-list"><option value="">— none saved —</option></select></span><span class="grp btn-row demo-btn"><button class="pushbtn" id="preset-recall" data-doc="preset-recall"></button><span class="btn-lbl">Recall</span></span><span class="grp btn-row demo-btn"><button class="pushbtn" id="preset-del" data-doc="preset-del"></button><span class="btn-lbl">Delete</span></span></span>
</div></div>
<div class="sect gen-osc" id="counter-module"><div class="sect-hdr" data-doc="counter">Counter</div>
<div class="row vmrow meterrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><span class="led" id="counter-led" data-doc="counter-led">-----.-</span></span><span class="grp vmbay"><span class="counter-gate" id="counter-gate" data-doc="counter-gate"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">gate</span></span><span class="grp vmbay"><span id="counter-gstack"></span></span><button class="rst" id="rst-counter-gate" data-doc="rst-counter-gate">↺</button><select id="counter-gatesel" data-doc="counter-gatesel" style="display:none"><option value="0.1" data-doc="counter-gatesel=0.1">0.1</option><option value="0.5" data-doc="counter-gatesel=0.5">0.5</option><option value="1" selected data-doc="counter-gatesel=1">1</option><option value="2" data-doc="counter-gatesel=2">2</option></select></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">trig</span><input type="number" class="numin" id="counter-trig-led" data-doc="counter-trig-led" min="0" max="30" step="1" value="4"></span><input type="range" id="counter-trig" min="0" max="30" step="1" value="4" style="display:none"><span class="grp vmbay"><span id="counter-tstack"></span></span><button class="rst" id="rst-counter-trig" data-doc="rst-counter-trig">↺</button></span>
</div></div>
<div class="sect gen-osc" id="thd-module"><div class="sect-hdr" data-doc="thd">Distortion</div>
<div class="row vmrow meterrow">
  <span class="pcell axcol vmcell gen-cell readcol"><span class="grp vmbay"><span class="ledlbl">thd</span><span class="led" id="thd-led" data-doc="thd-led">  --.---</span><span class="ledlbl">fund</span><span class="led" id="thd-fund-led" data-doc="thd-fund-led">-----.-</span></span>
    <span class="grp vmbay"><span class="ledlbl">thd+n</span><span class="led" id="thdn-led" data-doc="thdn-led">  --.---</span><span class="ledlbl">lvl</span><span class="led" id="thd-level-led" data-doc="thd-level-led">  --.-</span></span>
    <span class="grp vmbay"><span class="ledlbl">sinad</span><span class="led" id="thd-sinad-led" data-doc="thd-sinad-led">  --.-</span><span class="ledlbl">enob</span><span class="led" id="thd-enob-led" data-doc="thd-enob-led">--.--</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">src</span></span><span class="grp vmbay"><span id="thd-chanstack"></span></span><button class="rst" id="rst-thd-chan" data-doc="rst-thd-chan">↺</button><select id="thd-chan" data-doc="thd-chan" style="display:none"></select></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">harm</span><input type="number" class="numin" id="thd-harm-led" data-doc="thd-harm-led" min="2" max="20" step="1" value="10"></span><input type="range" id="thd-harm" min="2" max="20" step="1" value="10" style="display:none"><span class="grp vmbay"><span id="thd-hstack"></span></span><button class="rst" id="rst-thd-harm" data-doc="rst-thd-harm">↺</button></span>
</div></div>
<div class="sect gen-osc" id="lufs-module"><div class="sect-hdr" data-doc="lufs">Loudness</div>
<div class="row vmrow meterrow timingcol">
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">M</span><span class="led" id="lufs-m-led" data-doc="lufs-m-led">  --.-</span><span class="ledlbl">S</span><span class="led" id="lufs-s-led" data-doc="lufs-s-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">I</span><span class="led" id="lufs-i-led" data-doc="lufs-i-led">  --.-</span><span class="ledlbl">LRA</span><span class="led" id="lufs-lra-led" data-doc="lufs-lra-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">TP</span><span class="led" id="lufs-tp-led" data-doc="lufs-tp-led">  --.-</span><button class="rst" id="lufs-reset" data-doc="lufs-reset">↺</button></span></span>
  <span class="pcell axcol vmcell gen-cell"><input type="range" id="lufs-target" min="-40" max="0" step="1" value="-23" style="display:none"><span class="grp vmbay"><span class="ledlbl">tgt</span><input type="number" class="numin" id="lufs-target-led" data-doc="lufs-target-led" min="-40" max="0" step="1" value="-23"><span class="ledlbl">&#916;</span><span class="led" id="lufs-delta-led" data-doc="lufs-delta-led">  --.-</span></span><button class="rst" id="rst-lufs-target" data-doc="rst-lufs-target">↺</button></span>
  <span class="pcell axcol swcell" data-doc="meters-cell">
    <label class="swline" style="cursor:pointer;" data-doc="show-meters"><input type="checkbox" class="sw" id="show-meters" checked> Meters</label>
  </span>
</div></div>
<div class="sect gen-osc" id="wf-module"><div class="sect-hdr" data-doc="wf">Wow &amp; Flutter</div>
<div class="row vmrow meterrow">
  <span class="pcell axcol vmcell gen-cell readcol"><span class="grp vmbay"><span class="ledlbl">speed</span><span class="led" id="wf-speed-led" data-doc="wf-speed-led">  --.---</span><span class="ledlbl">carr</span><span class="led" id="wf-carrier-led" data-doc="wf-carrier-led">-----.-</span></span>
    <span class="grp vmbay"><span class="ledlbl">wow</span><span class="led" id="wf-wow-led" data-doc="wf-wow-led">  --.---</span><span class="ledlbl">flut</span><span class="led" id="wf-flutter-led" data-doc="wf-flutter-led">  --.---</span></span>
    <span class="grp vmbay"><span class="ledlbl">w&amp;f</span><span class="led" id="wf-weighted-led" data-doc="wf-weighted-led">  --.---</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">nom</span><input type="number" class="numin" id="wf-nom-led" data-doc="wf-nom-led" min="0" max="20000" step="10" value="3150"></span><input type="range" id="wf-nom" min="0" max="20000" step="10" value="3150" style="display:none"><span class="grp vmbay"><span id="wf-nstack"></span></span><button class="rst" id="rst-wf-nom" data-doc="rst-wf-nom">↺</button></span>
</div></div>
<div class="sect gen-osc" id="timing-module"><div class="sect-hdr" data-doc="timing">Timing</div>
<div class="row vmrow meterrow timingcol">
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">fps</span><span class="led" id="tm-fps-led" data-doc="tm-fps-led">  --.-</span><span class="ledlbl">frame</span><span class="led" id="tm-frame-led" data-doc="tm-frame-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">min</span><span class="led" id="tm-min-led" data-doc="tm-min-led">  --.-</span><span class="ledlbl">max</span><span class="led" id="tm-max-led" data-doc="tm-max-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">late</span><span class="led" id="tm-late-led" data-doc="tm-late-led">  --.-</span><span class="ledlbl">rest</span><span class="led" id="tm-rest-led" data-doc="tm-rest-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">model</span><span class="led" id="tm-model-led" data-doc="tm-model-led">  --.-</span><span class="ledlbl">meters</span><span class="led" id="tm-meters-led" data-doc="tm-meters-led">  --.-</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="grp vmbay"><span class="ledlbl">scope</span><span class="led" id="tm-scope-led" data-doc="tm-scope-led">  --.-</span></span></span>
</div></div>
<div class="sect gen-osc keys-sect" id="keys-module"><div class="sect-hdr" data-doc="keys">Keys</div>
<div class="row keysflex">
<div class="keys-bed" id="keys-bed" data-no-drag data-doc="keys-bed"></div>
</div></div>
<div class="sect gen-osc keys-sect" id="tm-module"><div class="sect-hdr" data-doc="tm">Matrix</div>
<div class="row keysflex">
<div class="vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">tempo</span><input type="number" class="numin" id="tm-tempo-led" data-doc="tm-tempo-led" min="40" max="300" step="1" value="120"></span><input type="range" id="tm-tempo" min="40" max="300" step="1" value="120" style="display:none"><span class="grp vmbay"><span id="tm-tstack"></span></span><button class="rst" id="rst-tm-tempo" data-doc="rst-tm-tempo">↺</button></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">steps</span></span><span class="grp vmbay"><span id="tm-sstack"></span></span><button class="rst" id="rst-tm-steps" data-doc="rst-tm-steps">↺</button><select id="tm-steps" data-doc="tm-steps" style="display:none"><option value="12" data-doc="tm-steps=12">12</option><option value="16" selected data-doc="tm-steps=16">16</option><option value="24" data-doc="tm-steps=24">24</option><option value="32" data-doc="tm-steps=32">32</option><option value="48" data-doc="tm-steps=48">48</option><option value="64" data-doc="tm-steps=64">64</option></select><select id="tm-root" data-doc="tm-root" style="display:none"><option value="1" data-doc="tm-root=1">C1</option><option value="2" data-doc="tm-root=2">C2</option><option value="3" selected data-doc="tm-root=3">C3</option><option value="4" data-doc="tm-root=4">C4</option></select></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin" id="tm-lvl-led" data-doc="tm-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="tm-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="tm-lstack"></span></span><button class="rst" id="rst-tm-lvl" data-doc="rst-tm-lvl">↺</button></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">wave</span></span><span class="grp vmbay"><span id="tm-ostack"></span></span><button class="rst" id="rst-tm-wave" data-doc="rst-tm-wave">↺</button><select id="tm-wave" data-doc="tm-wave" style="display:none"><option value="0" selected data-doc="tm-wave=0">sine</option><option value="1" data-doc="tm-wave=1">tri</option><option value="2" data-doc="tm-wave=2">sqr</option><option value="3" data-doc="tm-wave=3">saw</option><option value="4" data-doc="tm-wave=4">noise</option></select></span>
  <span class="pcell axcol vmcell gen-cell"><label class="grp" style="cursor:pointer;" data-doc="tm-run-cell"><input type="checkbox" class="sw" id="tm-run" checked> Run</label><span class="rhythm-beats" id="rhythm-beats" data-doc="rhythm-beats"></span><span class="grp btn-row demo-btn"><button class="pushbtn" id="tm-clear" data-doc="tm-clear"></button><span class="btn-lbl">Clear</span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">drums</span><input type="number" class="numin" id="rhythm-lvl-led" data-doc="rhythm-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="rhythm-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="rhythm-lstack"></span></span><button class="rst" id="rst-rhythm-lvl" data-doc="rst-rhythm-lvl">↺</button></span>
</div>
<div class="tm-body">
<div class="tm-grid" id="tm-grid" data-no-drag data-doc="tm-grid"></div>
<div class="tm-drums" id="tm-drums" data-no-drag data-doc="tm-drums"></div>
<div class="rhythm-tabs" id="rhythm-tabs" data-no-drag data-doc="rhythm-tabs"></div>
<select id="rhythm-preset" style="display:none"></select>
</div>
</div></div>
</div>
<div id="runtime" style="color:#555;font-size:11px;padding-left:40px;"></div>
`

// ── main ─────────────────────────────────────────────────────────────────────

// ledColorDefs is the ORDERED single source for the LED-readout color options.
// It drives the LED-color knob's detents (in sweep order — amber sits between
// red and green), the hidden <select>, the CSS-var color map, and the dial
// dots. Add a color here and it appears everywhere.
var ledColorDefs = []struct{ name, col, glow, bg, bd, desc string }{
	{"red", "#ff3b30", "#ff2a20", "#170000", "#3a0000", doc("led-color=red")},
	{"amber", "#ffb000", "#d08000", "#171000", "#3a2a00", doc("led-color=amber")},
	{"green", "#35e06a", "#1c9a44", "#001709", "#0a3a1a", doc("led-color=green")},
	{"blue", "#3bb0ff", "#1c7ad0", "#001017", "#0a2a3a", doc("led-color=blue")},
	{"cyan", "#2ce0e0", "#159a9a", "#001515", "#0a3838", doc("led-color=cyan")},     // the old audio-mod readout hue
	{"violet", "#b06cff", "#8040d0", "#12001f", "#2a0a3a", doc("led-color=violet")}, // new
}

// shellFurniture is what lives in the shell alongside the panel: the resize bar
// that straddles the panel's inner edge, and the dock/float cluster that clips
// onto the end of it.
//
// Outside the panel, because the panel scrolls and anything inside it is
// clipped by that. Inside the shell, because the shell is a positioning box
// that moves with the panel — so CSS can place these against whichever edge
// the dock is on, and no code has to recompute where they went.
//
// The dock cluster used to be declared inside the panel's Console module and
// rendered position:fixed out of it, which is why the module width quantizer
// still has to skip out-of-flow children.
const shellFurniture = `
<div id="dock-resize" data-doc="dock-resize"></div>
<span class="grp" id="dock-controls" data-doc="dock-controls"><span class="dock-lbl">DOCK</span><button class="ctrl-btn dockb" id="dock-top" data-doc="dock-top">↑</button><button class="ctrl-btn dockb" id="dock-bottom" data-doc="dock-bottom">↓</button><button class="ctrl-btn dockb" id="dock-left" data-doc="dock-left">←</button><button class="ctrl-btn dockb" id="dock-right" data-doc="dock-right">→</button><button class="ctrl-btn dockb" id="dock-float" data-doc="dock-float">⧉</button><button class="ctrl-btn dockb" id="dock-footer" data-doc="dock-footer">▣</button></span>
`
