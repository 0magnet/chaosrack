GOROOT := $(shell go env GOROOT)
TINYGOROOT := $(shell tinygo env TINYGOROOT)
PORT ?= 8080

.PHONY: all update generate wasms build pages clean tidy monkey uigolden

all: update generate build pages

update:
	go get -v -u
	go mod tidy
	go mod vendor

# wasms rebuilds both WebAssembly binaries and their wasm_exec.js runtimes
# into their embed sub-packages: assets/gowasm (standard Go) and
# assets/tinywasm (TinyGo), each //go:embed-ed independently.
wasms:
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" assets/gowasm/wasm_exec.js
	cp "$(TINYGOROOT)/targets/wasm_exec.js" assets/tinywasm/tinygo_wasm_exec.js
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o assets/gowasm/b.wasm ./cmd/wasm
	tinygo build -target wasm -no-debug -o assets/tinywasm/b-tiny.wasm ./cmd/wasm

# generate is an alias for wasms (kept for `make all`).
generate: wasms

build: generate
	go build -o wasm-stuff .

pages: build
	@echo "Starting server to generate pages..."
	@./wasm-stuff -p $(PORT) & PID=$$!; \
	sleep 2; \
	curl -sf http://127.0.0.1:$(PORT)/index.html -o index.html && \
	mkdir -p tinygo go && \
	curl -sf http://127.0.0.1:$(PORT)/tinygo/index.html -o tinygo/index.html && \
	curl -sf http://127.0.0.1:$(PORT)/go/index.html -o go/index.html && \
	echo "Generated: index.html (dual), go/index.html, tinygo/index.html"; \
	kill $$PID 2>/dev/null

tidy:
	go mod tidy
	go mod vendor

clean:
	rm -rf assets/gowasm/b.wasm assets/gowasm/wasm_exec.js assets/tinywasm/b-tiny.wasm assets/tinywasm/tinygo_wasm_exec.js wasm-stuff index.html tinygo/

# Random-walk UI fuzzer. Point a Chromium/Brave at the running app with
# --remote-debugging-port=9222, open the app tab, then: make monkey
# Extra flags via ARGS, e.g. make monkey ARGS="-seed 7 -steps 200 -delay 0"
monkey:
	go run ./cmd/uitool monkey $(ARGS)

# Golden/oracle UI checker (render sanity + permalink round-trip + golden diff).
# make uigolden ARGS="-capture"  to (re)write reference images.
uigolden:
	go run ./cmd/uitool golden $(ARGS)

# Lint both build configurations: the default (host) pass never loads the
# js/wasm-tagged files — most of the app — so a second GOOS=js pass covers
# them. pkg/attractor + pkg/audiosrc are linted in the wasm pass (their
# portable files load there too; linting them natively false-positives
# "unused" on anything whose only users are wasm-tagged).
lint:
	golangci-lint run ./cmd/audiows/ ./cmd/uitool/ ./internal/... ./pkg/server/ .
	GOOS=js GOARCH=wasm golangci-lint run ./pkg/attractor/ ./pkg/audiosrc/ ./cmd/wasm/
