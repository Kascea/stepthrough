FROM ubuntu:24.04

ARG GO_VERSION=1.25.4
ARG NODE_MAJOR=22
ARG PYTHON_VERSION=3.12
ARG DOTNET_CHANNEL=10.0

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:$PATH"

# Base tools + GTK4/WebKitGTK 6.0 dev headers (matches Azure ubuntu-latest capability).
# Required for go vet/build to compile Wails v3 CGo packages without stubs.
# Ubuntu 24.04 ships Python 3.12, GTK4, and WebKitGTK 6.0 natively — no PPAs needed.
RUN apt-get update -qq && \
    apt-get install -y -qq \
      git curl ca-certificates build-essential pkg-config \
      libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev \
      python${PYTHON_VERSION} python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Go (arch-aware: maps uname -m → Go's naming convention)
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz && \
    tar -xzf /tmp/go.tar.gz -C /usr/local && \
    rm /tmp/go.tar.gz && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# Node
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - && \
    apt-get install -y -qq nodejs && \
    rm -rf /var/lib/apt/lists/*

# .NET
RUN curl -sSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh --channel ${DOTNET_CHANNEL} --install-dir /usr/local/bin && \
    rm /tmp/dotnet-install.sh

WORKDIR /workspace
