FROM debian:bookworm-slim

ARG NANO_RS_VERSION=v2.3.1
ARG TARGETARCH=amd64

RUN apt-get update && apt-get install -y ca-certificates wget && rm -rf /var/lib/apt/lists/*

RUN set -e; \
    case "${TARGETARCH}" in \
      amd64) ARCH=x86_64-unknown-linux-gnu ;; \
      arm64) ARCH=aarch64-unknown-linux-gnu ;; \
      *) echo "Unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac; \
    wget -qO /tmp/nano-rs.tar.gz \
      "https://github.com/gleicon/nano-rs/releases/download/${NANO_RS_VERSION}/nano-rs-${NANO_RS_VERSION}-${ARCH}.tar.gz" && \
    tar -xzf /tmp/nano-rs.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/nano-rs && \
    rm /tmp/nano-rs.tar.gz && \
    nano-rs --version

COPY nano.json /etc/nano-rs/nano.json
COPY placeholder/ /etc/nano-rs/placeholder/

# KV engine uses current_dir()/.nano-kv — set to mounted data volume for persistence.
WORKDIR /var/lib/remo

EXPOSE 8080 8889
CMD ["nano-rs", "run"]
