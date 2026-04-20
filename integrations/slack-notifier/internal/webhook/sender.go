// Package webhook wraps the Slack incoming-webhook POST. Two implementations:
//
//   - HTTP  — posts to a real Slack webhook
//   - DryRun — logs the payload that would be sent (for --dry-run mode or
//     unconfigured webhooks)
//
// Both accept a receiver.Frame and derive the Slack payload from it.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/receiver"
)

// Sender is the contract both backends satisfy.
type Sender interface {
	Send(ctx context.Context, frame receiver.Frame) error
}

// --- HTTP (real webhook) --------------------------------------------------

type HTTP struct {
	url    string
	log    *slog.Logger
	client *http.Client
}

func NewHTTP(url string, log *slog.Logger) *HTTP {
	return &HTTP{url: url, log: log, client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *HTTP) Send(ctx context.Context, frame receiver.Frame) error {
	payload := BuildSlackPayload(frame)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook: %d: %s", resp.StatusCode, bytes.TrimSpace(b))
	}
	s.log.Info("slack message posted", "incident_id", frame.ID)
	return nil
}

// --- DryRun (log only) ----------------------------------------------------

type DryRun struct{ log *slog.Logger }

func NewDryRun(log *slog.Logger) *DryRun { return &DryRun{log: log} }

func (d *DryRun) Send(_ context.Context, frame receiver.Frame) error {
	payload := BuildSlackPayload(frame)
	body, _ := json.MarshalIndent(payload, "", "  ")
	d.log.Info("dry-run: would POST to Slack",
		"incident_id", frame.ID,
		"event_type", frame.EventType,
		"payload", string(body))
	return nil
}

// --- Payload --------------------------------------------------------------

// BuildSlackPayload formats a Slack blocks-kit message from an SSE frame.
// Exported so tests can exercise it in isolation.
func BuildSlackPayload(frame receiver.Frame) map[string]any {
	var ev map[string]any
	_ = json.Unmarshal([]byte(frame.Data), &ev)

	incidentID, _ := ev["incidentId"].(string)
	if incidentID == "" {
		incidentID = frame.ID
	}
	iocID, _ := ev["iocId"].(string)
	if iocID == "" {
		iocID = "(unknown)"
	}

	return map[string]any{
		"text": fmt.Sprintf(":warning: rampart incident opened: *%s*", incidentID),
		"blocks": []any{
			map[string]any{
				"type": "header",
				"text": map[string]any{"type": "plain_text", "text": "Supply-chain incident opened"},
			},
			map[string]any{
				"type": "section",
				"fields": []any{
					map[string]any{"type": "mrkdwn", "text": fmt.Sprintf("*Incident*\n`%s`", incidentID)},
					map[string]any{"type": "mrkdwn", "text": fmt.Sprintf("*IoC*\n`%s`", iocID)},
				},
			},
		},
	}
}
