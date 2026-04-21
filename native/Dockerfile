# syntax=docker/dockerfile:1.7
# rampart-native — distroless, non-root.
#
# The binary is pre-built by the `native:` matrix job in
# `.github/workflows/release.yml` (one cargo cross-compile per target)
# and staged into `native/artifacts/rampart-native-{amd64,arm64}` by
# the image-native job before it calls buildx. We COPY the matching
# pre-signed binary here rather than re-compiling under QEMU, which
# keeps the multi-arch image build to a COPY + push (~2 min) and
# guarantees the image ships exactly the bytes that shipped as the
# release tarball + cosign signature.
#
# TARGETARCH is Docker's automatic per-platform build-arg: `amd64`
# or `arm64` on the platforms we publish.

FROM busybox:1.36 AS scratchdir
# Pre-stage the UDS directory with nonroot ownership. A named volume
# mounted over /var/run/rampart is seeded from the image's contents
# on first use, so pre-creating the dir here survives the mount.
RUN mkdir -p /scratch/var/run/rampart \
    && chown -R 65532:65532 /scratch/var/run

FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH

COPY --from=scratchdir --chown=65532:65532 /scratch/var/run /var/run
COPY --chmod=0755 artifacts/rampart-native-${TARGETARCH} /rampart-native

ENV RAMPART_NATIVE_SOCKET=/var/run/rampart/native.sock \
    RUST_LOG=info

USER 65532:65532
ENTRYPOINT ["/rampart-native"]
