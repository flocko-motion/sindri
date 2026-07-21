.PHONY: help build sindri worker brokkr brokkr-linux image install clean test verify lint check-go check demo diag loop claude-check fullloop screenshot seed deb release major minor patch breaking feature fix

.DEFAULT_GOAL := help

PREFIX := $(HOME)/.local/bin

# Packaging. VERSION is derived from the latest git tag (overridden by CI with the
# exact tag, e.g. `make deb VERSION=1.2.3`); dashes are flattened so a
# describe-style "0.1.0-3-gabc" stays a valid deb version. ARCH is the Go target.
#
# On a DIRTY tree, git's plain "-dirty" suffix is content-blind: every uncommitted
# state stamps the SAME version, so the hub's version check (reconcileHubVersion)
# can't tell a rebuilt binary from the stale one it's already running, and never
# offers the restart — you edit code, `make install`, and silently keep running the
# old hub (a real trap: it cost an afternoon chasing a "fix that didn't take"). Append
# a short hash of the uncommitted changes (tracked diff + untracked file list) so
# distinct working trees get distinct versions and the mismatch is detected. CI passes
# VERSION=<exact tag>, so this dev-only path never runs there. `shasum` is used (not
# sha1sum) for macOS; the `while`-free pipeline can't hang on an empty file list.
VERSION ?= $(shell \
	v=$$(git describe --tags --dirty 2>/dev/null); v=$${v:-v0.0.0}; \
	if [ "$${v%-dirty}" != "$$v" ]; then \
		h=$$( { git diff HEAD; git status --porcelain --untracked-files=all; } 2>/dev/null | shasum | cut -c1-8); \
		v="$$v.$$h"; \
	fi; \
	echo "$$v" | sed 's/^v//; s/-/./g')
ARCH    := $(shell go env GOARCH)
NFPM    := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

help: ## list the available targets
	@echo "make targets:"
	@grep -hE '^[a-zA-Z][a-zA-Z_-]*:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: sindri worker brokkr brokkr-linux ## build all binaries (sindri, sindri-worker, brokkr, brokkr-linux) into bin/

sindri:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/sindri ./cmd/sindri/

# The single, role-agnostic agent browser (was sindri-worker + sindri-review).
# It runs ONLY inside the Linux pod (mounted read-only at runtime), never on the
# host, so it's always built for linux/$(ARCH) — native on a Linux host, a cross-
# compile on macOS/Windows. Pure Go, so CGO_ENABLED=0 keeps the cross-build
# hermetic. ARCH is the host's Go arch, which matches the native podman VM.
worker:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -ldflags "-X main.version=$(VERSION)" -o bin/sindri-worker ./cmd/sindri-worker/

# brokkr — the toolbelt: code map + linters, no orchestration.
brokkr:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/brokkr ./cmd/brokkr/

# brokkr-linux — a linux/$(ARCH) cross-build of brokkr, mounted into the (always
# linux) agent pods so `brokkr` works inside agents on any host. On a linux host
# it's identical to bin/brokkr; on macOS it's the only pod-runnable brokkr. Pure
# Go, so CGO_ENABLED=0 keeps the cross-build hermetic (mirrors the worker target).
brokkr-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -ldflags "-X main.version=$(VERSION)" -o bin/brokkr-linux ./cmd/brokkr/

install: check-go build ## build (on the latest Go), then install the binaries to ~/.local/bin
	mkdir -p $(PREFIX)
	# Use mv rather than cp so install succeeds even when the previous
	# binary is currently running (rename unlinks the in-use file; the
	# running process keeps executing the memory-mapped inode unharmed).
	mv bin/sindri $(PREFIX)/sindri
	mv bin/sindri-worker $(PREFIX)/sindri-worker
	mv bin/brokkr $(PREFIX)/brokkr
	mv bin/brokkr-linux $(PREFIX)/brokkr-linux
	# A running hub keeps executing the OLD binary after the mv above (it swapped
	# the on-disk inode, but the live process is memory-mapped to the previous one).
	# Restart it so the rebuild actually takes effect — but ONLY when one is already
	# up; `make install` must never launch a hub the user didn't have. Keying off the
	# status table's "PID" header fails safe: if it's absent, we skip rather than risk
	# starting an unwanted hub.
	@if $(PREFIX)/sindri hub status 2>/dev/null | grep -q '^PID'; then \
		echo "restarting the running hub to pick up this build…"; \
		$(PREFIX)/sindri hub restart; \
	fi

# Rebuild image when the (now embedded) build context changes. The agent binary
# is mounted at runtime, not baked in; arch-specific tools (yq, yazi) are
# downloaded in-container by the Dockerfile, mirroring what container.Ensure does
# at launch.
CONTAINER_DEPS := $(shell find internal/container/buildctx -type f 2>/dev/null)
.image-stamp: $(CONTAINER_DEPS)
	rm -rf bin/buildctx
	cp -r internal/container/buildctx bin/buildctx
	podman build -t sindri-agent:test -f bin/buildctx/Dockerfile bin/buildctx
	touch .image-stamp

image: .image-stamp ## build the agent container image (needs podman)

test: ## run the Go test suite
	go test ./...

screenshot: ## render the TUI headlessly (mock data) to eyeball its layout
	go test ./internal/tui/ -run Screenshot -v

seed: ## seed a mock task hierarchy into the current repo's td store
	./scripts/seed.sh

verify: check-go brokkr ## build + test + lint (deadcode, loc, comments, openspec) — the quality gate
	go build ./...
	go test ./...
	./bin/brokkr lint

lint: verify ## alias for verify

check-go: ## fail unless the active Go toolchain is the latest release (linters need current Go)
	@./scripts/check-go.sh

check: brokkr ## terse one-shot gate: build + test + lint, stops at the first failure
	@out=$$(go build ./... 2>&1) && echo "BUILD OK" || { echo "BUILD FAIL"; echo "$$out" | tail -20; exit 1; }
	@out=$$(go test ./... 2>&1) && echo "TESTS PASS" || { echo "TESTS FAIL"; echo "$$out" | tail -30; exit 1; }
	@out=$$(./bin/brokkr lint 2>&1) && echo "LINT PASS" || { echo "LINT FAIL"; echo "$$out" | tail -40; exit 1; }

demo: build ## end-to-end hub demo in a throwaway repo (needs podman + image)
	./scripts/devhub.sh demo

diag: build ## hub diagnostic in a throwaway repo
	./scripts/devhub.sh diag

loop: build ## full worker loop demo: task -> next -> submit -> approve -> merge
	./scripts/devhub.sh loop

claude-check: build ## launch a REAL Claude worker (uses your ~/.claude credentials)
	./scripts/devhub.sh claude

fullloop: build ## full autonomous loop with two real Claude agents (worker + reviewer)
	./scripts/devhub.sh fullloop

all: build image install ## build everything (binaries + agent image) and install

deb: build ## build the .deb package into bin/ (bundles brokkr, td, yq)
	cp "$$(command -v td)" bin/td
	cp "$$(command -v yq)" bin/yq
	VERSION="$(VERSION)" ARCH="$(ARCH)" $(NFPM) pkg --config nfpm.yaml --packager deb --target bin/
	@echo "built .deb in bin/ (version $(VERSION), arch $(ARCH))"

release: ## cut a release (validates arg, then lints): make release <major|minor|patch> (breaking|feature|fix too)
	@./scripts/release.sh $(filter major minor patch breaking feature fix,$(MAKECMDGOALS))
major minor patch breaking feature fix:
	@:

clean: ## remove build artifacts (bin/ binaries, .deb, image stamp)
	rm -rf bin/sindri bin/sindri-worker bin/brokkr bin/brokkr-linux bin/td bin/yq bin/*.deb bin/buildctx .image-stamp
