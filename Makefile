.PHONY: build sindri gh image clean

build: sindri gh

sindri:
	go build -o bin/sindri ./cmd/sindri/

gh:
	go build -o bin/gh ./cmd/gh/

# Rebuild image when container files, gh source, or go.mod change
CONTAINER_DEPS := $(shell find container cmd/gh internal/ghlocal -type f 2>/dev/null) go.mod go.sum
.image-stamp: $(CONTAINER_DEPS)
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .
	touch .image-stamp

image: .image-stamp

all: build image

clean:
	rm -f bin/sindri bin/gh bin/td bin/yq .image-stamp
