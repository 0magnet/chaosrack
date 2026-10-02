//go:build !js

package server

// The manual, at /manual: the rack itself, opened in its manual mode
// (pkg/attractor/manualmode_js.go), which lays the panel out without drawing
// and writes every bay's page, the way the Info window writes one. So the
// pages are the rack's own — where each control is, as well as what it is —
// and nothing that depends on the layout is written down anywhere.
//
// The text is the manual package's, built into the wasm. --manual-dir hands
// the page the files on disk instead (/manual/src.json), so whoever is writing
// them sees an edit on reload, without rebuilding anything.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0magnet/chaosrack/manual"
)

var manualDir string

func init() {
	runCmd.Flags().StringVar(&manualDir, "manual-dir", "", "serve the manual's markdown from this directory (the repository's manual/) instead of the copy built in, re-read on every load of /manual")
}

// manualRoutes puts the manual on r: /manual, the rack in its manual mode,
// and /manual/src.json, the markdown it is written from.
func manualRoutes(r *gin.Engine) {
	r.GET("/manual", func(c *gin.Context) { c.Redirect(http.StatusFound, "/?manual") })
	r.GET("/manual/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/?manual") })
	r.GET("/manual/src.json", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.JSON(http.StatusOK, manualSources(manualDir))
	})
}

// manualSources is the manual's markdown by file name without .md: dir's
// files when dir is set, the built-in copy otherwise or when dir has none.
func manualSources(dir string) map[string]string {
	if dir != "" {
		names, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err == nil && len(names) > 0 {
			src := map[string]string{}
			for _, n := range names {
				if b, err := os.ReadFile(n); err == nil { //nolint:gosec // the directory the operator named on the command line
					src[strings.TrimSuffix(filepath.Base(n), ".md")] = string(b)
				}
			}
			return src
		}
	}
	return manual.Sources()
}
