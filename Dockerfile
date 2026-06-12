FROM rust:1-slim-bookworm AS build
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends pkg-config libssl-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release --locked --bin bb

FROM debian:bookworm-slim
ARG SPRITE_VERSION=v0.0.1-rc44
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git curl \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://sprites-binaries.t3.storage.dev/client/${SPRITE_VERSION}/sprite-linux-amd64.tar.gz" \
    | tar -xzf - -C /usr/local/bin sprite \
    && chmod +x /usr/local/bin/sprite \
    && /usr/local/bin/sprite --version
WORKDIR /app
COPY --from=build /app/target/release/bb /usr/local/bin/bb
COPY plane ./plane
CMD ["bb", "--config", "plane", "serve"]
