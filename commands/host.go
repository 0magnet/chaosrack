//go:build !js

package commands

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/audiows"
	"github.com/0magnet/chaosrack/pkg/server"
	"github.com/0magnet/chaosrack/pkg/stlgen"
	"github.com/0magnet/chaosrack/pkg/uitool"
)

const platformHelp = "\n\n" + server.PanelHelp + `

serve is the server for that page. ctl and rack drive the same rack from a
script, render and models draw models to image files without a browser,
stlgen writes the models as STL files, audiows is the audio
WebSocket/WebTransport test harness, and uitool drives the page over CDP for
tests and captures.`

// platform adds what only a host has: where the rack is, and the server.
func platform(root *cobra.Command) {
	server.PanelFlags(root.Flags())
	root.AddCommand(server.Commands()...)
	root.AddCommand(audiows.Command(), stlgen.Command(), uitool.Command())
}

// panel drives a rack in a browser from this terminal.
func panel(cmd *cobra.Command) error { return server.Panel(cmd.Context()) }

// Execute runs chaosrack. Both the repo-root main and cmd/chaosrack call it.
func Execute() {
	if err := New().Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}
