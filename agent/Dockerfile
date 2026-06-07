FROM ubuntu:24.04

ARG GO_VERSION=1.25.4
ARG NODE_MAJOR=22
ARG PYTHON_VERSION=3.12
ARG DOTNET_SDKS="dotnet-sdk-8.0 dotnet-sdk-10.0"

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:$PATH"

RUN apt-get update -qq && \
    apt-get install -y -qq \
      git curl ca-certificates build-essential pkg-config sudo \
      python${PYTHON_VERSION} python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Non-root user with passwordless sudo — mirrors Azure's hosted agent (vsts user).
RUN useradd -m -s /bin/bash vsts && \
    echo "vsts ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/vsts && \
    chmod 0440 /etc/sudoers.d/vsts

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

# .NET — install via Microsoft apt repo, matching Azure's ubuntu-latest hosted agent.
# apt handles all runtime dependencies (libicu74, etc.) automatically.
RUN curl -sSL https://packages.microsoft.com/config/ubuntu/24.04/packages-microsoft-prod.deb -o /tmp/packages-microsoft-prod.deb && \
    dpkg -i /tmp/packages-microsoft-prod.deb && \
    rm /tmp/packages-microsoft-prod.deb && \
    apt-get update -qq && \
    apt-get install -y -qq ${DOTNET_SDKS} && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
USER vsts
