FROM ghcr.io/actions/actions-runner:latest

USER root

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        jq \
        sudo \
        unzip \
    && rm -rf /var/lib/apt/lists/*

RUN echo "runner ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/runner \
    && chmod 0440 /etc/sudoers.d/runner

RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

USER runner
