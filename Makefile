.PHONY: build sindri gh image clean

build: sindri gh

sindri:
	go build -o bin/sindri ./cmd/sindri/

gh:
	go build -o bin/gh ./cmd/gh/

image:
	cp "$$(which td)" bin/td
	cp "$$(which yq)" bin/yq
	podman build -t sindri-agent:test -f container/Dockerfile .

clean:
	rm -f bin/sindri bin/gh bin/td bin/yq
