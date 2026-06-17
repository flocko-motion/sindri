.PHONY: build sindri worker image install clean test lint demo diag loop claude-check fullloop screenshot

PREFIX := $(HOME)/.local/bin

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

clean:
	rm -f bin/sindri bin/sindri-worker bin/td bin/yq .image-stamp
