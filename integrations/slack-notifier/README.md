# rampart Slack notifier

Subscribes to a running rampart engine's `/v1/stream` Server-Sent
Events feed and posts a Slack message whenever an `incident.opened`
event arrives. Ships as a single static Go binary and as a container
image at `ghcr.io/ugurcan-aytar/rampart-slack-notifier`.

## When to use this

Use the Slack notifier when a rampart engine is running and you want
incident pings in a Slack channel. For other consumer shapes
(PagerDuty, Datadog, Splunk), follow the same adapter pattern — see
[Architecture note](#architecture-note) below.

## Usage

```bash
# Real webhook — reads SLACK_WEBHOOK_URL from env.
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/XXX/YYY/ZZZ
slack-notifier --engine-url http://engine:8080

# Dry run — logs payloads instead of POSTing. Auto-enabled when
# SLACK_WEBHOOK_URL is unset.
slack-notifier --dry-run --engine-url http://localhost:8080
```

In Docker Compose:

```yaml
slack-notifier:
  image: ghcr.io/ugurcan-aytar/rampart-slack-notifier:0.1.0
  environment:
    RAMPART_ENGINE_URL: http://engine:8080
    SLACK_WEBHOOK_URL: ${SLACK_WEBHOOK_URL:-}
  depends_on: [engine]
```

## Flags

| Flag | Env | Default |
|---|---|---|
| `--engine-url` | `RAMPART_ENGINE_URL` | `http://localhost:8080` |
| `--webhook` | `SLACK_WEBHOOK_URL` | *(empty)* |
| `--dry-run` | — | auto-on if webhook empty |

## What gets posted

For each `incident.opened` event, the notifier sends a Slack message
containing the incident ID, the IoC that matched, the affected
component reference, the severity, and a link to the incident page
on the configured Backstage instance (if `BACKSTAGE_BASE_URL` is set).

Heartbeat events (`:heartbeat\n\n` framing) and other event types are
ignored. Reconnect policy: 1-second backoff on transient errors;
`ctx.Done()` shuts the loop down cleanly.

## Architecture note

The `slack-notifier` Go module **does not import the rampart engine's
internal Go packages**. It speaks HTTP and SSE exclusively. That
makes this binary the canonical example of rampart's adapter pattern:

> The engine owns the contract
> ([`schemas/openapi.yaml`](https://github.com/ugurcan-aytar/rampart/blob/main/schemas/openapi.yaml)
> + the `text/event-stream` framing on `/v1/stream`). Consumers
> attach by reading the wire, not by linking to the engine. A new
> notifier (Teams, Opsgenie, PagerDuty, …) follows this same shape:
> a separate Go module that speaks HTTP/SSE only, ships as its own
> binary + container image, and dry-runs by default.

## Build

```bash
cd integrations/slack-notifier
go build -o slack-notifier ./cmd/slack-notifier
go test -race -cover ./...
```

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
