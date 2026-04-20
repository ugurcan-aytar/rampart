package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// sseWriter frames an HTTP response body as text/event-stream. The headers
// are committed inside the constructor so callers cannot accidentally
// write JSON before the first SSE frame.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Nginx and its derivatives buffer text/event-stream by default; this
	// header opts us out so clients see frames immediately.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush() // flush headers so connect completes before first event
	return &sseWriter{w: w, flusher: f}, nil
}

// WriteEvent frames a single SSE event. eventID is optional (empty ⇒ omit).
// Returns the first write error; the caller is expected to close out.
func (s *sseWriter) WriteEvent(eventType, eventID string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("sse marshal: %w", err)
	}
	if eventID != "" {
		if _, err := fmt.Fprintf(s.w, "id: %s\n", eventID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\n", eventType); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", payload); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteComment writes a `: <text>\n\n` frame. Clients and proxies treat
// this as a no-op heartbeat — nothing is delivered to the EventSource
// `onmessage` handler, but the connection stays alive.
func (s *sseWriter) WriteComment(text string) error {
	if _, err := fmt.Fprintf(s.w, ": %s\n\n", text); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// streamLoop is the body of Server.Stream. It's factored out so unit tests
// can drive it with a bufio.Pipe-backed ResponseWriter and a hand-fed
// channel, independent of the full HTTP stack.
func streamLoop(
	ctx context.Context,
	log *slog.Logger,
	sse *sseWriter,
	ch <-chan domain.DomainEvent,
	heartbeat time.Duration,
) {
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := sse.WriteComment("keep-alive"); err != nil {
				log.Debug("sse: heartbeat write failed", "err", err)
				return
			}
		case e, ok := <-ch:
			if !ok {
				// Bus dropped us (slow consumer) or upstream cancel fired.
				return
			}
			envelope := toStreamEnvelope(e)
			if err := sse.WriteEvent(e.EventType(), e.AggregateID(), envelope); err != nil {
				log.Debug("sse: event write failed", "err", err)
				return
			}
		}
	}
}

// toStreamEnvelope shapes a domain.DomainEvent into the JSON form promised
// by schemas/openapi.yaml #/components/schemas/StreamEvent. We emit a
// map[string]any rather than the oapi-codegen-generated union type
// because the generated Go union resists direct marshalling; the map's
// keys come straight from the schema, so drift is immediately visible.
func toStreamEnvelope(e domain.DomainEvent) map[string]any {
	env := map[string]any{
		"type":       e.EventType(),
		"occurredAt": e.OccurredAt(),
	}
	switch v := e.(type) {
	case domain.IncidentOpenedEvent:
		env["incidentId"] = v.IncidentID
		env["iocId"] = v.IoCID
		if v.AffectedComponentsSnapshot != nil {
			env["affectedComponentsSnapshot"] = v.AffectedComponentsSnapshot
		}
	case domain.IncidentTransitionedEvent:
		env["incidentId"] = v.IncidentID
		env["from"] = v.From
		env["to"] = v.To
	case domain.RemediationAddedEvent:
		env["incidentId"] = v.IncidentID
		env["remediationId"] = v.RemediationID
		env["kind"] = v.Kind
	case domain.SBOMIngestedEvent:
		env["sbomId"] = v.SBOMID
		env["componentRef"] = v.ComponentRef
	case domain.IoCMatchedEvent:
		env["iocId"] = v.IoCID
		env["matchedComponentRefs"] = v.MatchedComponents
	}
	return env
}
