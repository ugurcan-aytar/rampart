package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

// TestNoHandlerReturns501 is the belt-and-braces regression guard:
// every registered route must answer with something other than 501.
// Individual handler tests already assert specific behaviours; this
// one only catches the regression where a new handler is added to the
// OpenAPI schema and someone forgets to wire it up.
//
// The set of routes + methods here tracks the OpenAPI paths directly;
// if the schema grows, add a line here.
func TestNoHandlerReturns501(t *testing.T) {
	srv := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	h := srv.Handler()

	cases := []struct {
		name, method, path string
	}{
		{"Healthz", http.MethodGet, "/healthz"},
		{"Readyz", http.MethodGet, "/readyz"},
		{"ListComponents", http.MethodGet, "/v1/components"},
		{"UpsertComponent", http.MethodPost, "/v1/components"},
		{"ListSBOMsByComponent", http.MethodGet, "/v1/components/kind:Component%2Fdefault%2Fweb/sboms"},
		{"SubmitSBOM", http.MethodPost, "/v1/components/kind:Component%2Fdefault%2Fweb/sboms"},
		{"GetSBOM", http.MethodGet, "/v1/sboms/SBOM1"},
		{"ListIoCs", http.MethodGet, "/v1/iocs"},
		{"SubmitIoC", http.MethodPost, "/v1/iocs"},
		{"ListIncidents", http.MethodGet, "/v1/incidents"},
		{"GetIncident", http.MethodGet, "/v1/incidents/INC1"},
		{"TransitionIncident", http.MethodPost, "/v1/incidents/INC1/transition"},
		{"AddRemediation", http.MethodPost, "/v1/incidents/INC1/remediations"},
		{"BlastRadius", http.MethodPost, "/v1/blast-radius"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			require.NotEqual(t, http.StatusNotImplemented, rr.Code,
				"%s regressed to 501; body=%s", tc.name, rr.Body.String())
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
