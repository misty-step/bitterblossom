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

FROM tailscale/tailscale:stable@sha256:25cde9ad76020b0e29229136d0c38b5962e9a0e1774ffac9b0df68e4a37d6cf0 AS tailscale

FROM debian:bookworm-slim
ARG SPRITE_VERSION=v0.0.1-rc44
ARG LITESTREAM_VERSION=0.5.13
# BuildKit auto-populates TARGETARCH; kaniko (DO App Platform) does not -- default
# to amd64 so both builders work. BuildKit still overrides per-platform.
ARG TARGETARCH=amd64
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git curl openssh-client passwd socat util-linux \
    && useradd --system --create-home --home-dir /home/bb --shell /usr/sbin/nologin bb \
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
COPY --from=tailscale /usr/local/bin/tailscaled /usr/local/bin/tailscaled
COPY --from=tailscale /usr/local/bin/tailscale /usr/local/bin/tailscale
COPY vendor/roster ./vendor/roster
COPY scripts/bb-litestream-entrypoint.sh /usr/local/bin/bb-litestream-entrypoint
COPY scripts/bb-mint-tailnet-entrypoint.sh /usr/local/bin/bb-mint-tailnet-entrypoint
ENV BB_PLANE_DIR=/app/plane
RUN mkdir -p "$BB_PLANE_DIR" \
    && chown bb:bb "$BB_PLANE_DIR" \
    && chmod +x /usr/local/bin/bb-litestream-entrypoint /usr/local/bin/bb-mint-tailnet-entrypoint
ENTRYPOINT ["/usr/local/bin/bb-mint-tailnet-entrypoint"]
CMD ["bb", "serve"]
