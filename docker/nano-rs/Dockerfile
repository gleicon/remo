FROM rust:1.88-slim AS builder

RUN apt-get update && apt-get install -y \
        pkg-config libssl-dev ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

ARG NANO_RS_VERSION=v2.3.1
RUN git clone --depth 1 --branch ${NANO_RS_VERSION} https://github.com/gleicon/nano-rs.git /build/nano-rs
WORKDIR /build/nano-rs
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/nano-rs/target/release/nano-rs /usr/local/bin/nano-rs
COPY nano.json /etc/nano-rs/nano.json
COPY placeholder/ /etc/nano-rs/placeholder/

# KV engine uses current_dir()/.nano-kv — set to mounted data volume for persistence.
WORKDIR /var/lib/remo

EXPOSE 8080 8889
CMD ["nano-rs", "run"]
