.PHONY: build sindri gh image install clean

PREFIX := $(HOME)/.local/bin

build: sindri gh

sindri:
	go build -o bin/sindri ./cmd/sindri/

gh:
	go build -o bin/sindri-gh ./cmd/gh/

install: build
	mkdir -p $(PREFIX)
	cp bin/sindri $(PREFIX)/sindri
	cp bin/sindri-gh $(PREFIX)/sindri-gh

# Rebuild image when container files change (gh is now mounted, not built in image)
CONTAINER_DEPS := $(shell find container -type f 2>/dev/null)
.image-stamp: $(CONTAINER_DEPS)
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .
	touch .image-stamp

image: .image-stamp

all: build image install

clean:
	rm -f bin/sindri bin/sindri-gh bin/gh bin/td bin/yq .image-stamp
