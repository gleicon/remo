FROM debian:bookworm-slim

ARG REMO_VERSION=v0.5.2
ARG TARGETARCH=amd64

RUN apt-get update && apt-get install -y \
        ca-certificates \
        wget \
        git \
    && rm -rf /var/lib/apt/lists/*

RUN wget -qO /usr/local/bin/remo \
      "https://github.com/gleicon/remo/releases/download/${REMO_VERSION}/remo-linux-${TARGETARCH}" && \
    chmod +x /usr/local/bin/remo && \
    remo --version

# git system user for SSH forced-command deploys (UID 2000 — consistent across containers).
# password field set to '*' (disabled, not locked) so OpenSSH's allowed_user() check
# passes for pubkey auth. '!' (default for --system) is treated as locked and denied.
# Shell must be a valid shell (/bin/sh) so sshd executes the forced command
# from authorized_keys before inspecting the shell. /usr/sbin/nologin causes
# sshd to reject the session before the command: prefix runs.
# Security boundary: forced command in authorized_keys + PasswordAuthentication no.
RUN useradd --uid 2000 --system --shell /bin/sh --no-create-home git \
    && usermod -p '*' git

EXPOSE 7070

CMD ["remo", "server", "start"]
