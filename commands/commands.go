//go:build !js

// Package commands assembles the one chaosrack command: the web server at the
// root, with the harnesses (audiows, stlgen, uitool) beside its own tui, ctl,
// rack, render and models subcommands.
package commands

import (
	"log"

	"github.com/0magnet/calvin/clihelp"

	"github.com/0magnet/chaosrack/pkg/audiows"
	"github.com/0magnet/chaosrack/pkg/server"
	"github.com/0magnet/chaosrack/pkg/stlgen"
	"github.com/0magnet/chaosrack/pkg/uitool"
)

// RootCmd is the chaosrack command.
var RootCmd = server.Command()

func init() {
	RootCmd.AddCommand(audiows.Command(), stlgen.Command(), uitool.Command())
}

// Execute runs RootCmd. Both the repo-root main and cmd/chaosrack call it.
func Execute() {
	clihelp.Init(RootCmd, "chaosrack")
	if err := RootCmd.Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}
