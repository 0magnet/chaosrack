//go:build !js

package commands

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/0magnet/chaosrack/pkg/audiows"
	"github.com/0magnet/chaosrack/pkg/server"
	"github.com/0magnet/chaosrack/pkg/subcmds"
	"github.com/0magnet/chaosrack/pkg/uitool"
)

// platform adds what only a host has: where the rack is, and the commands
// that need this machine — the server, its audio, a browser to drive.
func platform(root *cobra.Command) {
	server.PanelFlags(root.Flags())
	root.AddCommand(server.Serve(), audiows.Command(), uitool.Command())
}

// rackDialer reaches the rack in a browser tab, for ctl and rack.
func rackDialer() subcmds.Dialer { return server.RackDialer() }

// panel drives a rack in a browser from this terminal.
func panel(cmd *cobra.Command) error { return server.Panel(cmd.Context()) }

// Execute runs chaosrack. Both the repo-root main and cmd/chaosrack call it.
func Execute() {
	if err := New().Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}
