//go:build js

package clihelp

// Color is on in a browser. There is nothing to detect: the writer the help
// goes to is a pipe, and the terminal reading the other end of it renders ANSI.
var Color = true
