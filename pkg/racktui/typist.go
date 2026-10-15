package racktui

import "github.com/gdamore/tcell/v3"

// Typist is a source whose model takes typing, as a terminal on show does:
// with the window put away, every key is the model's — Escape and Ctrl+C
// too, which a shell needs — but Ctrl+], telnet's way back out, which brings
// the window back.
type Typist interface {
	// TypeKey types ev into the model on show, and reports whether it took
	// it: a model that takes no typing leaves the key the panel's.
	TypeKey(ev *tcell.EventKey) bool
}

// typingMsg is the status line while keys go to the model.
const typingMsg = "typing to the model: Ctrl+] for the panel"

// typeKey offers ev to the model on show, and reports whether it took it.
// Only with the window put away, as a game's keys are.
func (p *panel) typeKey(ev *tcell.EventKey) bool {
	ty, ok := p.src.(Typist)
	if !ok || p.winMode != winHidden || !ev.Pressed() {
		return false
	}
	if isCtrlRightBracket(ev) {
		p.winMode = winHalf
		p.scene, p.canv = nil, nil
		if p.msg == typingMsg {
			p.msg = ""
		}
		return true
	}
	if !ty.TypeKey(ev) {
		return false
	}
	p.msg = typingMsg
	return true
}

// isCtrlRightBracket reports whether ev is Ctrl+], as a terminal sends it
// (GS) or as the kitty keyboard protocol reports it.
func isCtrlRightBracket(ev *tcell.EventKey) bool {
	return ev.Key() == tcell.KeyGS ||
		ev.Key() == tcell.KeyRune && ev.Str() == "]" && ev.Modifiers()&tcell.ModCtrl != 0
}
