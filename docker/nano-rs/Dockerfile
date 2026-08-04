FROM rust:1.88-slim AS builder

RUN apt-get update && apt-get install -y \
        pkg-config \
        libssl-dev \
        ca-certificates \
        git \
        curl \
        xz-utils \
        python3 \
    && rm -rf /var/lib/apt/lists/*

ARG NANO_RS_VERSION=v2.2.4
RUN git clone --depth 1 --branch ${NANO_RS_VERSION} https://github.com/gleicon/nano-rs.git /build/nano-rs
WORKDIR /build/nano-rs
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates wget && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/nano-rs/target/release/nano-rs /usr/local/bin/nano-rs
COPY nano.json /etc/nano-rs/nano.json
COPY placeholder/ /etc/nano-rs/placeholder/

EXPOSE 8080 8889
# Default runs without config (empty registry + admin API on :8889).
# Docker compose overrides with env NANO_ADMIN_API_KEY to enable admin API.
CMD ["nano-rs", "run"]
