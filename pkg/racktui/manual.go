package racktui

import "github.com/0magnet/chaosrack/manual"

// ManualPage is what the Info pane shows of a rack: the model on show and its
// description, "" with none, and the page of the manual for one bay — its
// title, and each module on it with each of its controls by address.
type ManualPage struct {
	Model     string          `json:"model,omitempty"`
	ModelText string          `json:"modelText,omitempty"`
	Title     string          `json:"title"`
	Mods      []manual.Module `json:"mods"`
}

// Manualed is a Source whose manual a panel shows: the Info pane (i), and the
// first sentence of a control's entry on the status line.
type Manualed interface {
	// Manual is bay n's page of the manual, counting bays from 1, with the
	// model on show.
	Manual(bay int) (ManualPage, error)
}
