//go:build js

package subcmds

import (
	"errors"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// bayPage is bay n's page of the manual, read off the page it runs in, which
// is the bay as it stands: the model is the one on the rack.
func bayPage(d Dialer, n int, model string) (racktui.ManualPage, error) {
	if model != "" {
		return racktui.ManualPage{}, errors.New("--model is for a rack without the page; here, choose the model on the rack")
	}
	c, err := d.Dial()
	if err != nil {
		return racktui.ManualPage{}, err
	}
	return c.Manual(n)
}
