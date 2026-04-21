# syntax=docker/dockerfile:1.7
# Engine image — multi-stage, distroless-static, non-root.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags "-s -w" \
      -o /out/engine ./cmd/engine

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/engine /engine
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/engine"]
