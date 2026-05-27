# golang:latest is Debian-slim-based with Go already correctly installed.
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
    # better find  (binary is fdfind; symlinked to fd below)
    fd-find \
    # file / archive
    file tree zip unzip tar gzip bzip2 xz-utils \
    # network
    wget netcat-openbsd dnsutils iputils-ping \
    # process / system
    procps lsof watch htop \
    # python
    python3 python3-pip python3-venv python3-pytest \
    # misc
    less diffutils vim-tiny \
    && ln -s /usr/bin/fdfind /usr/local/bin/fd \
    && rm -rf /var/lib/apt/lists/*

# ── yq + td (copied from host via build context) ─────────────────────────────
COPY bin/yq  /usr/local/bin/yq
COPY bin/td  /usr/local/bin/td
RUN chmod +x /usr/local/bin/yq /usr/local/bin/td

# ── claude CLI ────────────────────────────────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest
RUN mkdir -p /opt/sindri && cd /opt/sindri && npm init -y && npm install "@anthropic-ai/claude-agent-sdk@^0.1.59"

# ── gh-local (mounted from host at runtime, symlinked here) ──────────────────
# The sindri-gh binary is mounted read-only at /opt/sindri/sindri-gh
# and symlinked to /usr/local/bin/gh at container startup.

# ── Shims ─────────────────────────────────────────────────────────────────────
COPY container/shims/docker          /usr/local/bin/docker
COPY container/shims/docker-compose  /usr/local/bin/docker-compose
RUN chmod +x /usr/local/bin/docker /usr/local/bin/docker-compose

# ── Sindri scripts ────────────────────────────────────────────────────────────
COPY container/wait-for-task.sh /usr/local/bin/wait-for-task
COPY container/sindri-agent.sh  /usr/local/bin/sindri-agent
COPY container/agent-bridge.mjs /opt/sindri/agent-bridge.mjs
COPY container/skills/          /opt/sindri/skills/
COPY container/CLAUDE.md        /opt/sindri/CLAUDE.md
COPY container/CLAUDE.reviewer.md /opt/sindri/CLAUDE.reviewer.md
# Skills are copied into each worker's ~/.claude/skills/ at launch time
RUN chmod +x /usr/local/bin/wait-for-task /usr/local/bin/sindri-agent

# ── Non-root user (claude refuses --dangerously-skip-permissions as root) ──
RUN useradd -m -s /bin/bash sindri

USER sindri
ENV GOMODCACHE=/home/sindri/go/pkg/mod

RUN git config --global user.name  "sindri-agent" \
 && git config --global user.email "agent@sindri.local" \
 && git config --global init.defaultBranch main

WORKDIR /workspace
