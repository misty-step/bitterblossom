FROM rust:1-slim-bookworm AS build
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends pkg-config libssl-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY Cargo.toml Cargo.lock ./
COPY src ./src
COPY vendor/roster ./vendor/roster
RUN cargo build --release --locked --bin bb
RUN cargo build --release --locked --manifest-path vendor/roster/Cargo.toml --bin roster

FROM debian:bookworm-slim
ARG SPRITE_VERSION=v0.0.1-rc44
ARG LITESTREAM_VERSION=0.5.13
ARG TARGETARCH
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git curl \
    && rm -rf /var/lib/apt/lists/*
RUN case "${TARGETARCH}" in amd64|arm64) sprite_arch="${TARGETARCH}" ;; *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; esac \
    && curl -fsSL "https://sprites-binaries.t3.storage.dev/client/${SPRITE_VERSION}/sprite-linux-${sprite_arch}.tar.gz" \
    | tar -xzf - -C /usr/local/bin sprite \
    && chmod +x /usr/local/bin/sprite \
    && /usr/local/bin/sprite --version
RUN case "${TARGETARCH}" in amd64) litestream_arch="x86_64" ;; arm64) litestream_arch="arm64" ;; *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; esac \
    && curl -fsSL "https://github.com/benbjohnson/litestream/releases/download/v${LITESTREAM_VERSION}/litestream-${LITESTREAM_VERSION}-linux-${litestream_arch}.tar.gz" \
    | tar -xzf - -C /usr/local/bin litestream \
    && chmod +x /usr/local/bin/litestream \
    && /usr/local/bin/litestream version
WORKDIR /app
COPY --from=build /app/target/release/bb /usr/local/bin/bb
COPY --from=build /app/vendor/roster/target/release/roster /usr/local/bin/roster
COPY vendor/roster ./vendor/roster
COPY scripts/bb-litestream-entrypoint.sh /usr/local/bin/bb-litestream-entrypoint
ENV BB_PLANE_DIR=/app/plane
RUN mkdir -p "$BB_PLANE_DIR" && chmod +x /usr/local/bin/bb-litestream-entrypoint
ENTRYPOINT ["/usr/local/bin/bb-litestream-entrypoint"]
CMD ["bb", "serve"]
