.DEFAULT_GOAL := help
.PHONY: help format tidy lint vet test test-wasm test-browser cover check install-linters docs pages onefile site readme-check site-check sweep lint-panel publish-gifs highlights catalog check-names check-inits

# The targets that matter are `format` and `check`, and they mean the same
# thing here as in 0pcom/skywire, which is the reference for these repos.
# .golangci.yml is copied from there.

PROJECT_BASE := github.com/0magnet/chaosrack
OPTS ?= GO111MODULE=on

# Whether the host checks run with cgo. 0 matches skywire, which is pure Go.
# Repos whose real build needs cgo — a GUI, audio, anything binding a C library
# — set this to 1, because linting them without it type-checks almost nothing.
CGO ?= 0

# Packages this toolchain cannot build at all — firmware for a different
# target, say. Not the same as code that merely does not build for this host:
# js/wasm is handled below by running the checks again in that context, which
# is the better answer whenever it is available. Empty in most repos.
SKIP ?=

# Directories rather than import paths, because golangci-lint resolves a bare
# import path against the working directory and then cannot find it.
#
# Listed with the same CGO_ENABLED the checks use. Listed with cgo on and
# linted with it off, a cgo-only package is named and then found to have no
# files in it, which fails the run rather than reporting anything about it.
#
# -e so that one package that cannot be listed does not empty the list. Without
# it, a single unbuildable package makes `go list` fail for the whole module,
# this comes back blank, and the run below reports that there is nothing to
# check — which reads exactly like passing.
# Nested modules. `go list ./...` STOPS AT A MODULE BOUNDARY, so a repo that
# keeps a second go.mod — desk's panes, pisano's web — had that half silently
# unchecked: not failing, not skipped with a message, simply absent, which reads
# exactly like passing. Each one is handled by re-running this same Makefile
# inside it, so a submodule is checked the way the root is by construction
# rather than by remembering to add it somewhere.
#
# Discovered rather than listed, so adding a module does not also mean editing
# this. SUBMODULES= on the recursive call is what stops it recursing forever.
THIS := $(abspath $(firstword $(MAKEFILE_LIST)))
# The one lint config, by absolute path: a submodule has no .golangci.yml of
# its own, and the recursive call runs with that submodule as the working
# directory, so a relative -c would look for it there and fail.
LINTCFG := $(dir $(THIS)).golangci.yml
# Vendor mode only where there IS a vendor directory. The config names one mode
# for the whole repo, and a submodule that does not vendor cannot satisfy it —
# golangci-lint then reports "inconsistent vendoring" for a tree that is
# perfectly consistent, because it was told to look for something that was
# never there. Evaluated in the working directory, so the recursion gets the
# answer for the module it is actually in.
MODMODE = $(if $(wildcard vendor/modules.txt),vendor,readonly)
SUBMODULES := $(patsubst %/go.mod,%,$(shell find . -mindepth 2 -name go.mod -not -path './vendor/*' -not -path '*/vendor/*' 2>/dev/null | sort))

define recurse
@for m in $(SUBMODULES); do \
		echo "--- $$m"; \
		$(MAKE) --no-print-directory -C $$m -f $(THIS) $@ SUBMODULES= || exit 1; \
	done
endef

PKGS = $(shell CGO_ENABLED=$(CGO) go list -e -f '{{.Dir}}' ./... 2>/dev/null $(if $(SKIP),| grep -vE '$(SKIP)'))
JSPKGS = $(shell CGO_ENABLED=0 GOOS=js GOARCH=wasm go list -e -f '{{.Dir}}' ./... 2>/dev/null $(if $(SKIP),| grep -vE '$(SKIP)'))

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

tidy: ## Tidy dependencies
	${OPTS} go mod tidy -v

format: tidy ## Format the code. Needs goimports (make install-linters)
	@if grep -qE '^(replace|exclude)' go.mod; then \
		echo "ERROR: go.mod contains replace or exclude directives which break go install @version"; \
		grep -E '^(replace|exclude)' go.mod; \
		exit 1; \
	fi
	@# Files, not directories. goimports -w on a DIRECTORY walks it recursively,
	@# and the module root is one of the directories `go list` returns — so the
	@# /vendor/ filter that used to be here filtered a path that was never passed
	@# and goimports reformatted all 263 vendored files anyway, every time. The
	@# pattern matches at any depth: a nested vendor tree (0magnet/desk, pisano)
	@# is not caught by an anchored ./vendor/*.
	${OPTS} goimports -w -local ${PROJECT_BASE} $(shell find . -name "*.go" -not -path "*/vendor/*" -not -path "*/.git/*")

lint: ## Run golangci-lint. Needs it installed (make install-linters)
	command -v golangci-lint || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	golangci-lint --version
	@# Some of these repos are entirely js/wasm-tagged, so the host context has
	@# nothing in it and linting it is an error rather than a pass.
	@if [ -n "$(PKGS)" ]; then \
		CGO_ENABLED=$(CGO) ${OPTS} golangci-lint run --modules-download-mode=$(MODMODE) -c $(LINTCFG) $(PKGS); \
	else \
		echo '--- nothing builds for this host; skipping the host pass'; \
	fi
	@# A host run cannot see js/wasm-tagged files, so anything only they use
	@# reads as dead — and anything wrong inside them is never checked at all.
	@if grep -rlq '^//go:build js' --include='*.go' --exclude-dir=vendor . 2>/dev/null; then \
		echo '--- again in the js/wasm build context'; \
		CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} golangci-lint run --modules-download-mode=$(MODMODE) -c $(LINTCFG) $(JSPKGS); \
	fi
	$(recurse)

vet: ## Run go vet
	@if [ -n "$(PKGS)" ]; then \
		CGO_ENABLED=$(CGO) ${OPTS} go vet $(PKGS); \
	fi
	@if grep -rlq '^//go:build js' --include='*.go' --exclude-dir=vendor . 2>/dev/null; then \
		CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} go vet $(JSPKGS); \
	fi
	$(recurse)

test: ## Run tests. Falls back to the js/wasm ones where nothing builds here
	@if [ -n "$(PKGS)" ]; then \
		${OPTS} go test $(PKGS); \
	else \
		echo '--- nothing builds for this host; running the js/wasm tests instead'; \
		$(MAKE) --no-print-directory -f $(THIS) test-wasm; \
	fi
	$(recurse)

# The exec wrapper Go ships for running a js/wasm binary under Node. It is what
# makes `go test` work for code the host cannot build at all.
# The wrapper moved from misc/wasm to lib/wasm in Go 1.24, and CI may install
# either side of that, so both are tried. Empty means this toolchain cannot
# run js/wasm tests at all.
WASMEXEC = $(shell for d in lib misc; do p="$$(go env GOROOT)/$$d/wasm/go_js_wasm_exec"; if [ -x "$$p" ]; then echo "$$p"; break; fi; done)

test-wasm: ## Run the js/wasm tests under Node
	@# One shell for the whole target. An `exit 0` in a recipe line of its own
	@# ends that line and nothing else, so the earlier form announced it was
	@# skipping and then ran the tests regardless, against the exec wrapper it
	@# had just reported missing.
	@# Node has no DOM: document, window and requestAnimationFrame are all
	@# undefined. JS core is there, and a fake document can be installed from
	@# Go with js.Global().Set, which is how the DOM-facing code is covered.
	@if [ -z "$(WASMEXEC)" ]; then \
		echo 'no js/wasm exec wrapper in this Go installation; skipping'; \
	elif ! command -v node >/dev/null; then \
		echo 'node is not installed; skipping'; \
	elif [ -n "$(JSPKGS)" ]; then \
		CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} go test -exec="$(WASMEXEC)" $(JSPKGS); \
	else \
		echo 'no js/wasm packages'; \
	fi
	$(recurse)

# Running the js/wasm tests in a real browser instead of Node. Node has no DOM;
# a browser has one, plus canvas and WebGL. Tests that build a fake DOM should
# detect a real one and use it, so the same tests run both ways — anything the
# fake gets wrong then shows up as a test that passes under Node and fails here.
#
# CHROME may name any Chrome-compatible binary. Brave is one. Not called BROWSER:
# that is a conventional environment variable (xdg uses it) and ?= would take
# whatever it happens to be set to — firefox, on this machine.
CHROME ?= $(shell command -v google-chrome chromium chromium-browser brave 2>/dev/null | head -1)

test-browser: ## Run the js/wasm tests in a headless browser (needs wasmbrowsertest)
	@command -v wasmbrowsertest >/dev/null || { \
		echo 'wasmbrowsertest is not installed; go install github.com/agnivade/wasmbrowsertest@latest'; exit 0; }
	@[ -n "$(CHROME)" ] || { echo 'no Chrome-compatible browser found (set CHROME=); skipping'; exit 0; }
	@if [ -z "$(JSPKGS)" ]; then echo 'no js/wasm packages'; exit 0; fi
	@# chromedp looks for a binary called google-chrome and does not pass
	@# --no-sandbox, which some environments require. A shim supplies both.
	@d=$$(mktemp -d); printf '#!/bin/sh\nexec %s --no-sandbox "$$@"\n' '$(CHROME)' > $$d/google-chrome; \
		chmod +x $$d/google-chrome; \
		PATH=$$d:$$PATH CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} \
			go test -exec=wasmbrowsertest $(JSPKGS); \
		rc=$$?; rm -rf $$d; exit $$rc
	$(recurse)

cover: ## Report test coverage per package
	@if [ -n "$(PKGS)" ]; then \
		CGO_ENABLED=$(CGO) ${OPTS} go test -cover $(PKGS) 2>&1 | grep -v '^ok.*no test files'; \
	fi
	$(recurse)

check: lint vet test readme-check check-names check-inits ## Run linters, vet, tests and the guards

install-linters: ## Install the linters
	${OPTS} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	${OPTS} go install golang.org/x/tools/cmd/goimports@latest

docs: ## Regenerate the dependency graph and code counts in the README
	./gendocs.sh

site: ## Regenerate models/ — one readable page per model — and sitemap.xml
	@# The app is one URL that draws every model onto a canvas, so the only
	@# thing outside it that can name a model is a page that says so in text.
	@# Same attractor.Catalog() the README and the selector knobs come from:
	@# adding a mode adds a page, and `site-check` fails when it has not.
	go run . uitool site

readme-check: ## Report whether README.md's generated regions are up to date
	@# The models section is built from attractor.Catalog() and the same
	@# descriptions the Info window shows, and the modules section from each
	@# module header's own tooltip. Nothing checked that the file still matched
	@# them, so it had drifted: the app's descriptions had been rewritten and
	@# trimmed underneath it, and the modules section listed 23 of the rack's
	@# 35 modules. Both are the kind of drift nobody notices by reading.
	go run . uitool readme --check

site-check: ## Report whether models/ is up to date; change nothing
	go run . uitool site --site-check

wasm: ## Rebuild the embedded Go wasm (assets/gowasm/chaosrack.wasm)
	CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} go build -o assets/gowasm/chaosrack.wasm .
	@cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" assets/gowasm/wasm_exec.js
	@ls -l assets/gowasm/chaosrack.wasm
	@# Recorded so a stale artifact fails a test instead of shipping quietly.
	@go run ./cmd/wasmstamp > assets/gowasm/built-from.txt

# The page, driven: every model and every switch in a private headless
# browser, against a server of its own, with nothing on screen and fresh
# storage. Fails on a page error, a freeze, a bay that overflows, or a
# listener freed while still attached (see pkg/uitool/sweep.go). Built from
# the current source first, since the server serves the embedded wasm.
# SWEEP= passes flags on: SWEEP='--models custom,lorenz --switches=false'.
# LINT= likewise for lint-panel: LINT='--models globe,lorenz'.
sweep: wasm ## Walk every model and switch in a headless browser (nothing on screen)
	@d=$$(mktemp -d); trap 'rm -rf $$d' EXIT; \
		go build -o $$d/chaosrack . && \
		$$d/chaosrack uitool sweep --headless --serve $$d/chaosrack $(SWEEP)

HL_AIZAWA := --set trail-slider=15277 --set rainbow-freq=14.0022 --set gradient-source=3 --set gradient-colors=4 --set auto-rotate=off --set aizawa-dt=0.006 --set aizawa-a=0.85 --set aizawa-b=0.654 --set aizawa-c=0.623 --set aizawa-d=3.07 --set aizawa-e=0.236 --set aizawa-f=0.02
highlights: ## Render the README's highlights reel to docs/img/gif/highlights.gif (then make publish-gifs)
	@# Five 4 s scenes drawn by the rack, crossfaded. The Aizawa values are
	@# the end-on mandala from docs/aizawa-harmonics.md.
	@d=$$(mktemp -d); trap 'rm -rf $$d' EXIT; \
		go build -o $$d/chaosrack . && \
		r="$$d/chaosrack render --engine rack --width 960 --height 540 --fps 30 --duration 4" && \
		$$r --model thomas --set bg-visual=termanim --set gradient-colors=10 --set gradient-source=3 -o $$d/1.mp4 && \
		$$r --model aizawa $(HL_AIZAWA) -o $$d/2.mp4 && \
		$$r --model magnetosphere --set gradient-colors=4 --set gradient-source=3 --set auto-rotate=on --set line-width=2 -o $$d/3.mp4 && \
		$$r --model clifford --set gradient-colors=8 --set gradient-source=0 --set trail-slider=200000 -o $$d/4.mp4 && \
		$$r --model halvorsen --set phosphor=4 --set persist-trail=on --set auto-rotate=on -o $$d/5.mp4 && \
		ffmpeg -loglevel error -y -i $$d/1.mp4 -i $$d/2.mp4 -i $$d/3.mp4 -i $$d/4.mp4 -i $$d/5.mp4 -filter_complex \
			"[0][1]xfade=duration=0.6:offset=3.4[a];[a][2]xfade=duration=0.6:offset=6.8[b];[b][3]xfade=duration=0.6:offset=10.2[c];[c][4]xfade=duration=0.6:offset=13.6,fps=12,scale=640:-1:flags=lanczos,split[x][y];[x]palettegen=max_colors=192:stats_mode=diff[p];[y][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
			-loop 0 docs/img/gif/highlights.gif && \
		ls -l docs/img/gif/highlights.gif

publish-gifs: ## Put the GIFs (gallery and model loops) and videos on the media branch and redeploy Pages
	@# None of them is on main (see .gitignore). media is ONE commit, replaced
	@# each time rather than added to, so remaking the GIFs never grows the
	@# repository; Pages adds them to the site (pages.yml). The commit starts
	@# from what media holds, so publishing one set keeps the others.
	@ls docs/img/gif/*.gif docs/img/model/*.gif docs/video/*.mp4 2>/dev/null | grep -q . || { echo "no GIFs: run chaosrack uitool gifs or uitool portraits first"; exit 1; }
	@git fetch -q origin media
	@idx=$$(mktemp -u); trap 'rm -f $$idx' EXIT; \
		GIT_INDEX_FILE=$$idx git read-tree FETCH_HEAD && \
		for g in 'docs/img/gif/*.gif' 'docs/img/model/*.gif' 'docs/video/*.mp4'; do \
			if ls $$g >/dev/null 2>&1; then GIT_INDEX_FILE=$$idx git add -f $$g || exit 1; fi; \
		done && \
		tree=$$(GIT_INDEX_FILE=$$idx git write-tree) && \
		commit=$$(git commit-tree $$tree -m "media: the animated gallery, the models' loops and the videos") && \
		git push -f origin $$commit:refs/heads/media && \
		gh workflow run pages.yml --ref main

lint-panel: wasm ## Measure the panel for faults (overflow, overlap, off-center, tooltips) on every model, headless
	@d=$$(mktemp -d); trap 'rm -rf $$d' EXIT; \
		go build -o $$d/chaosrack . && \
		$$d/chaosrack uitool lint --headless --serve $$d/chaosrack $(LINT)

tinywasm: ## Rebuild the embedded TinyGo wasm (assets/tinywasm/chaosrack-tiny.wasm)
	@# Recorded here because it was recorded NOWHERE: not the Makefile, not a
	@# script, not the README — while README.md advertises the TinyGo build as
	@# live on GitHub Pages. The committed asset had therefore gone stale by
	@# days against the Go one, which nothing would have caught, because the
	@# only way to rebuild it was to remember the command.
	@command -v tinygo >/dev/null || { echo "tinygo not installed"; exit 1; }
	@# TinyGo trails each new Go release by some weeks; until it catches up,
	@# building against the newer Go fails outright. The helper reports the
	@# newest Go this TinyGo accepts, or "auto" once the system one will do.
	@# -opt=2 rather than TinyGo's default of -opt=z. Size is not what is
	@# being worked on at the moment and the size setting is expensive: at
	@# -opt=z this build takes about 23 minutes, of which some 16 are LLVM
	@# running essentially single-threaded and about 6 are Binaryen's
	@# asyncify pass. Every fix that has to be seen in the browser costs one
	@# of those. Put -opt=z back when the behaviour is settled and size is
	@# the thing being tuned again; the measured cost of doing so belongs in
	@# this comment when it is.
	@# The hash is taken BEFORE the build and written after it succeeds. Taken
	@# after, it records whatever the sources say when the compiler finishes,
	@# and a TinyGo build here runs for tens of minutes: edit a file in that
	@# window and the stamp certifies an artifact that was never built from it.
	@# That happened, and the staleness test passed on it.
	@stamp=$$(go run ./cmd/wasmstamp) && \
		GOTOOLCHAIN=$$(sh scripts/tinygo-toolchain.sh) \
			tinygo build -o assets/tinywasm/chaosrack-tiny.wasm -target wasm -no-debug -opt=2 . && \
		printf '%s\n' "$$stamp" > assets/tinywasm/built-from.txt
	@cp "$$(tinygo env TINYGOROOT)/targets/wasm_exec.js" assets/tinywasm/tinygo_wasm_exec.js
	@sh scripts/fix-sleepticks.sh assets/tinywasm/tinygo_wasm_exec.js
	@ls -l assets/tinywasm/chaosrack-tiny.wasm

metersworker: ## Rebuild the analyzers' worker wasm (assets/metersworker/meters.wasm)
	@# The analyzers alone, for the Web Worker. Built out of pkg/meters rather
	@# than pkg/attractor, which is the whole reason that package exists: the
	@# same code compiled with the panel attached is 23 MB instead of 4.
	CGO_ENABLED=0 GOOS=js GOARCH=wasm ${OPTS} go build -o assets/metersworker/meters.wasm ./cmd/wasmmeters
	@ls -l assets/metersworker/meters.wasm

wasms: wasm tinywasm metersworker ## Rebuild every embedded wasm binary

CATALOG_PAGE ?= http://127.0.0.2:8080
catalog: ## Capture pkg/rackcatalog from a page: chaosrack serve, open $(CATALOG_PAGE)/ fresh, then run this
	@# A fresh page, in an origin that has never run chaosrack (127.0.0.2 beside
	@# the usual 127.0.0.1): what it opens with is every control's default in
	@# the native rack, and an origin keeps its last session's settings.
	go run . uitool catalog --catalog-server $(CATALOG_PAGE)

pages: ## Regenerate the deployed index.html / go/index.html / tinygo/index.html
	@# These FETCH the wasm from assets/, which is where it already is in this
	@# repository and therefore on GitHub Pages. That makes them tens of
	@# kilobytes instead of megabytes, lets instantiateStreaming compile the
	@# module while it downloads, and stops the dual page transferring both
	@# runtimes to run one of them. `make onefile` builds the other form.
	@# The serverless pages are the served pages, saved. They carry their own
	@# copy of the boot script, so a fix to assets/index.tmpl.html reaches the
	@# GitHub Pages deployment only by regenerating them — which is why the
	@# #gocanvas boot race outlived its fix on the served page.
	@# go/index.html is deployed and routed like the other two, and was the one
	@# this target forgot: a template fix reached / and /tinygo/ and left /go/
	@# on whatever it was built from, which is the same drift by a quieter road.
	@# And refuse to run against a server that is already there. The build
	@# compiles a 23 MB embedded asset and takes minutes, long enough to start
	@# a second one, have it fail to bind, and have the curls below fetch three
	@# pages from the FIRST server's older binary and report success. That is
	@# how a template fix and a payload fix both got saved as pages built
	@# without them. Hence the check after the build, just before the serve.
	@# Built and then run, not go run: killing go run leaves the server it
	@# started serving, which held the port against the next run and held
	@# open the output of anything this was piped into.
	@port=$${PAGES_PORT:-8399}; bin=$$(mktemp -d)/chaosrack; \
	trap "rm -rf $${bin%/*}" EXIT; \
	go build -o $$bin . || exit 1; \
	if curl -sf -o /dev/null -m 2 "http://127.0.0.1:$$port/" 2>/dev/null; then \
		echo "pages: something is already serving 127.0.0.1:$$port — stop it, or set PAGES_PORT" >&2; \
		exit 1; \
	fi; \
	$$bin serve -p $$port & srv=$$!; \
	trap "kill $$srv 2>/dev/null; rm -rf $${bin%/*}" EXIT INT TERM; \
	for i in $$(seq 1 300); do \
		curl -sf -o /dev/null "http://127.0.0.1:$$port/" && break || sleep 1; \
	done; \
	curl -sf "http://127.0.0.1:$$port/" -o index.html && echo 'pages: index.html'; \
	mkdir -p go && \
	curl -sf "http://127.0.0.1:$$port/go/" -o go/index.html && echo "pages: go/index.html"; \
	mkdir -p tinygo && \
	curl -sf "http://127.0.0.1:$$port/tinygo/" -o tinygo/index.html && echo "pages: tinygo/index.html"; \
	kill $$srv 2>/dev/null; wait $$srv 2>/dev/null || true

ONEFILE ?= chaosrack-standalone.html

onefile: ## Build the single self-contained HTML file (wasm carried inside it)
	@# The whole app in one file: no server, no sibling files, nothing to fetch.
	@# Save it, mail it, open it off a disk. That is what --inline is for, and it
	@# is the reason the inlining code is still there rather than deleted along
	@# with the payload the deployed pages no longer carry.
	@#
	@# Both runtimes, because a file you were given should not also need to be
	@# the right one: ?wasm=tinygo still switches inside it.
	@#
	@# Built and then run, as pages does and for its reason: killing go run
	@# leaves the server it started. This target wrote the file and then never
	@# returned, with that server still on the port.
	@port=$${PAGES_PORT:-8399}; bin=$$(mktemp -d)/chaosrack; \
	trap "rm -rf $${bin%/*}" EXIT; \
	go build -o $$bin . || exit 1; \
	if curl -sf -o /dev/null -m 2 "http://127.0.0.1:$$port/" 2>/dev/null; then \
		echo "onefile: something is already serving 127.0.0.1:$$port — stop it, or set PAGES_PORT" >&2; \
		exit 1; \
	fi; \
	$$bin serve -p $$port --inline & srv=$$!; \
	trap "kill $$srv 2>/dev/null; rm -rf $${bin%/*}" EXIT INT TERM; \
	for i in $$(seq 1 300); do \
		curl -sf -o /dev/null "http://127.0.0.1:$$port/" && break || sleep 1; \
	done; \
	curl -sf "http://127.0.0.1:$$port/" -o "$(ONEFILE)" && \
		echo "onefile: $(ONEFILE) ($$(du -h '$(ONEFILE)' | cut -f1))"; \
	kill $$srv 2>/dev/null; wait $$srv 2>/dev/null || true

# The naming and init rules of AGENTS.md (github.com/0magnet/guard).
GUARD ?= go run github.com/0magnet/guard@main

check-names: ## Fail on a new generic name, generic package or is/has bool (AGENTS.md Naming; scripts/guard/names-allowlist.txt only shrinks)
	$(GUARD) names

check-inits: ## Fail on a new func init() outside package main (scripts/guard/init-allowlist.txt only shrinks)
	$(GUARD) inits
