package racktui

import (
	"strings"

	"github.com/gdamore/tcell/v3"
)

// Text a hand types into a control: an equation, a banner, a path. Enter on
// a text control opens it for editing on the status line, Enter again sets
// it, and Escape leaves it as it was. While it is open every key is the
// text's: q, p and the rest are letters.

// Prompter is a source that can ask for text: a press that needs a path to
// go on, as the page's Load opens a file picker.
type Prompter interface {
	// Prompt is the text control waiting to be filled, and what to ask, if
	// one is; asking once takes it.
	Prompt() (id, ask string, ok bool)
}

// textEdit is a text control being edited.
type textEdit struct {
	id, ask, buf string
}

// startEdit opens the control under the cursor for editing, if it is text.
func (p *panel) startEdit() {
	c, ok := p.at()
	if !ok || !c.IsText {
		return
	}
	p.edit = &textEdit{id: c.ID, buf: c.Value}
}

// editKey is a key while a text control is open, and reports whether it
// closed it.
func (p *panel) editKey(ev *tcell.EventKey) {
	e := p.edit
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		p.edit = nil
		p.msg = e.id + " left as it was"
	case tcell.KeyEnter:
		p.edit = nil
		if err := p.src.Set(e.id, e.buf); err != nil {
			p.err = err.Error()
			return
		}
		p.err = ""
		p.msg = e.id + " = " + e.buf
		p.reload()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r := []rune(e.buf); len(r) > 0 {
			e.buf = string(r[:len(r)-1])
		}
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			e.buf += ev.Str()
		}
	}
}

// editLine is the status line while a control is open.
func (p *panel) editLine() string {
	ask := p.edit.ask
	if ask == "" {
		ask = p.edit.id
	}
	return ask + ": " + strings.ReplaceAll(p.edit.buf, "\n", " ") + "_   enter to set, esc to leave"
}

// takePrompt opens the control the source asks for, if it asks.
func (p *panel) takePrompt() {
	pr, ok := p.src.(Prompter)
	if !ok || p.edit != nil {
		return
	}
	id, ask, ok := pr.Prompt()
	if !ok {
		return
	}
	buf := ""
	for _, c := range p.all {
		if c.ID == id {
			buf = c.Value
		}
	}
	p.edit = &textEdit{id: id, ask: ask, buf: buf}
}
