package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

// seedIncidentScenario boots an engine with 3 components + axios SBOMs
// + one IoC → 3 pending incidents. Returns the engine handler and the
// list of incident IDs (ordered by component ref).
func seedIncidentScenario(t *testing.T) (http.Handler, *memory.Store, []string) {
	t.Helper()
	store := memory.New()
	bus := events.NewBus(64)
	h := api.NewServer(store, bus, time.Minute).Handler()

	for _, ref := range []string{
		"kind:Component/default/billing",
		"kind:Component/default/reporting",
		"kind:Component/default/web-app",
	} {
		postComponent(t, h, ref, "team-platform")
		postSBOM(t, h, ref, readFixture(t, "axios-compromise.json"))
	}
	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-AXIOS-1-11-0",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.SeverityCritical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
		},
	})

	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incs, 3)
	ids := make([]string, 0, 3)
	for _, inc := range incs {
		ids = append(ids, inc.ID)
	}
	return h, store, ids
}

func TestListIncidents_Returns3_NewestFirst(t *testing.T) {
	h, _, _ := seedIncidentScenario(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var page gen.IncidentPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Items, 3)
	for i := 1; i < len(page.Items); i++ {
		require.False(t, page.Items[i-1].OpenedAt.Before(page.Items[i].OpenedAt),
			"incidents must be newest-first")
	}
}

func TestListIncidents_FilterByState(t *testing.T) {
	h, store, ids := seedIncidentScenario(t)

	// Transition one of the incidents to "triaged"; filter should narrow.
	transition(t, h, ids[0], domain.StateTriaged)

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents?state=triaged", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var page gen.IncidentPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, ids[0], page.Items[0].Id)

	// Sanity: pending filter returns the other two.
	req2 := httptest.NewRequest(http.MethodGet, "/v1/incidents?state=pending", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
	var page2 gen.IncidentPage
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &page2))
	require.Len(t, page2.Items, 2)

	_ = store // silence unused
}

// TestListIncidents_FilterCombinations exercises the multi-dimension
// filter expansion from the E3 backend split: multi-state, ecosystem
// array, time range, search substring, owner exact, limit cap, plus
// the legacy `since` alias for `from`.
func TestListIncidents_FilterCombinations(t *testing.T) {
	h, _, ids := seedIncidentScenario(t)

	// Transition one incident so we can exercise multi-state.
	transition(t, h, ids[0], domain.StateTriaged)

	t.Run("multi-state OR returns both", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet,
			"/v1/incidents?state=pending&state=triaged", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Items, 3)
	})

	t.Run("ecosystem array narrows to npm", func(t *testing.T) {
		// Fixture seeds an npm IoC; gomod array should produce 0.
		req := httptest.NewRequest(http.MethodGet, "/v1/incidents?ecosystem=npm", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Items, 3, "all 3 fixture incidents are npm")

		req2 := httptest.NewRequest(http.MethodGet, "/v1/incidents?ecosystem=gomod", nil)
		rr2 := httptest.NewRecorder()
		h.ServeHTTP(rr2, req2)
		var page2 gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &page2))
		require.Empty(t, page2.Items)
	})

	t.Run("search substring matches incident id", func(t *testing.T) {
		// Use the last 6 chars of the ULID — those are the random
		// suffix; the time-prefix is shared across same-millisecond
		// fixture incidents so an early-prefix search would match all.
		needle := ids[0][len(ids[0])-6:]
		req := httptest.NewRequest(http.MethodGet,
			"/v1/incidents?search="+needle, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Items, 1)
		require.Equal(t, ids[0], page.Items[0].Id)
	})

	t.Run("search substring matches ioc id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/incidents?search=axios", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Items, 3)
	})

	t.Run("owner narrows to component-keyed match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet,
			"/v1/incidents?owner=team-platform", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.NotEmpty(t, page.Items, "fixture seeds team-platform components")
	})

	t.Run("limit caps result count", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/incidents?limit=1", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Items, 1)
	})

	t.Run("since is honoured as a from alias", func(t *testing.T) {
		// All fixtures opened recently; a `since` in the future returns 0.
		future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet,
			"/v1/incidents?since="+future, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var page gen.IncidentPage
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Empty(t, page.Items)
	})
}

func TestGetIncident_Hydrated(t *testing.T) {
	h, _, ids := seedIncidentScenario(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents/"+ids[0], nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var inc gen.Incident
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &inc))
	require.Equal(t, ids[0], inc.Id)
	require.Equal(t, "01IOC-AXIOS-1-11-0", inc.IocId)
	require.Equal(t, gen.Pending, inc.State)
	require.NotNil(t, inc.AffectedComponentsSnapshot)
	require.Len(t, *inc.AffectedComponentsSnapshot, 1)
}

func TestGetIncident_MissingReturns404(t *testing.T) {
	h, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/incidents/NOPE", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestGetIncidentDetail_JoinedShape exercises the drawer-backing
// endpoint: incident + IoC + every affected component hydrated in a
// single round-trip.
func TestGetIncidentDetail_JoinedShape(t *testing.T) {
	h, _, ids := seedIncidentScenario(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents/"+ids[0]+"/detail", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var detail gen.IncidentDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &detail))

	require.Equal(t, ids[0], detail.Incident.Id)
	require.Equal(t, gen.Pending, detail.Incident.State)

	require.NotNil(t, detail.Ioc, "IoC must be hydrated")
	require.Equal(t, "01IOC-AXIOS-1-11-0", detail.Ioc.Id)
	require.Equal(t, gen.IoCKindPackageVersion, detail.Ioc.Kind)

	require.NotNil(t, detail.AffectedComponents)
	require.Len(t, *detail.AffectedComponents, 1, "one affected component per fixture incident")
	first := (*detail.AffectedComponents)[0]
	require.Contains(t, first.Ref, "kind:Component/default/")
	require.NotNil(t, first.Owner)
	require.Equal(t, "team-platform", *first.Owner)
}

// TestGetIncidentDetail_MissingIncidentReturns404 ensures the detail
// endpoint participates in the same 404 contract as GetIncident.
func TestGetIncidentDetail_MissingIncidentReturns404(t *testing.T) {
	h, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/incidents/NOPE/detail", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestGetIncidentDetail_HappyPathRegression is a regression check on
// the joined endpoint with a fully-resolvable seed; the
// "dropped component ref" survival contract is documented in the
// handler comment but not reachable from the test fixture today
// (memory.Store doesn't expose a delete surface in v0.2.0).
func TestGetIncidentDetail_HappyPathRegression(t *testing.T) {
	h, _, ids := seedIncidentScenario(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents/"+ids[0]+"/detail", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestTransitionIncident_ValidChain walks one incident through the
// happy-path state machine: pending → triaged → acknowledged →
// remediating → closed. Each step asserts the response, storage, and
// that the bus saw an incident.transitioned event.
func TestTransitionIncident_ValidChain(t *testing.T) {
	store := memory.New()
	bus := events.NewBus(64)
	h := api.NewServer(store, bus, time.Minute).Handler()

	postComponent(t, h, "kind:Component/default/web", "team")
	postSBOM(t, h, "kind:Component/default/web", readFixture(t, "axios-compromise.json"))
	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-X",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.SeverityCritical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
		},
	})

	incs, _ := store.ListIncidents(context.Background())
	require.Len(t, incs, 1)
	id := incs[0].ID

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev, unsub := bus.Subscribe(ctx)
	defer unsub()

	chain := []domain.IncidentState{
		domain.StateTriaged,
		domain.StateAcknowledged,
		domain.StateRemediating,
		domain.StateClosed,
	}
	prev := domain.StatePending
	for _, next := range chain {
		transition(t, h, id, next)
		select {
		case e := <-ev:
			te, ok := e.(domain.IncidentTransitionedEvent)
			require.True(t, ok, "expected IncidentTransitionedEvent, got %T", e)
			require.Equal(t, id, te.IncidentID)
			require.Equal(t, prev, te.From)
			require.Equal(t, next, te.To)
		case <-ctx.Done():
			t.Fatalf("timeout waiting for incident.transitioned %s→%s", prev, next)
		}
		prev = next
	}
}

func TestTransitionIncident_InvalidReturns409(t *testing.T) {
	h, _, ids := seedIncidentScenario(t)
	// pending → closed is illegal (closed isn't a direct successor).
	body, _ := json.Marshal(gen.IncidentTransitionRequest{To: gen.Closed})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/incidents/"+ids[0]+"/transition", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict, rr.Code)

	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "INVALID_TRANSITION", e.Code)
}

func TestTransitionIncident_MissingReturns404(t *testing.T) {
	h, _ := newTestServer(t)
	body, _ := json.Marshal(gen.IncidentTransitionRequest{To: gen.Triaged})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/incidents/NOPE/transition", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestBlastRadius_ReturnsAffectedComponents(t *testing.T) {
	h, _, _ := seedIncidentScenario(t)

	body, _ := json.Marshal(gen.BlastRadiusRequest{
		Iocs: []gen.IoC{
			{
				Id:          "01IOC-HYPO",
				Kind:        gen.IoCKindPackageVersion,
				Severity:    gen.SeverityHigh,
				Ecosystem:   "npm",
				PublishedAt: time.Now().UTC(),
				PackageVersion: &gen.IoCPackageVersion{
					Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/blast-radius", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp gen.BlastRadiusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.ElementsMatch(t, resp.AffectedComponentRefs, []string{
		"kind:Component/default/billing",
		"kind:Component/default/reporting",
		"kind:Component/default/web-app",
	})
}

func TestBlastRadius_NoIoCs_400(t *testing.T) {
	h, _ := newTestServer(t)
	body, _ := json.Marshal(gen.BlastRadiusRequest{Iocs: []gen.IoC{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/blast-radius", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestBlastRadius_CachedPathForIngestedIoC pins the v0.2.1 fast path:
// when the request carries an IoC ID the engine has already ingested,
// BlastRadius reads matched component refs straight out of the
// incidents table (storage.MatchedComponentRefsByIoC) instead of
// re-running the matcher across every SBOM.
//
// seedIncidentScenario submits IoC `01IOC-AXIOS-1-11-0` (axios@1.11.0)
// against three components, opening three incidents. We then ask
// BlastRadius for the same IoC ID but with a deliberately impossible
// version (`1.99.99`) in the request body — no SBOM in the fixture
// carries that version, so a live matcher pass would return [].
// Returning the original three components proves the engine routed
// the request through the cached lookup keyed on ID, not the live
// matcher keyed on the body's version predicate.
func TestBlastRadius_CachedPathForIngestedIoC(t *testing.T) {
	h, _, _ := seedIncidentScenario(t)

	body, _ := json.Marshal(gen.BlastRadiusRequest{
		Iocs: []gen.IoC{{
			Id:          "01IOC-AXIOS-1-11-0", // already ingested at 1.11.0
			Kind:        gen.IoCKindPackageVersion,
			Severity:    gen.SeverityCritical,
			Ecosystem:   "npm",
			PublishedAt: time.Now().UTC(),
			PackageVersion: &gen.IoCPackageVersion{
				// Deliberate mismatch: no SBOM has 1.99.99. A live
				// matcher would return []; cache returns the snapshot.
				Name: "axios", Version: "1.99.99", Purl: "pkg:npm/axios@1.99.99",
			},
		}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/blast-radius", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp gen.BlastRadiusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.ElementsMatch(t, resp.AffectedComponentRefs, []string{
		"kind:Component/default/billing",
		"kind:Component/default/reporting",
		"kind:Component/default/web-app",
	}, "cache path must return the snapshot persisted at IoC submit time")
}

// transition is a helper for driving /v1/incidents/{id}/transition in tests.
func transition(t *testing.T, h http.Handler, id string, to domain.IncidentState) {
	t.Helper()
	body, _ := json.Marshal(gen.IncidentTransitionRequest{
		To: gen.IncidentState(to),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/incidents/"+id+"/transition", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "transition %s→%s: %s", id, to, rr.Body.String())
}
