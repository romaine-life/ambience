FROM ghcr.io/actions/actions-runner:latest

USER root

ENV DEBIAN_FRONTEND=noninteractive \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PLAYWRIGHT_PACKAGE_PATH=/opt/ambience-agent/node_modules/playwright/index.mjs

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
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
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN echo "runner ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/runner \
    && chmod 0440 /etc/sudoers.d/runner

RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

RUN KUBECTL_VERSION="$(curl -fsSL https://dl.k8s.io/release/stable.txt)" \
    && curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
    && chmod +x /usr/local/bin/kubectl

RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

RUN npm install -g @openai/codex \
    && mkdir -p /opt/ambience-agent \
    && npm install --prefix /opt/ambience-agent playwright \
    && npx --prefix /opt/ambience-agent playwright install --with-deps chromium \
    && chmod -R a+rX /ms-playwright /opt/ambience-agent \
    && npm cache clean --force \
    && rm -rf /root/.npm /opt/ambience-agent/.cache

COPY scripts/glimmung-native /opt/ambience-native/scripts
RUN chmod +x /opt/ambience-native/scripts/*.sh

USER runner
