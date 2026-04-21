# syntax=docker/dockerfile:1.7
# rampart-native — multi-stage, distroless, non-root.
#
# Why distroless + static: the sidecar's sole runtime behaviour is
# "read UDS, call into serde_json, write UDS". It ships zero network
# sockets and no HTTP endpoints, so glibc + musl symbols + OpenSSL are
# all unnecessary. A static binary on a distroless base keeps the image
# under ~25 MiB and shrinks the attack surface to effectively "just the
# parser."

FROM rust:1.95-slim AS build

WORKDIR /src

# Enable the musl target for a static binary when we're on linux/amd64
# or linux/arm64 — matches the default ghcr.io architectures for this
# repo's release workflow (Adım 8).
RUN apt-get update && apt-get install -y --no-install-recommends \
        musl-tools \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

ARG TARGET=aarch64-unknown-linux-musl
RUN rustup target add "$TARGET"

# Cache `cargo fetch` between builds by copying only the manifests first.
COPY Cargo.toml Cargo.lock* ./
COPY crates/rampart-native/Cargo.toml crates/rampart-native/Cargo.toml
COPY crates/rampart-native-cli/Cargo.toml crates/rampart-native-cli/Cargo.toml
RUN mkdir -p crates/rampart-native/src crates/rampart-native-cli/src \
    && echo 'fn main() {}' > crates/rampart-native-cli/src/main.rs \
    && echo '' > crates/rampart-native/src/lib.rs \
    && cargo fetch --target "$TARGET"

# Bring in real sources and build.
COPY crates/ crates/
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/src/target \
    cargo build --release --target "$TARGET" --bin rampart-native \
    && cp "/src/target/$TARGET/release/rampart-native" /rampart-native

# Intermediate stage to prepare the UDS directory with nonroot ownership.
# When docker-compose mounts a named volume at `/var/run/rampart`, Docker
# seeds the fresh volume from whatever exists at that path in the image
# (content + ownership), so pre-creating the dir here survives the mount.
FROM busybox:1.36 AS scratchdir
RUN mkdir -p /scratch/var/run/rampart \
    && chown -R 65532:65532 /scratch/var/run

# Runtime stage — distroless-static, non-root uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=scratchdir --chown=65532:65532 /scratch/var/run /var/run
COPY --from=build /rampart-native /rampart-native

# Sidecar binds the UDS socket under /var/run/rampart/. In Docker
# Compose we mount that directory as a named volume shared with the
# engine container.
ENV RAMPART_NATIVE_SOCKET=/var/run/rampart/native.sock \
    RUST_LOG=info

USER 65532:65532
ENTRYPOINT ["/rampart-native"]
