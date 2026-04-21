# syntax=docker/dockerfile:1.7
# mock-npm-registry — demo-only HTTP server.
# Same multi-stage + distroless-static shape as engine/Dockerfile so
# the demo stack images look alike.

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download || true
COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags "-s -w" \
      -o /out/mock-npm-registry ./cmd/mock-npm-registry

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mock-npm-registry /mock-npm-registry
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/mock-npm-registry"]
