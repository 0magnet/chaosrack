#!/bin/sh
# Print the GOTOOLCHAIN value TinyGo needs, or "auto" when the system Go will do.
#
#   GOTOOLCHAIN=$(sh scripts/tinygo-toolchain.sh) tinygo build -target=wasm .
#
# Go releases a new minor twice a year; TinyGo supports it some weeks later.
# In between, every TinyGo build fails with
#
#     requires go version 1.19 through 1.26, got go1.27
#
# The usual answers are to downgrade Go or to pin a version in the build script.
# Downgrading breaks everything else, and a pin goes stale: it keeps building
# against an old patch release long after newer ones have fixed security bugs.
#
# Both unknowns are discoverable, so nothing here is written down:
#
#   the ceiling  TinyGo names it in the message above, and the check runs
#                before any package is loaded, so asking costs nothing.
#   the patch    go.dev publishes every release; take the newest of that minor.
#
# When TinyGo catches up, the probe succeeds, this prints "auto", and the build
# uses the system Go with no network access at all.
set -u

say() { echo "tinygo-toolchain: $*" >&2; }

command -v tinygo >/dev/null 2>&1 || { echo auto; exit 0; }

# Probe. The package does not exist, and does not need to: the toolchain check
# happens first, so this returns immediately either way.
probe=$(cd / && tinygo build -o /dev/null -target=wasm ./tinygo-toolchain-probe 2>&1)
minor=$(printf '%s\n' "$probe" |
	sed -n 's/.*requires go version [0-9][0-9.]* through \([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' |
	head -1)

# No complaint about the Go version: whatever else the probe said is the
# caller's problem, and the system toolchain is fine.
[ -n "$minor" ] || { echo auto; exit 0; }

# Newest patch of that minor, from the release list.
latest=$(curl -fsS --max-time 20 'https://go.dev/dl/?mode=json&include=all' 2>/dev/null |
	grep -oE '"version"[[:space:]]*:[[:space:]]*"go[0-9.]+"' | grep -oE 'go[0-9.]+' |
	grep -E "^go${minor}(\.[0-9]+)?$" | sort -uV | tail -1)

# Same question to the module proxy, which some networks reach when go.dev is
# blocked and which serves the toolchains themselves.
[ -n "$latest" ] || latest=$(curl -fsS --max-time 20 'https://proxy.golang.org/golang.org/toolchain/@v/list' 2>/dev/null |
	grep -oE "go${minor}(\.[0-9]+)?" | sort -uV | tail -1)

# Offline: a toolchain downloaded by an earlier build is still on disk, and an
# older patch that exists beats a newer one that cannot be fetched.
if [ -z "$latest" ]; then
	cache=$(go env GOMODCACHE 2>/dev/null)/golang.org
	[ -d "$cache" ] && latest=$(ls "$cache" 2>/dev/null |
		grep -oE "go${minor}(\.[0-9]+)?" | sort -uV | tail -1)
	[ -n "${latest:-}" ] && say "offline; using the cached $latest"
fi

if [ -z "${latest:-}" ]; then
	say "TinyGo needs Go $minor.x and no release of it could be found."
	say "Leaving the toolchain alone so TinyGo reports it plainly."
	echo auto
	exit 0
fi

say "TinyGo supports Go $minor at most; building with $latest"
echo "$latest"
