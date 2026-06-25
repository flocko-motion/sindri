.PHONY: build sindri worker image install clean test lint check demo diag loop claude-check fullloop screenshot deb release major minor patch

PREFIX := $(HOME)/.local/bin

# Packaging. VERSION is derived from the latest git tag (overridden by CI with the
# exact tag, e.g. `make deb VERSION=1.2.3`); dashes are flattened so a
# describe-style "0.1.0-3-gabc" stays a valid deb version. ARCH is the Go target.
VERSION ?= $(shell v=$$(git describe --tags --dirty 2>/dev/null); echo "$${v:-v0.0.0}" | sed 's/^v//; s/-/./g')
ARCH    := $(shell go env GOARCH)
NFPM    := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

build: sindri worker

sindri:
	go build -o bin/sindri ./cmd/sindri/

# The single, role-agnostic agent browser (was sindri-worker + sindri-review).
worker:
	go build -o bin/sindri-worker ./cmd/sindri-worker/

install: build
	mkdir -p $(PREFIX)
	# Use mv rather than cp so install succeeds even when the previous
	# binary is currently running (rename unlinks the in-use file; the
	# running process keeps executing the memory-mapped inode unharmed).
	mv bin/sindri $(PREFIX)/sindri
	mv bin/sindri-worker $(PREFIX)/sindri-worker

# Rebuild image when container files change (agent CLI is mounted, not built in image)
CONTAINER_DEPS := $(shell find container -type f 2>/dev/null)
.image-stamp: $(CONTAINER_DEPS)
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .
	touch .image-stamp

image: .image-stamp

test:
	go test ./...

# Render the TUI headlessly (mock data) so its layout can be eyeballed.
screenshot:
	go test ./internal/tui/ -run Screenshot -v

# Seed a mock task hierarchy into the current repo's td store (titles "Mock:…").
seed:
	./scripts/seed.sh

lint: sindri
	./bin/sindri lint all

# One-shot quality gate with terse output: build + test + lint, each reporting
# PASS or printing the tail of its failure. Stops at the first failure.
check: sindri
	@out=$$(go build ./... 2>&1) && echo "BUILD OK" || { echo "BUILD FAIL"; echo "$$out" | tail -20; exit 1; }
	@out=$$(go test ./... 2>&1) && echo "TESTS PASS" || { echo "TESTS FAIL"; echo "$$out" | tail -30; exit 1; }
	@out=$$(./bin/sindri lint all 2>&1) && echo "LINT PASS" || { echo "LINT FAIL"; echo "$$out" | tail -40; exit 1; }

# End-to-end hub demo / diagnostic in a throwaway repo (needs podman + image).
demo: build
	./scripts/devhub.sh demo

diag: build
	./scripts/devhub.sh diag

# Full worker loop: task -> next -> submit -> approve -> merge -> notify.
loop: build
	./scripts/devhub.sh loop

# Launch a REAL Claude worker (uses your ~/.claude credentials + API tokens).
claude-check: build
	./scripts/devhub.sh claude

# Full autonomous loop with two real Claude agents (worker + reviewer).
fullloop: build
	./scripts/devhub.sh fullloop

all: build image install

# Build the .deb: the binaries we ship (sindri, sindri-worker) plus the bundled
# tools (td, yq) staged from PATH, packaged via nfpm. git/podman are declared as
# apt dependencies, not bundled. Run after `go install`ing td/yq (CI does this).
deb: build
	cp "$$(command -v td)" bin/td
	cp "$$(command -v yq)" bin/yq
	VERSION="$(VERSION)" ARCH="$(ARCH)" $(NFPM) pkg --config nfpm.yaml --packager deb --target bin/
	@echo "built .deb in bin/ (version $(VERSION), arch $(ARCH))"

# Cut a release: bump the latest semver tag and push it; the release workflow then
# builds and attaches the .deb. Usage: make release <major|minor|patch>.
release:
	@./scripts/release.sh $(filter major minor patch,$(MAKECMDGOALS))
major minor patch:
	@:

clean:
	rm -f bin/sindri bin/sindri-worker bin/td bin/yq bin/*.deb .image-stamp
