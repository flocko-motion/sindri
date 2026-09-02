# golang:latest is Debian-slim-based with Go already correctly installed..
# Fully qualify the registry (docker.io/library/...) so the build works under
# podman even when the host has no unqualified-search-registries configured —
# a bare "golang:latest" makes podman error out with "short-name did not
# resolve to an alias and no unqualified-search registries are defined".
FROM docker.io/library/golang:latest

# ── Node.js LTS ───────────────────────────────────────────────────────────────
# The nodesource `nodejs` package bundles npm — do NOT also install Debian's
# standalone `npm` package: it Conflicts with nodesource nodejs and the build
# fails with "held broken packages" (npm is available after this either way).
RUN apt-get update && apt-get install -y --no-install-recommends \
  curl ca-certificates \
  && curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && rm -rf /var/lib/apt/lists/*


# ── Agent toolbox ─────────────────────────────────────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
  # build
  git make patch build-essential \
  # text processing
  grep sed gawk jq ripgrep \
  # better find / cat / fuzzy-finder  (fdfind→fd, batcat→bat symlinked below)
  fd-find bat fzf \
  # file / archive
  file tree zip unzip tar gzip bzip2 xz-utils \
  # network + remote fs (sshfs needs --cap-add SYS_ADMIN + /dev/fuse at run time)
  wget netcat-openbsd dnsutils iputils-ping openssh-client sshfs \
  # process / system
  procps lsof watch htop \
  # init: PID 1 must reap orphans, or every process whose parent died accumulates
  # as a zombie until the cgroup's pids.max stops the pod forking at all
  tini \
  # terminal multiplexer — the agent runs interactively inside a tmux session
  # that the hub injects into (send-keys) and humans attach to
  tmux \
  # shells / editors / file manager
  zsh mc nano \
  # python
  python3 python3-pip python3-venv python3-pytest \
  # misc
  less diffutils vim-tiny locales \
  && ln -s /usr/bin/fdfind /usr/local/bin/fd \
  && ln -s /usr/bin/batcat /usr/local/bin/bat \
  && sed -i 's/# en_US.UTF-8/en_US.UTF-8/' /etc/locale.gen && locale-gen \
  && rm -rf /var/lib/apt/lists/*

# ── yazi (TUI file manager — not in apt; prebuilt release binary) ──────────────
# Pick the release matching the build arch, so the image is correct whether it's
# built on amd64 or arm64 (e.g. an Apple Silicon podman VM).
RUN case "$(uname -m)" in \
  x86_64)  target=x86_64-unknown-linux-gnu ;; \
  aarch64) target=aarch64-unknown-linux-gnu ;; \
  *) echo "unsupported arch $(uname -m) for yazi" >&2; exit 1 ;; \
  esac; \
  curl -fsSL "https://github.com/sxyazi/yazi/releases/latest/download/yazi-$target.zip" -o /tmp/yazi.zip \
  && unzip -q /tmp/yazi.zip -d /tmp/yazi \
  && mv /tmp/yazi/*/yazi /tmp/yazi/*/ya /usr/local/bin/ \
  && chmod +x /usr/local/bin/yazi /usr/local/bin/ya \
  && rm -rf /tmp/yazi /tmp/yazi.zip

# ── yq (prebuilt release binary, arch-matched) ────────────────────────────────
# Downloaded in-container for the build arch rather than copied from the host:
# the host may be a different OS/arch (e.g. macOS) than the Linux pod, so a copied
# host binary wouldn't run. No task tool is present at all: an agent reaches tasks
# only through the hub, via the in-pod `sindri` command.
RUN case "$(uname -m)" in \
  x86_64)  arch=amd64 ;; \
  aarch64) arch=arm64 ;; \
  *) echo "unsupported arch $(uname -m) for yq" >&2; exit 1 ;; \
  esac; \
  curl -fsSL "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_$arch" -o /usr/local/bin/yq \
  && chmod +x /usr/local/bin/yq

# ── micromamba (standalone conda/mamba; bzip2 above is needed for `tar -xvj`) ───
# Arch-matched like yazi/yq — the image also builds on an arm64 podman VM.
RUN case "$(uname -m)" in \
  x86_64)  mamba=linux-64 ;; \
  aarch64) mamba=linux-aarch64 ;; \
  *) echo "unsupported arch $(uname -m) for micromamba" >&2; exit 1 ;; \
  esac; \
  curl -Ls "https://micro.mamba.pm/api/micromamba/$mamba/latest" \
  | tar -xvj -C /usr/local/bin --strip-components=1 bin/micromamba

# ── Go dev tools ──────────────────────────────────────────────────────────────
# gopls (the Go language server) so agents get semantic code intelligence —
# references/definition/rename — instead of blind text find/replace. `go install`
# builds it from source, so it's automatically the pod's arch (no download/arch-case).
# GOBIN=/usr/local/bin drops it where every other agent tool lives (fd/bat/yazi/yq/
# sindri) — on PATH for ALL shells, incl. a login shell (a dialed-in human) whose
# /etc/profile would otherwise reset PATH and hide GOPATH/bin.
RUN GOBIN=/usr/local/bin go install golang.org/x/tools/gopls@latest

# go-upgrade: install a chosen Go toolchain, when go.mod's own answer is not the one wanted
# — `go-upgrade latest`, or an exact version to pin. Reaching go.mod's version needs nothing
# now that GOTOOLCHAIN=auto is set below; this fetches into the agent's own tree and links
# into ~/.local/bin, which the PATH below puts first, since /usr/local/go is root-owned.
# brokkr's deadcode linter still names it for a refusal a custom image recipe can bring back.
COPY go-upgrade.sh /usr/local/bin/go-upgrade
RUN chmod +x /usr/local/bin/go-upgrade

# ── claude CLI ────────────────────────────────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest
RUN mkdir -p /opt/sindri && cd /opt/sindri && npm init -y && npm install "@anthropic-ai/claude-agent-sdk@^0.1.59"

# ── openspec CLI (spec-driven workflow, optional per project) ──────────────────
RUN npm install -g @fission-ai/openspec@latest

# ── tool-version manifest (sd-39dad3) ──────────────────────────────────────────
# The hub compares these against its own host's, to catch the gate and the agents quietly
# running different tools. Printed to the build log, which the hub already captures — never a
# file inside the image, never something read back by execing into a built container.
# A podman layer-cache hit on this RUN skips it silently, leaving the prior manifest (if any) in
# place rather than clearing it — a stale-but-present manifest reads as "nothing new to compare".
# The marker is assembled from two adjacent shell strings so it never appears whole in this RUN's
# own source text: podman/BuildKit echo that source verbatim as a "STEP N/M: RUN ..." line (or a
# cache-hit line replacing it) before ever running it, and a literal marker there would make a cache
# hit's echo of the instruction itself look like a real, freshly-printed reading.
RUN m="SINDRI-TOOL-""VERSION" \
  && echo "$m go $(go version | awk '{print $3}')" \
  && echo "$m node $(node --version)" \
  && echo "$m openspec $(openspec --version)"

# ── host-built tools (mounted from host at runtime) ──────────────────────────
# The hub mounts its pod-bin DIRECTORY read-only at /opt/sindri/host-bin. These
# symlinks resolve on every exec, so a reinstalled binary lands without
# recreating the container — a per-file bind used to pin the old inode.
#
# The browser is presented simply as `sindri` (in the pod it IS the interface to
# the hub, and no host CLI is here to collide). The sindri user can't write
# /usr/local/bin, so the links are made at build time.
RUN ln -sf /opt/sindri/host-bin/sindri-worker /usr/local/bin/sindri \
  && ln -sf /opt/sindri/host-bin/brokkr       /usr/local/bin/brokkr

# ── Shims ─────────────────────────────────────────────────────────────────────
# git: /usr/local/bin precedes /usr/bin, so this fronts the real git without replacing it.
# It passes everything through and only adds the `sindri git` pointer when a repo-touching
# command fails for want of a reachable repo — the sandbox dead end. A coauthor, whose
# /workspace IS the user's checkout, never sees it.
COPY shims/docker          /usr/local/bin/docker
COPY shims/docker-compose  /usr/local/bin/docker-compose
COPY shims/git             /usr/local/bin/git
RUN chmod +x /usr/local/bin/docker /usr/local/bin/docker-compose /usr/local/bin/git

# ── Sindri agent entrypoint ───────────────────────────────────────────────────
# The agent is a thin browser driven by the hub; its instructions come from the
# hub, not baked-in skills/CLAUDE files (those were the pre-hub worker's).
COPY sindri-agent.sh  /usr/local/bin/sindri-agent
RUN chmod +x /usr/local/bin/sindri-agent

# ── Non-root user (claude refuses --dangerously-skip-permissions as root) ──
RUN useradd -m -s /bin/bash sindri

# `y` yazi wrapper, sourced by interactive shells (attach / post-Claude bash).
COPY yazi.sh /etc/sindri/yazi.sh
RUN echo '. /etc/sindri/yazi.sh' >> /home/sindri/.bashrc

# `claude` wrapper that resets the terminal on exit, so a dialed-in human always
# has echo in the fallback shell (see shell.sh). Sourced by interactive shells.
COPY shell.sh /etc/sindri/shell.sh
RUN echo '. /etc/sindri/shell.sh' >> /home/sindri/.bashrc

USER sindri
# GOPATH, not just GOMODCACHE: the checksum-database cache lives under GOPATH/pkg/sumdb, and the
# base image's /go/pkg is root-owned — so fetching a toolchain died on "open /go/pkg/sumdb/
# sum.golang.org/latest: permission denied", which names verification rather than the toolchain.
ENV GOPATH=/home/sindri/go
ENV GOMODCACHE=/home/sindri/go/pkg/mod
# The base image pins GOTOOLCHAIN=local, and the image only gets its Go at build time — so it falls
# behind every project whose go.mod asks for a newer one, and every go command refuses rather than
# fetching what it needs. `auto` makes a stale image cost one download. `go-upgrade` still pins.
ENV GOTOOLCHAIN=auto
# ~/.local/bin ahead of /usr/local/go/bin is what lets `go-upgrade` take effect without
# root: a newer toolchain linked there shadows the base install for every later command.
ENV PATH=/home/sindri/.local/bin:${PATH}
# UTF-8 locale so Claude's box-drawing/bullet glyphs render instead of "_"
# (Debian's default C locale mangles multibyte output). C.UTF-8 is built in.
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8

RUN git config --global user.name  "sindri-agent" \
  && git config --global user.email "agent@sindri.local" \
  && git config --global init.defaultBranch main

WORKDIR /workspace
