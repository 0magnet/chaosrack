package attractor

import (
	"regexp"
	"strconv"
	"strings"
)

// Four scopes, each beside a generator. The markup has one, Scope 1's, and
// the others are copies of it made before the panel is built (withRackScopes):
// the same tube and knobs under ids of their own, scope2-volts beside
// scope-volts, so every control on each one is a control of its own — its own
// reset, its own place in a link.
//
// Each picks what its two channels are fed from (scopeInNames): the rack's
// signal, either side; the capture, wherever the Mixer sends it; any one
// generator by itself, wherever it is pinned; or a coordinate of the model.

// rackScopeCount is how many scopes the rack has.
const rackScopeCount = 4

// scopePrefix is the id prefix of scope n, from 0: "scope" for the first, so
// its ids are the ones the markup was written with, and "scope2" and on for
// the copies.
func scopePrefix(n int) string {
	if n == 0 {
		return "scope"
	}
	return "scope" + strconv.Itoa(n+1)
}

// scopeIDs is an id the scope's markup gives, written for the first scope.
var scopeIDs = regexp.MustCompile(`id="(rst-)?scope-`)

// scopeBlock is where the first scope's markup starts, and what follows it.
const (
	scopeBlockStart = `<div class="sect" id="scope-module">`
	scopeBlockEnd   = "\n</div></div>\n"
)

// withRackScopes is body with the scope's markup copied for the others, each
// copy straight after the one before, renamed and retitled. Nothing else is
// touched: a body without the scope is returned as it is.
func withRackScopes(body string) string {
	i := strings.Index(body, scopeBlockStart)
	if i < 0 {
		return body
	}
	n := strings.Index(body[i:], scopeBlockEnd)
	if n < 0 {
		return body
	}
	j := i + n + len(scopeBlockEnd)
	block := body[i:j]
	var b strings.Builder
	b.WriteString(body[:j])
	for k := 1; k < rackScopeCount; k++ {
		p := scopePrefix(k)
		c := scopeIDs.ReplaceAllString(block, `id="${1}`+p+`-`)
		c = strings.Replace(c, ">Scope 1</div>", ">Scope "+strconv.Itoa(k+1)+"</div>", 1)
		c = strings.Replace(c, `title="Scope 1 — `, `title="Scope `+strconv.Itoa(k+1)+` — `, 1)
		b.WriteString(c)
	}
	b.WriteString(body[j:])
	return b.String()
}
