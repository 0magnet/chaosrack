package racktui

// powerID is the rack's power switch.
const powerID = "power-sw"

// PowerOn switches the rack on for as long as a panel drives it, and gives
// back what puts it as it was. In the page, chaosrack in the instrument's
// terminal is what runs the model, and stopping it switches the rack off; a
// panel on a host is the same command, so it does the same. Without this a
// host panel attached to a page whose instrument was stopped drove a rack
// that was off: a scene that never moved, and turns that changed nothing
// anyone could see.
func PowerOn(src Source) (restore func()) {
	cs, err := src.Controls()
	if err != nil {
		return func() {}
	}
	for _, c := range cs {
		if c.ID != powerID || c.Value != "0" {
			continue
		}
		if src.Set(powerID, "1") != nil {
			return func() {}
		}
		return func() { _ = src.Set(powerID, "0") } //nolint:errcheck // on the way out; the page may already be gone
	}
	return func() {}
}
