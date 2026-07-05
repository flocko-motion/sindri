# golang:latest is Debian-slim-based with Go already correctly installed..
FROM golang:latest

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
# host binary wouldn't run. td is deliberately absent: agents reach the task
# tracker only via the hub (the in-pod `sindri` command → hub), never a local td
# that could write .todos.
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

# ── claude CLI ────────────────────────────────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest
RUN mkdir -p /opt/sindri && cd /opt/sindri && npm init -y && npm install "@anthropic-ai/claude-agent-sdk@^0.1.59"

# ── openspec CLI (spec-driven workflow, optional per project) ──────────────────
RUN npm install -g @fission-ai/openspec@latest

# ── agent browser (mounted from host at runtime) ─────────────────────────────
# The single, role-agnostic agent binary (built host-side as sindri-worker) is
# mounted read-only at /opt/sindri/sindri-worker at runtime. Inside the isolated
# pod it's the agent's interface to the hub, so we present it simply as `sindri`
# (no host CLI is in the pod to collide with). Role is hub-side, one binary for
# every agent; the sindri user can't write /usr/local/bin, so symlink here.
RUN ln -sf /opt/sindri/sindri-worker /usr/local/bin/sindri

# ── Shims ─────────────────────────────────────────────────────────────────────
COPY shims/docker          /usr/local/bin/docker
COPY shims/docker-compose  /usr/local/bin/docker-compose
RUN chmod +x /usr/local/bin/docker /usr/local/bin/docker-compose

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

USER sindri
ENV GOMODCACHE=/home/sindri/go/pkg/mod
# UTF-8 locale so Claude's box-drawing/bullet glyphs render instead of "_"
# (Debian's default C locale mangles multibyte output). C.UTF-8 is built in.
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8

RUN git config --global user.name  "sindri-agent" \
  && git config --global user.email "agent@sindri.local" \
  && git config --global init.defaultBranch main

WORKDIR /workspace
