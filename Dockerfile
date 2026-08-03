FROM rust:1.87-slim AS builder
RUN apt-get update && apt-get install -y pkg-config && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY Cargo.toml Cargo.lock ./
COPY src/ ./src/
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y \
        ca-certificates \
        git \
        openssh-server \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/target/release/remo /usr/local/bin/remo

# git system user for SSH forced-command deploys (UID 2000 — consistent across containers).
RUN useradd --uid 2000 --system --shell /usr/sbin/nologin --no-create-home git

# sshd needs /run/sshd and a host key directory.
RUN mkdir -p /run/sshd /etc/ssh/remo_host_keys

EXPOSE 7070 22
