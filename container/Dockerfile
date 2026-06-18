# golang:latest is Debian-slim-based with Go already correctly installed..
FROM golang:latest

# ── Node.js LTS ───────────────────────────────────────────────────────────────
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
    # network
    wget netcat-openbsd dnsutils iputils-ping \
    # process / system
    procps lsof watch htop \
    # terminal multiplexer — the agent runs interactively inside a tmux session
    # that the hub injects into (send-keys) and humans attach to
    tmux \
    # python
    python3 python3-pip python3-venv python3-pytest \
    # misc
    less diffutils vim-tiny \
    && ln -s /usr/bin/fdfind /usr/local/bin/fd \
    && ln -s /usr/bin/batcat /usr/local/bin/bat \
    && rm -rf /var/lib/apt/lists/*

# ── yazi (TUI file manager — not in apt; prebuilt release binary) ──────────────
RUN curl -fsSL https://github.com/sxyazi/yazi/releases/latest/download/yazi-x86_64-unknown-linux-gnu.zip -o /tmp/yazi.zip \
 && unzip -q /tmp/yazi.zip -d /tmp/yazi \
 && mv /tmp/yazi/*/yazi /tmp/yazi/*/ya /usr/local/bin/ \
 && chmod +x /usr/local/bin/yazi /usr/local/bin/ya \
 && rm -rf /tmp/yazi /tmp/yazi.zip

# ── yq + td (copied from host via build context) ─────────────────────────────
COPY bin/yq  /usr/local/bin/yq
COPY bin/td  /usr/local/bin/td
RUN chmod +x /usr/local/bin/yq /usr/local/bin/td

# ── claude CLI ────────────────────────────────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest
RUN mkdir -p /opt/sindri && cd /opt/sindri && npm init -y && npm install "@anthropic-ai/claude-agent-sdk@^0.1.59"

# ── openspec CLI (spec-driven workflow, optional per project) ──────────────────
RUN npm install -g @fission-ai/openspec@latest

# ── agent browser (mounted from host at runtime) ─────────────────────────────
# The single, role-agnostic agent binary is mounted read-only at
# /opt/sindri/sindri-worker at runtime (deliberately NOT named `gh`, so agents
# don't mistake it for the GitHub CLI). Role is hub-side, so there is one binary
# for every agent. The sindri user can't write to /usr/local/bin, so symlink here.
RUN ln -sf /opt/sindri/sindri-worker /usr/local/bin/sindri-worker

# ── Shims ─────────────────────────────────────────────────────────────────────
COPY container/shims/docker          /usr/local/bin/docker
COPY container/shims/docker-compose  /usr/local/bin/docker-compose
RUN chmod +x /usr/local/bin/docker /usr/local/bin/docker-compose

# ── Sindri agent entrypoint ───────────────────────────────────────────────────
# The agent is a thin browser driven by the hub; its instructions come from the
# hub, not baked-in skills/CLAUDE files (those were the pre-hub worker's).
COPY container/sindri-agent.sh  /usr/local/bin/sindri-agent
RUN chmod +x /usr/local/bin/sindri-agent

# ── Non-root user (claude refuses --dangerously-skip-permissions as root) ──
RUN useradd -m -s /bin/bash sindri

# `y` yazi wrapper, sourced by interactive shells (attach / post-Claude bash).
COPY container/yazi.sh /etc/sindri/yazi.sh
RUN echo '. /etc/sindri/yazi.sh' >> /home/sindri/.bashrc

USER sindri
ENV GOMODCACHE=/home/sindri/go/pkg/mod
# UTF-8 locale so Claude's box-drawing/bullet glyphs render instead of "_"
# (Debian's default C locale mangles multibyte output). C.UTF-8 is built in.
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8

RUN git config --global user.name  "sindri-agent" \
 && git config --global user.email "agent@sindri.local" \
 && git config --global init.defaultBranch main

WORKDIR /workspace
