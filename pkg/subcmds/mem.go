package subcmds

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newMemCmd(d Dialer) *cobra.Command {
	var (
		asJSON bool
		top    int
	)
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "how much memory the rack's page uses, and on what",
		Long: `Print how much memory the running rack's page uses, and on what.

It lists:
  - the wasm heap: the Go runtime's own account of it, and the linear memory
    it lives in, which only grows;
  - the wasm module, which the browser keeps besides its compiled code;
  - the page's JavaScript heap;
  - its elements, per module;
  - its canvases.

The tab's whole footprint is more than this. The browser's side, the
compositor's tiles and the GPU's copies of the canvases, shows in Brave's
task manager (Shift+Esc).

  chaosrack mem
  chaosrack mem --top 0
  chaosrack mem --json`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := d.Dial()
			if err != nil {
				return err
			}
			rep, err := c.Mem()
			if err != nil {
				return err
			}
			if asJSON {
				b, err := json.MarshalIndent(rep, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(stdout, string(b))
				return err
			}
			return rep.Write(stdout, top)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the reading as JSON")
	cmd.Flags().IntVar(&top, "top", 8, "list this many modules and canvases, the largest first (0 for all)")
	if d.Flags != nil {
		d.Flags(cmd.Flags())
	}
	return cmd
}
