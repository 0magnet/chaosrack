// Subcommand lint: the faults a person finds by looking at the panel, found by
// measuring it.
//
// Every one of these was first reported by eye, one at a time, with a
// description of where on the panel to look. Each is a measurement, and a
// measurement can be made of every control on every model in a second:
//
//	OVERFLOW     a readout whose text is wider than its box (a digit cut off,
//	             and the text scrolling as the value changes)
//	OVERLAP      two parts of the panel on top of each other: a legend over
//	             a dial's ticks, a reset over a readout, a control over the
//	             one beside it
//	OFFCENTER    a readout not on its knob's center line
//	SEG-LETTERS  a seven-segment readout asked to spell a word
//	TIP-MISSING  a control whose tooltip says nothing but where it is
//	TIP-TERSE    a tooltip that is only the "Module / label / part" stamp,
//	             which never spells out what an abbreviated label means
//	TIP-ADDRESS  an address in the middle of a tooltip, borrowed from another
//	NONUNIFORM   one part of a bank at two sizes
//	DISPLAY-CUT  a character display given more text than it has characters
//	CLIPPED      a part reaching past a box that cuts off what overflows it
//	SLACK        a module a whole slot or more wider than what is on it
//	EMPTY        a module on the rack with nothing visible on it
//
// Each finding names the control by its address, bay.module.position, the same one
// its tooltip starts with (see pkg/attractor/designators_js.go), so a finding
// and a sentence about the panel point at the same thing. A finding is printed
// once, at the first model it appears on, with how many models it is on.
//
// Usage:
//
//	uitool lint -headless -serve ./chaosrack          # every model, nothing on screen
//	uitool lint -headless -serve ./chaosrack -models globe,lorenz
package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// lintJS measures the panel as it stands and returns its findings.
const lintJS = `(function(){
  var out=[], p=document.getElementById('controls-panel'); if(!p) return '[]';
  var PREFIX=/^(?:[0-9S]+\.\d+(?:\.\d+(?:\.[a-z])?)? · )+/, LOC=/^[0-9S]+\.\d+\.\d+(?:\.[a-z])?$/;
  function vis(e){
    if(!e.getClientRects().length) return false;
    if(e.checkVisibility && !e.checkVisibility({visibilityProperty:true,opacityProperty:true})) return false;
    var r=e.getBoundingClientRect(); return r.width>0&&r.height>0;
  }
  // The module a finding is in, by its header, so an address can be read
  // without the panel open.
  var modOf={};
  [].forEach.call(p.querySelectorAll('[data-loc]'),function(c){
    var s=c.closest('.sect'), h=s&&s.querySelector('.sect-hdr');
    modOf[c.dataset.loc]=h?h.textContent.trim():(c.closest('.runit-panel')?'Scope':'');
  });
  function add(loc,kind,msg){ out.push({l:loc,k:kind,m:msg,h:modOf[loc]||''}); }
  function name(e){ return (e.tagName.toLowerCase()+'.'+String(e.className.baseVal||e.className).split(' ')[0]); }
  var cells=[].filter.call(p.querySelectorAll('[data-loc]'),vis);

  // OVERFLOW: inputs, and boxes that clip what they hold.
  cells.forEach(function(c){
    [].forEach.call(c.querySelectorAll('input.numin,.led'),function(e){
      if(!vis(e)) return;
      var clips=e.tagName==='INPUT'||/hidden|clip/.test(getComputedStyle(e).overflowX);
      if(clips&&e.scrollWidth>e.clientWidth+1)
        add(c.dataset.loc,'OVERFLOW',name(e)+' "'+(e.value||e.textContent).trim()+'" needs '+e.scrollWidth+'px, has '+e.clientWidth);
    });
  });

  // OVERLAP: the visible parts of every control in a module, pairwise. A
  // knob is its circle, everything else its box; a part inside another is
  // one thing, not two.
  var PARTS='.dmdwin,.u-val,.u-step,.dmdval,.led,.numin,.rst,.stepknob,.vdial-tick,.knob-dial-lab,.knob:not(.knob-fine),.plabel,.u-lbl,input.sw,.twoway-name,.pslot,.trio-btn';
  var byMod=new Map();
  cells.forEach(function(c){
    var m=c.closest('.sect')||c.closest('.runit-panel'); if(!m) return;
    if(!byMod.has(m)) byMod.set(m,[]);
    [].forEach.call(c.querySelectorAll(PARTS),function(e){
      if(!vis(e)) return;
      if(e.classList.contains('knob') && e.parentElement.closest('.knob')) return; // a knob's inner ring is the knob
      var r=e.getBoundingClientRect();
      var circ=e.classList.contains('knob');
      byMod.get(m).push({e:e,c:c,r:r,circ:circ,cx:r.left+r.width/2,cy:r.top+r.height/2,rad:r.width/2});
    });
  });
  function hit(a,b){
    if(a.circ&&b.circ){ var dx=a.cx-b.cx,dy=a.cy-b.cy; return Math.hypot(dx,dy)<a.rad+b.rad-1; }
    if(a.circ||b.circ){
      var k=a.circ?a:b, o=a.circ?b:a;
      var nx=Math.max(o.r.left,Math.min(k.cx,o.r.right)), ny=Math.max(o.r.top,Math.min(k.cy,o.r.bottom));
      return Math.hypot(nx-k.cx,ny-k.cy)<k.rad-1;
    }
    var w=Math.min(a.r.right,b.r.right)-Math.max(a.r.left,b.r.left);
    var h=Math.min(a.r.bottom,b.r.bottom)-Math.max(a.r.top,b.r.top);
    return w>1&&h>1;
  }
  byMod.forEach(function(ps){
    for(var i=0;i<ps.length;i++) for(var j=i+1;j<ps.length;j++){
      var a=ps[i], b=ps[j];
      if(a.e.contains(b.e)||b.e.contains(a.e)) continue;
      // A dial's ticks and labels ring their own knob by design.
      var ad=a.e.closest('.knobwrap,.knobstack'), bd=b.e.closest('.knobwrap,.knobstack');
      if(ad&&ad===bd) continue;
      if(a.e.classList.contains('vdial-tick')&&b.e.classList.contains('vdial-tick')) continue;
      // Concentric knobs (a hue ring round a level knob) are one control.
      if(a.circ&&b.circ&&Math.hypot(a.cx-b.cx,a.cy-b.cy)<3) continue;
      if(!hit(a,b)) continue;
      var la=a.c.dataset.loc, lb=b.c.dataset.loc;
      add(la,'OVERLAP',name(a.e)+' over '+name(b.e)+(la===lb?'':' of '+lb));
    }
  });

  // CLIPPED: a part reaching past a box that cuts off what overflows it
  // (a dial's outer labels, half a character gone), up to its module.
  byMod.forEach(function(ps,m){
    ps.forEach(function(a){
      for(var p=a.e.parentElement;p;p=p.parentElement){
        var s=getComputedStyle(p);
        if(/hidden|clip|auto|scroll/.test(s.overflowX+s.overflowY)){
          var pr=p.getBoundingClientRect();
          if(a.r.left<pr.left-0.5||a.r.right>pr.right+0.5||a.r.top<pr.top-0.5||a.r.bottom>pr.bottom+0.5){
            add(a.c.dataset.loc,'CLIPPED',name(a.e)+' "'+(a.e.textContent||'').trim().slice(0,8)+'" cut off by '+name(p));
            break;
          }
        }
        if(p===m) break;
      }
    });
  });

  // OFFCENTER: a cell's readouts on its knob's center line.
  cells.forEach(function(c){
    if(!c.matches('.punit')) return;
    var k=[].filter.call(c.querySelectorAll('.knob:not(.knob-fine)'),function(e){return vis(e)&&!e.parentElement.closest('.knob');})[0];
    if(!k) return;
    var kr=k.getBoundingClientRect(), kx=kr.left+kr.width/2;
    [].forEach.call(c.querySelectorAll('.punit-top>.u-val,.punit-top>.dmdval,.u-step'),function(e){
      if(!vis(e)) return;
      var r=e.getBoundingClientRect(), dx=r.left+r.width/2-kx;
      if(Math.abs(dx)>1.5) add(c.dataset.loc,'OFFCENTER',name(e)+' '+dx.toFixed(1)+'px off the knob');
    });
  });

  // DISPLAY-CUT: a character display given more than it has characters for.
  cells.forEach(function(c){
    [].forEach.call(c.querySelectorAll(".dmdwin[data-chars]"),function(w){
      if(!vis(w)) return;
      var svg=w.querySelector("svg"), t=svg?(svg.getAttribute("aria-label")||""):"", n=+w.getAttribute("data-chars");
      if([].slice.call(t).length>n) add(c.dataset.loc,"DISPLAY-CUT","\""+t+"\" on "+n+" characters");
    });
  });

  // SEG-LETTERS: seven segments cannot spell.
  cells.forEach(function(c){
    [].forEach.call(c.querySelectorAll('input.numin,.led'),function(e){
      if(!vis(e)) return;
      var t=(e.value||e.textContent||'').trim();
      // Hex is the exception: A to F are the letters seven segments were
      // always asked for, and a color readout is hex.
      if(/DSEG/.test(getComputedStyle(e).fontFamily)&&/[A-Za-z]/.test(t)&&!/^#?[0-9A-Fa-f]+$/.test(t)) add(c.dataset.loc,'SEG-LETTERS',name(e)+' "'+t+'"');
    });
  });

  // TIP: what a hover over the control's actuator shows.
  var ACT='.knob:not(.knob-fine),input.sw,button:not(.rst),select,.pslot';
  // A cell with no actuator is a readout (designate addresses those too):
  // what a hover shows there is the readout's.
  var RO='.led,input[type=text]';
  cells.forEach(function(c){
    var a=[].filter.call(c.querySelectorAll(ACT),vis)[0]||[].filter.call(c.querySelectorAll(RO),vis)[0]||c, t='';
    // Not the module's own: every module carries its address and summary
    // now, which would pass any control inside it.
    for(var e=a;e&&!t&&!(e.classList&&e.classList.contains('sect'));e=e.parentElement) t=e.getAttribute&&e.getAttribute('title')||'';
    t=t.replace(PREFIX,'').trim();
    if(!t||LOC.test(t)) { add(c.dataset.loc,'TIP-MISSING',name(a)); return; }
    // Terse is the bare stamp, "Module / label / part", with nothing after
    // it: a sentence of the markup's own is help even without a dash in it.
    var seg=t.split(' / ');
    if(t.indexOf(' — ')<0&&seg.length===3&&/^(selector |hue |level |fine-trim )?knob\b|^label$|^slider$|field$|readout$/.test(seg[2]))
      add(c.dataset.loc,'TIP-TERSE','"'+t.slice(0,60)+'"');
    // An address belongs at the front of a tooltip, once.
    if(/[0-9S]+\.\d+\.\d+[a-z]? · /.test(t)) add(c.dataset.loc,'TIP-ADDRESS','"'+t.slice(0,60)+'"');
  });

  // NONUNIFORM: a bank is one part repeated.
  [].forEach.call(p.querySelectorAll('.catbank'),function(b){
    var loc=(b.querySelector('[data-loc]')||{dataset:{}}).dataset.loc||'?';
    [['.knob.knobb','knob'],['.punit-top>.u-val,.punit-top>.dmdval','readout'],['.u-step','step readout'],['.dmdwin.dmdv','legend'],['.stepknob','step knob']].forEach(function(q){
      var sizes={};
      [].forEach.call(b.querySelectorAll(q[0]),function(e){ if(!vis(e)) return; var r=e.getBoundingClientRect(); sizes[Math.round(r.width)+'x'+Math.round(r.height)]=1; });
      var ks=Object.keys(sizes); if(ks.length>1) add(loc,'NONUNIFORM',q[1]+' sizes '+ks.join(', '));
    });
  });

  // SLACK and EMPTY: a module against what is on it. Every check above looks
  // at controls, so a 22-slot module holding one knob passed all of them.
  // What is on a module is the span of its visible leaves (a knob, a display
  // and a button count whole); quantizing leaves under a slot of slack, so a
  // whole empty slot is a fault. The narrowest module is one slot.
  var mods=[].filter.call(p.querySelectorAll('.sect'),vis), slot=Infinity;
  mods.forEach(function(m){ slot=Math.min(slot,m.getBoundingClientRect().width); });
  var WHOLE=/^(CANVAS|svg|INPUT|BUTTON|SELECT|TEXTAREA)$/;
  mods.forEach(function(m){
    var mr=m.getBoundingClientRect(), L=Infinity, R=-Infinity, n=0;
    [].forEach.call(m.querySelectorAll('*'),function(e){
      if(e.closest('.sect-hdr')) return;
      if(e.children.length&&!WHOLE.test(e.tagName)&&!e.matches('.knob,.led,.dmdwin')) return;
      if(e.parentElement&&e.parentElement.closest('svg,.knob,.dmdwin,button')) return;
      if(!vis(e)) return;
      var r=e.getBoundingClientRect(); n++; L=Math.min(L,r.left); R=Math.max(R,r.right);
    });
    var h=m.querySelector('.sect-hdr'), c=m.querySelector('[data-loc]');
    var loc=c?c.dataset.loc:'-', hd=h?h.textContent.trim():(m.id||'');
    if(!n){ out.push({l:loc,k:'EMPTY',m:'nothing visible on '+Math.round(mr.width)+'px',h:hd}); return; }
    if(mr.width-(R-L)>slot) out.push({l:loc,k:'SLACK',m:Math.round(mr.width)+'px wide, holds '+Math.round(R-L)+'px',h:hd});
  });
  return JSON.stringify(out);
})()`

type lintFinding struct {
	loc, kind, msg string
	module         string // the header of the module it is in
	first          string // the first model it was seen on
	models         int
}

func runLint() {
	c, err := dial()
	if err != nil {
		fmt.Println("lint:", err)
		exit(1)
	}
	defer c.Close() //nolint:errcheck // the process is ending
	c.Eval(setupJS)
	if !*sweepRender {
		c.Eval(`(function(){var p=document.getElementById('power-sw');if(p&&p.checked){p.checked=false;p.dispatchEvent(new Event('change'));}})()`)
	}
	var models []string
	if *shotModels != "" {
		for _, m := range strings.Split(*shotModels, ",") {
			models = append(models, strings.TrimSpace(m))
		}
	} else {
		for _, m := range toList(anyJSON(c.Eval(modelsJS))) {
			models = append(models, str(m["v"]))
		}
	}
	fmt.Printf("lint: %s, %d models\n", c.URL, len(models))
	start := time.Now()
	seen := map[string]*lintFinding{}
	var order []string
	for _, mode := range models {
		c.Eval(fmt.Sprintf(`(function(){var s=document.getElementById('mode-select');s.value=%q;s.dispatchEvent(new Event('change'));})()`, mode))
		c.Eval(settleJS)
		time.Sleep(time.Duration(*sweepSettle) * time.Millisecond)
		c.Eval(settleJS) // the designators are written a frame after the rack settles
		for _, f := range toAnyList(anyJSON(c.Eval(lintJS))) {
			m, _ := f.(map[string]any)
			key := str(m["k"]) + " " + str(m["l"]) + " " + str(m["m"])
			if lf, ok := seen[key]; ok {
				lf.models++
				continue
			}
			seen[key] = &lintFinding{loc: str(m["l"]), kind: str(m["k"]), msg: str(m["m"]), module: str(m["h"]), first: mode, models: 1}
			order = append(order, key)
		}
	}
	byKind := map[string]int{}
	for _, k := range order {
		f := seen[k]
		byKind[f.kind]++
		on := f.first
		if f.models > 1 {
			on = fmt.Sprintf("%s +%d", f.first, f.models-1)
		}
		fmt.Printf("  %-11s %-8s %-18s %s   [%s]\n", f.kind, f.loc, f.module, f.msg, on)
	}
	kinds := make([]string, 0, len(byKind))
	for k, n := range byKind {
		kinds = append(kinds, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(kinds)
	fmt.Printf("\nlint: %d finding(s) over %d models in %s   %s\n", len(order), len(models), time.Since(start).Round(time.Millisecond), strings.Join(kinds, ", "))
	if len(order) > 0 {
		exit(1)
	}
}
