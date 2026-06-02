.PHONY: build sindri worker review image install clean

PREFIX := $(HOME)/.local/bin

build: sindri worker review

sindri:
	go build -o bin/sindri ./cmd/sindri/

worker:
	go build -o bin/sindri-worker ./cmd/sindri-worker/

review:
	go build -o bin/sindri-review ./cmd/sindri-review/

install: build
	mkdir -p $(PREFIX)
	# Use mv rather than cp so install succeeds even when the previous
	# binary is currently running (rename unlinks the in-use file; the
	# running process keeps executing the memory-mapped inode unharmed).
	mv bin/sindri $(PREFIX)/sindri
	mv bin/sindri-worker $(PREFIX)/sindri-worker
	mv bin/sindri-review $(PREFIX)/sindri-review

# Rebuild image when container files change (agent CLIs are mounted, not built in image)
CONTAINER_DEPS := $(shell find container -type f 2>/dev/null)
.image-stamp: $(CONTAINER_DEPS)
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .
	touch .image-stamp

image: .image-stamp

all: build image install

clean:
	rm -f bin/sindri bin/sindri-worker bin/sindri-review bin/td bin/yq .image-stamp
