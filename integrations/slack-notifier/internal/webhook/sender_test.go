package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/receiver"
	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/webhook"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildSlackPayload_IncidentOpened(t *testing.T) {
	frame := receiver.Frame{
		ID:        "INC-axios-2026-03-31",
		EventType: "incident.opened",
		Data:      `{"type":"incident.opened","incidentId":"INC-axios-2026-03-31","iocId":"IOC-axios-1-11-0"}`,
	}
	p := webhook.BuildSlackPayload(frame)
	require.Contains(t, p["text"], "INC-axios-2026-03-31")
	blocks := p["blocks"].([]any)
	require.Equal(t, 2, len(blocks))
	header := blocks[0].(map[string]any)["text"].(map[string]any)
	require.Equal(t, "Supply-chain incident opened", header["text"])
}

func TestDryRun_LogsPayload(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	dr := webhook.NewDryRun(log)
	err := dr.Send(context.Background(), receiver.Frame{
		ID:        "INC1",
		EventType: "incident.opened",
		Data:      `{"incidentId":"INC1","iocId":"IOC1"}`,
	})
	require.NoError(t, err)
	s := buf.String()
	require.Contains(t, s, "dry-run")
	require.Contains(t, s, "INC1")
}

func TestHTTP_PostsPayload(t *testing.T) {
	var got bytes.Buffer
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		got.Write(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sender := webhook.NewHTTP(ts.URL, silentLogger())
	err := sender.Send(context.Background(), receiver.Frame{
		ID:        "INC1",
		EventType: "incident.opened",
		Data:      `{"incidentId":"INC1","iocId":"IOC1"}`,
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(got.Bytes(), &payload))
	require.Contains(t, payload["text"], "INC1")
}

func TestHTTP_PropagatesNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate-limited"))
	}))
	defer ts.Close()

	sender := webhook.NewHTTP(ts.URL, silentLogger())
	err := sender.Send(context.Background(), receiver.Frame{EventType: "incident.opened", Data: "{}"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "429"))
}
