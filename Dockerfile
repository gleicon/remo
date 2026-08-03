FROM debian:bookworm-slim

ARG REMO_VERSION=v0.1.0
# Set REMO_SHA256 to the expected sha256sum of remo-linux-amd64 to verify integrity.
# Leave empty to skip verification (dev/test only).
ARG REMO_SHA256=""

RUN apt-get update && apt-get install -y \
        ca-certificates \
        git \
        openssh-server \
        wget \
    && wget -qO /usr/local/bin/remo \
         "https://github.com/gleicon/remo/releases/download/${REMO_VERSION}/remo-linux-amd64" \
    && if [ -n "${REMO_SHA256}" ]; then \
         echo "${REMO_SHA256}  /usr/local/bin/remo" | sha256sum -c - || exit 1; \
       fi \
    && chmod +x /usr/local/bin/remo \
    && rm -rf /var/lib/apt/lists/*

# git system user for SSH forced-command deploys (UID 2000 — consistent across containers).
RUN useradd --uid 2000 --system --shell /usr/sbin/nologin --no-create-home git

# sshd needs /run/sshd and a host key directory.
RUN mkdir -p /run/sshd /etc/ssh/remo_host_keys

EXPOSE 7070 22
