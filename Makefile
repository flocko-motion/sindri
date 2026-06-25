.PHONY: help build sindri worker brokkr image install clean test verify lint check-go check demo diag loop claude-check fullloop screenshot seed deb release major minor patch breaking feature fix

.DEFAULT_GOAL := help

PREFIX := $(HOME)/.local/bin

# Packaging. VERSION is derived from the latest git tag (overridden by CI with the
# exact tag, e.g. `make deb VERSION=1.2.3`); dashes are flattened so a
# describe-style "0.1.0-3-gabc" stays a valid deb version. ARCH is the Go target.
VERSION ?= $(shell v=$$(git describe --tags --dirty 2>/dev/null); echo "$${v:-v0.0.0}" | sed 's/^v//; s/-/./g')
ARCH    := $(shell go env GOARCH)
NFPM    := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

help: ## list the available targets
	@echo "make targets:"
	@grep -hE '^[a-zA-Z][a-zA-Z_-]*:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: sindri worker brokkr ## build all binaries (sindri, sindri-worker, brokkr) into bin/

sindri:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/sindri ./cmd/sindri/

# The single, role-agnostic agent browser (was sindri-worker + sindri-review).
worker:
	go build -o bin/sindri-worker ./cmd/sindri-worker/

# brokkr — the toolbelt: code map + linters, no orchestration.
brokkr:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/brokkr ./cmd/brokkr/

install: build ## build, then install the binaries to ~/.local/bin
	mkdir -p $(PREFIX)
	# Use mv rather than cp so install succeeds even when the previous
	# binary is currently running (rename unlinks the in-use file; the
	# running process keeps executing the memory-mapped inode unharmed).
	mv bin/sindri $(PREFIX)/sindri
	mv bin/sindri-worker $(PREFIX)/sindri-worker
	mv bin/brokkr $(PREFIX)/brokkr

# Rebuild image when container files change (agent CLI is mounted, not built in image)
CONTAINER_DEPS := $(shell find container -type f 2>/dev/null)
.image-stamp: $(CONTAINER_DEPS)
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .
	touch .image-stamp

image: .image-stamp ## build the agent container image (needs podman)

test: ## run the Go test suite
	go test ./...

screenshot: ## render the TUI headlessly (mock data) to eyeball its layout
	go test ./internal/tui/ -run Screenshot -v

seed: ## seed a mock task hierarchy into the current repo's td store
	./scripts/seed.sh

verify: check-go brokkr ## run the linters (deadcode, loc, comments, openspec) — the quality gate
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
	rm -f bin/sindri bin/sindri-worker bin/brokkr bin/td bin/yq bin/*.deb .image-stamp
