.PHONY: build sindri worker image install clean test lint demo diag

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

lint: sindri
	./bin/sindri lint all

# End-to-end hub demo / diagnostic in a throwaway repo (needs podman + image).
demo: build
	./scripts/devhub.sh demo

diag: build
	./scripts/devhub.sh diag

all: build image install

clean:
	rm -f bin/sindri bin/sindri-worker bin/td bin/yq .image-stamp
