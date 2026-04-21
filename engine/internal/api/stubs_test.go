package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

// TestStubEndpoints_Return501WithJSONError is the contract regression
// test for every endpoint that is currently a 501 stub. When the stub
// is replaced with real logic in a later step, this test moves to the
// new behaviour — the table entry updates, the test form stays.
//
// What we assert:
//   - HTTP 501 Not Implemented
//   - Content-Type is application/json (no HTML leakage)
//   - Body deserialises into gen.Error, with Code = NOT_IMPLEMENTED and
//     Message naming the operation (so log scrapers can filter).
func TestStubEndpoints_Return501WithJSONError(t *testing.T) {
	srv := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	h := srv.Handler()

	cases := []struct {
		name, method, path, body string
	}{
		{
			"AddRemediation", http.MethodPost, "/v1/incidents/INC1/remediations",
			`{"id":"R1","incidentId":"INC1","kind":"notify","executedAt":"2026-04-20T00:00:00Z"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *bytes.Reader
			if tc.body != "" {
				body = bytes.NewReader([]byte(tc.body))
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotImplemented, rr.Code,
				"%s: want 501, got %d; body=%s", tc.name, rr.Code, rr.Body.String())

			ct := strings.SplitN(rr.Header().Get("Content-Type"), ";", 2)[0]
			require.Equal(t, "application/json", ct, "%s: Content-Type must be JSON", tc.name)

			var errBody gen.Error
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errBody),
				"%s: body must unmarshal to gen.Error; got %q", tc.name, rr.Body.String())
			require.Equal(t, "NOT_IMPLEMENTED", errBody.Code,
				"%s: Code must be NOT_IMPLEMENTED", tc.name)
			require.Contains(t, errBody.Message, tc.name,
				"%s: Message should name the operation; got %q", tc.name, errBody.Message)
		})
	}
}

// TestLiveEndpoints_AreNotStubs makes sure the live handlers keep their
// non-501 behaviour so the stub table above doesn't accidentally grow
// when one of them regresses.
func TestLiveEndpoints_AreNotStubs(t *testing.T) {
	srv := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	h := srv.Handler()

	live := []struct{ method, path string }{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/v1/components"},
	}
	for _, r := range live {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			require.NotEqual(t, http.StatusNotImplemented, rr.Code,
				"live endpoint %s regressed to 501", r.path)
		})
	}
}

// TestServeMux_WrongMethodReturns405 documents the stdlib ServeMux
// behaviour that rampart relies on: a registered path hit with an
// unexpected method yields 405 Method Not Allowed, not 404. This matters
// for consumer tooling — e.g., a SARIF uploader treats 405 as "you hit
// the wrong verb" (retryable) vs 404 "rampart is down" (actionable).
func TestServeMux_WrongMethodReturns405(t *testing.T) {
	srv := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	h := srv.Handler()

	cases := []struct{ name, method, path string }{
		{"DELETE /v1/components", http.MethodDelete, "/v1/components"},
		{"PATCH /v1/iocs", http.MethodPatch, "/v1/iocs"},
		{"PUT /v1/incidents", http.MethodPut, "/v1/incidents"},
		{"DELETE /v1/incidents/{id}/transition", http.MethodDelete, "/v1/incidents/INC1/transition"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			require.Equal(t, http.StatusMethodNotAllowed, rr.Code,
				"%s expected 405; got %d", tc.name, rr.Code)
		})
	}
}
