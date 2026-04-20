# rampart Slack notifier

Subscribes to a running engine's `/v1/stream` SSE feed and posts a Slack
message whenever an `incident.opened` event arrives.

## Usage

```bash
# Real webhook — reads SLACK_WEBHOOK_URL from env.
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/XXX/YYY/ZZZ
slack-notifier --engine-url http://engine:8080

# Dry run — logs payloads instead of sending. Auto-enabled if
# SLACK_WEBHOOK_URL is unset.
slack-notifier --dry-run --engine-url http://localhost:8080
```

Flags:

| Flag            | Env                    | Default                  |
|-----------------|------------------------|--------------------------|
| `--engine-url`  | `RAMPART_ENGINE_URL`   | `http://localhost:8080`  |
| `--webhook`     | `SLACK_WEBHOOK_URL`    | *(empty)*                |
| `--dry-run`     | —                      | auto-on if webhook empty |

## Build

```bash
cd integrations/slack-notifier
go build -o slack-notifier ./cmd/slack-notifier
```

## Architecture note

**slack-notifier's `go.mod` does not import the engine's Go packages.**
It speaks HTTP + SSE exclusively. That makes this binary the first
concrete example of rampart's adapter pattern:

> The engine owns the contract (`schemas/openapi.yaml` + the
> `text/event-stream` framing in `/v1/stream`). Consumers attach by
> reading the wire, not by linking to the engine's internals. A new
> notifier (Teams, Opsgenie, PagerDuty, …) follows this template
> exactly — separate module, HTTP/SSE only.

Reconnect policy: 1 s backoff on transient errors; ctx-cancel stops cleanly.

## Tests

```bash
go test -race -cover ./...
```

Covers the SSE parser (single / multiple frames / heartbeat skipping) and
the webhook sender (payload shape, POST, non-2xx error propagation, dry-run).
