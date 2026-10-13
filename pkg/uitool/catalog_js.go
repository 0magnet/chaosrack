package uitool

import (
	"fmt"
	"os"
)

// runCatalog needs a link to a page's server, which a page itself has no
// way to dial.
func runCatalog() {
	fmt.Fprintln(os.Stderr, "catalog: runs on a host, against a page's server")
	exit(1)
}
