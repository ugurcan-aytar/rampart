# syntax=docker/dockerfile:1.7
# slack-notifier — SSE consumer that posts incident events to Slack.
# In the demo stack it runs with SLACK_WEBHOOK_URL unset, which forces
# the binary into dry-run mode (logs the JSON payload instead of
# POSTing it).

FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags "-s -w" \
      -o /out/slack-notifier ./cmd/slack-notifier

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/slack-notifier /slack-notifier
USER 65532:65532
ENTRYPOINT ["/slack-notifier"]
