FROM ghcr.io/actions/actions-runner:latest

USER root

ENV DEBIAN_FRONTEND=noninteractive \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PLAYWRIGHT_PACKAGE_PATH=/opt/ambience-agent/node_modules/playwright/index.mjs

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg \
        jq \
        sudo \
        unzip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN echo "runner ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/runner \
    && chmod 0440 /etc/sudoers.d/runner

RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

RUN npm install -g @openai/codex \
    && mkdir -p /opt/ambience-agent \
    && npm install --prefix /opt/ambience-agent playwright \
    && npx --prefix /opt/ambience-agent playwright install --with-deps chromium \
    && npm cache clean --force \
    && rm -rf /root/.npm /opt/ambience-agent/.cache

USER runner
