package attractor

import "strings"

// moduleSelector finds a module anywhere in the rack frame.
//
// It is a DESCENDANT selector and must stay one. Modules were once direct
// children of .modules, and the selector was `.modules > .sect` to match. Bay
// packing wraps them — .modules > .runit > .runit-open > .sect — and the child
// selector then matched NOTHING.
//
// What made that expensive is how quietly it failed. buildModEQModules looks
// up the primary module each MOD/EQ pair attaches to and skips any group whose
// primary it cannot find; with the selector matching nothing it skipped every
// group, so turning Audio mod on built no modulation modules at all. Nothing
// errored: the checkbox still flipped the panel's am-on class, and the CSS
// that hides .modmodule while mod is off still worked perfectly on the
// modules that were never created.
//
// Any code that looks a module up by its header must use this rather than
// writing the selector again, because the next thing to wrap a module will
// break every private copy the same way.
const moduleSelector = ".modules .sect"

// selectorIsNested reports whether a selector will match an element at any
// depth rather than only as a direct child.
//
// Pure, so it can be tested on a host with no DOM — which is the only kind
// this package's tests run on. It does not prove the selector finds the right
// modules; it pins the one property whose loss is invisible.
func selectorIsNested(sel string) bool { return !strings.Contains(sel, ">") }
