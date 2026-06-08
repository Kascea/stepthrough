FROM ubuntu:24.04

ARG GO_VERSION=1.25.4
ARG NODE_MAJOR=22
ARG PYTHON_VERSION=3.12
ARG DOTNET_SDKS="dotnet-sdk-8.0 dotnet-sdk-10.0"

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:/home/vsts/.local/bin:/home/vsts/.npm-global/bin:$PATH"
ENV PYTHON_VERSION=3.12
ENV NPM_CONFIG_PREFIX=/home/vsts/.npm-global

RUN apt-get update -qq && \
    apt-get install -y -qq \
      git curl ca-certificates build-essential pkg-config sudo \
      python3 python${PYTHON_VERSION} python3-pip python-is-python3 \
    && rm -f /usr/lib/python${PYTHON_VERSION}/EXTERNALLY-MANAGED \
    && rm -rf /var/lib/apt/lists/*

# Non-root user with passwordless sudo — mirrors Azure's hosted agent (vsts user).
# /usr/local is group-writable by vsts so pipeline steps can remove/replace
# pre-installed tools (e.g. rm -rf /usr/local/go to swap Go versions).
RUN useradd -m -s /bin/bash vsts && \
    echo "vsts ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/vsts && \
    chmod 0440 /etc/sudoers.d/vsts && \
mkdir -p /home/vsts/.npm-global && \
    chown root:vsts /usr/local && chmod g+w /usr/local && \
    chown -R vsts:vsts /home/vsts/.npm-global

# Go (arch-aware: maps uname -m → Go's naming convention)
# Owned by vsts so pipeline steps can replace it without sudo — mirrors Azure's hosted agent.
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz && \
    tar -xzf /tmp/go.tar.gz -C /usr/local && \
    rm /tmp/go.tar.gz && \
    chown -R vsts:vsts /usr/local/go && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# Node
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - && \
    apt-get install -y -qq nodejs && \
    rm -rf /var/lib/apt/lists/*

# .NET — install via Microsoft apt repo, matching Azure's ubuntu-latest hosted agent.
# apt handles all runtime dependencies (libicu74, etc.) automatically.
RUN curl -sSL https://packages.microsoft.com/config/ubuntu/24.04/packages-microsoft-prod.deb -o /tmp/packages-microsoft-prod.deb && \
    dpkg -i /tmp/packages-microsoft-prod.deb && \
    rm /tmp/packages-microsoft-prod.deb && \
    apt-get update -qq && \
    apt-get install -y -qq ${DOTNET_SDKS} && \
    rm -rf /var/lib/apt/lists/*

USER vsts
WORKDIR /workspace
