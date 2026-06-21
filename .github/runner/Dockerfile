FROM ghcr.io/actions/actions-runner:latest

USER root

ENV DEBIAN_FRONTEND=noninteractive \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PLAYWRIGHT_PACKAGE_PATH=/opt/ambience-agent/node_modules/playwright/index.mjs

RUN printf '%s\n' \
        'Acquire::Retries "5";' \
        'Acquire::http::Timeout "30";' \
        'Acquire::https::Timeout "30";' \
        'APT::Get::Assume-Yes "true";' \
        'APT::Install-Recommends "false";' \
        'APT::Install-Suggests "false";' \
        >/etc/apt/apt.conf.d/80ambience-retries \
    && printf '%s\n' \
        '#!/bin/sh' \
        'set -eu' \
        'attempt=1' \
        'while [ "$attempt" -le 5 ]; do' \
        '  if apt-get "$@"; then' \
        '    exit 0' \
        '  fi' \
        '  if [ "$attempt" -eq 5 ]; then' \
        '    exit 1' \
        '  fi' \
        '  sleep $((attempt * 10))' \
        '  attempt=$((attempt + 1))' \
        'done' \
        >/usr/local/bin/apt-retry \
    && chmod +x /usr/local/bin/apt-retry \
    && find /etc/apt -type f \( -name '*.list' -o -name '*.sources' \) -print0 \
        | xargs -0 sed -i \
            -e 's|http://archive.ubuntu.com/ubuntu|http://azure.archive.ubuntu.com/ubuntu|g' \
            -e 's|http://security.ubuntu.com/ubuntu|http://azure.archive.ubuntu.com/ubuntu|g'

RUN apt-retry update \
    && apt-retry install \
        ca-certificates \
        curl \
        git \
        gnupg \
        jq \
        python3 \
        python3-pip \
        python3-venv \
        sudo \
        unzip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-retry update \
    && apt-retry install nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN echo "runner ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/runner \
    && chmod 0440 /etc/sudoers.d/runner

RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

RUN KUBECTL_VERSION="$(curl -fsSL https://dl.k8s.io/release/stable.txt)" \
    && curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
    && chmod +x /usr/local/bin/kubectl

RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Go toolchain: the Glimmung run-harness is the Go module under
# glimmung-harness/, built from the run's checkout at re-registration time
# (the harness is git_ref-controlled, exactly like spirelens). This replaces
# the retired scripts/glimmung-native shell fork the image used to bake.
RUN GO_VERSION=1.26.2 \
    && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz
ENV PATH="/usr/local/go/bin:${PATH}" \
    GOPRIVATE="github.com/romaine-life/*"

RUN npm install -g @openai/codex \
    && mkdir -p /opt/ambience-agent \
    && npm install --prefix /opt/ambience-agent playwright \
    && npx --prefix /opt/ambience-agent playwright install --with-deps chromium \
    && chmod -R a+rX /ms-playwright /opt/ambience-agent \
    && npm cache clean --force \
    && rm -rf /root/.npm /opt/ambience-agent/.cache

RUN mkdir -p /workspace \
    && chown runner:runner /workspace

USER runner
