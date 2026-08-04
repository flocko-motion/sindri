.PHONY: help build sindri worker brokkr brokkr-linux image install clean test verify lint check-go upgrade-go check demo diag loop claude-check fullloop screenshot seed tarball release major minor patch breaking feature fix

.DEFAULT_GOAL := help

PREFIX := $(HOME)/.local/bin

# Packaging. VERSION is derived from the latest git tag (overridden by CI with the
# exact tag, e.g. `make tarball VERSION=1.2.3`); dashes are flattened so a
# describe-style "0.1.0-3-gabc" stays a clean version string. ARCH/GOOS are the Go target.
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
GOOS    := $(shell go env GOOS)

# When this build was linked (RFC 3339, UTC) for `brokkr version`. Go's build info records
# the COMMIT time and never the build's own, so without stamping it there is no way to tell
# a fresh binary from one built days ago off the same commit. Kept OUT of VERSION on
# purpose: the hub compares version strings to spot a stale running hub, and a timestamp
# would make every rebuild of identical source look new, so the check would cry wolf.
# `date -u` is portable to macOS's BSD date.
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

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
	go build -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o bin/brokkr ./cmd/brokkr/

# brokkr-linux — a linux/$(ARCH) cross-build of brokkr, mounted into the (always
# linux) agent pods so `brokkr` works inside agents on any host. On a linux host
# it's identical to bin/brokkr; on macOS it's the only pod-runnable brokkr. Pure
# Go, so CGO_ENABLED=0 keeps the cross-build hermetic (mirrors the worker target).
brokkr-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o bin/brokkr-linux ./cmd/brokkr/

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

seed: ## seed a mock task hierarchy into the current repo (via sindri task new)
	./scripts/seed.sh

verify: check-go brokkr ## build + test + lint (deadcode, loc, comments, openspec) — the quality gate
	go build ./...
	go test ./...
	./bin/brokkr lint

lint: verify ## alias for verify

check-go: ## fail unless the active Go toolchain is the latest release (linters need current Go)
	@./scripts/check-go.sh

# The fix for a failing check-go. Deliberately NOT wired into check-go/verify:
# `go get go@latest` rewrites the `go` directive in go.mod, and a check that
# silently mutates tracked source produces surprise diffs mid-`make verify` and
# a dirty tree in CI. Bumping the module's minimum Go is an explicit decision.
#
# This does not touch /usr/local/go — with GOTOOLCHAIN=auto the raised `go`
# directive is enough: the go command downloads the matching toolchain and
# re-execs into it, so the base install only has to bootstrap the switch.
#
# Agent pods do NOT get that for free: the golang base image pins
# GOTOOLCHAIN=local, so raising the directive past the image's Go breaks every go
# command in the pod until it runs `go-upgrade` (in the image) or gets a newer base
# via `sindri agent rebuild`. `brokkr lint deadcode` says as much when it hits it.
upgrade-go: ## bump the go directive to the latest release, tidy, and rebuild
	go get go@latest
	go mod tidy
	go build ./...
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

# One artifact shape for every OS: a tarball that install.sh unpacks into ~/.local/bin.
# There is deliberately no .deb — a system package installs to /usr/bin, which then
# shadows (or is shadowed by) the ~/.local/bin install depending on PATH order, and the
# two drift apart silently. One location means one build can ever be in play.
tarball: build ## build the release tarball into dist/ (binaries + bundled yq + install.sh)
	cp "$$(command -v yq)" bin/yq
	rm -rf "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH)"
	mkdir -p "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH)"
	cp bin/sindri bin/sindri-worker bin/brokkr bin/brokkr-linux bin/yq \
	   LICENSE THIRD_PARTY_LICENSES.md "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH)/"
	cp scripts/install.sh "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH)/install.sh"
	chmod +x "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH)/install.sh"
	tar -C dist -czf "dist/sindri_$(VERSION)_$(GOOS)_$(ARCH).tar.gz" "sindri_$(VERSION)_$(GOOS)_$(ARCH)"
	@echo "built dist/sindri_$(VERSION)_$(GOOS)_$(ARCH).tar.gz"

release: ## cut a release (validates arg, then lints): make release <major|minor|patch> (breaking|feature|fix too)
	@./scripts/release.sh $(filter major minor patch breaking feature fix,$(MAKECMDGOALS))
major minor patch breaking feature fix:
	@:

clean: ## remove build artifacts (bin/ binaries, dist/ tarballs, image stamp)
	rm -rf bin/sindri bin/sindri-worker bin/brokkr bin/brokkr-linux bin/yq bin/buildctx dist .image-stamp
