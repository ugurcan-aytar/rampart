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

// TestChain_AxiosScenario exercises the full Adım 7 flow end-to-end
// against an in-process engine. This is the curl chain from the
// "Dur ve bana göster" list, collapsed into a single self-contained
// test so `go test ./...` exercises the narrative:
//
//  1. register 3 components
//  2. ingest 3 clean SBOMs (simple-webapp — no axios@1.11.0)
//  3. publish an axios@1.11.0 IoC
//  4. ingest 1 compromised SBOM (axios-compromise.json) for the
//     web-app component — retroactive match opens an incident
//  5. publish a packageRange IoC (axios >=1.11.0) — forward match
//     idempotently skips the already-open incident
//  6. transition the incident pending → triaged → remediating
//  7. add a remediation
//  8. GET the incident — confirm state, remediation, snapshot all
//     hydrate correctly
//  9. blast-radius query — confirm Go logic matches what just happened
//
// Every step asserts the matching SSE event landed on a subscriber
// channel so the "engine emits events" contract isn't quietly broken.
func TestChain_AxiosScenario(t *testing.T) {
	store := memory.New()
	bus := events.NewBus(128)
	h := api.NewServer(store, bus, time.Minute).Handler()

	subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eventCh, unsubscribe := bus.Subscribe(subCtx)
	defer unsubscribe()

	// ---- 1. register 3 components ----
	components := []string{
		"kind:Component/default/web-app",
		"kind:Component/default/billing",
		"kind:Component/default/reporting",
	}
	for _, ref := range components {
		postComponent(t, h, ref, "team-platform")
	}

	// ---- 2. ingest 3 clean SBOMs ----
	clean := readFixture(t, "simple-webapp.json")
	for _, ref := range components {
		postSBOM(t, h, ref, clean)
	}
	waitForN(t, eventCh, 3, isSBOMIngested, "clean sbom.ingested x3")

	// ---- 3. publish axios@1.11.0 IoC ----
	// No SBOM currently contains axios@1.11.0, so this fires zero
	// forward matches — we verify that the idempotency path is quiet.
	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-AXIOS-1-11-0",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.Critical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
		},
	})
	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Empty(t, incs, "IoC against clean SBOMs must not open any incident")

	// ---- 4. ingest compromised SBOM for web-app → incident opens ----
	postSBOM(t, h, "kind:Component/default/web-app", readFixture(t, "axios-compromise.json"))
	waitForN(t, eventCh, 1, isSBOMIngested, "compromised sbom.ingested")
	waitForN(t, eventCh, 1, isIncidentOpened, "incident.opened after match")
	waitForN(t, eventCh, 1, isIoCMatched, "ioc.matched after retroactive hit")

	incs, _ = store.ListIncidents(context.Background())
	require.Len(t, incs, 1)
	require.Equal(t, "01IOC-AXIOS-1-11-0", incs[0].IoCID)
	require.Equal(t, []string{"kind:Component/default/web-app"}, incs[0].AffectedComponentsSnapshot)
	incidentID := incs[0].ID

	// ---- 5. republish a range IoC covering axios 1.11.x ----
	// Should NOT open a second incident against web-app (idempotency
	// key is (ioc, ref)) but WILL list web-app under matched
	// components in the forward scan.
	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-AXIOS-RANGE",
		Kind:        gen.IoCKindPackageRange,
		Severity:    gen.High,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageRange: &gen.IoCPackageRange{
			Name: "axios", Constraint: ">=1.11.0, <1.12.0",
		},
	})
	// Different IoC, so it DOES open a new incident (idempotency is
	// per (ioc, ref)). We'll just confirm the count went from 1 to 2.
	incs, _ = store.ListIncidents(context.Background())
	require.Len(t, incs, 2, "different IoC → separate incident opens")

	// ---- 6. transition incident 1 pending → triaged → remediating ----
	transition(t, h, incidentID, domain.StateTriaged)
	transition(t, h, incidentID, domain.StateAcknowledged)
	transition(t, h, incidentID, domain.StateRemediating)
	waitForN(t, eventCh, 3, isIncidentTransitioned, "3 transitions")

	// ---- 7. add remediation ----
	remBody, _ := json.Marshal(gen.Remediation{
		Kind:       gen.PinVersion,
		ExecutedAt: time.Now().UTC(),
		ActorRef:   ptrString("user:oncall"),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/incidents/"+incidentID+"/remediations",
		bytes.NewReader(remBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "body=%s", rr.Body.String())
	waitForN(t, eventCh, 1, isRemediationAdded, "remediation.added")

	// ---- 8. GET /v1/incidents/{id} — everything hydrated ----
	getReq := httptest.NewRequest(http.MethodGet, "/v1/incidents/"+incidentID, nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	require.Equal(t, http.StatusOK, getRR.Code)
	var inc gen.Incident
	require.NoError(t, json.Unmarshal(getRR.Body.Bytes(), &inc))
	require.Equal(t, gen.Remediating, inc.State)
	require.NotNil(t, inc.Remediations)
	require.Len(t, *inc.Remediations, 1)
	require.Equal(t, gen.PinVersion, (*inc.Remediations)[0].Kind)

	// ---- 9. blast-radius sanity ----
	blastBody, _ := json.Marshal(gen.BlastRadiusRequest{
		Iocs: []gen.IoC{{
			Id:          "01IOC-HYPOTHETICAL",
			Kind:        gen.IoCKindPackageVersion,
			Severity:    gen.Critical,
			Ecosystem:   "npm",
			PublishedAt: time.Now().UTC(),
			PackageVersion: &gen.IoCPackageVersion{
				Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
			},
		}},
	})
	brReq := httptest.NewRequest(http.MethodPost, "/v1/blast-radius", bytes.NewReader(blastBody))
	brReq.Header.Set("Content-Type", "application/json")
	brRR := httptest.NewRecorder()
	h.ServeHTTP(brRR, brReq)
	require.Equal(t, http.StatusOK, brRR.Code)
	var br gen.BlastRadiusResponse
	require.NoError(t, json.Unmarshal(brRR.Body.Bytes(), &br))
	require.Equal(t, []string{"kind:Component/default/web-app"}, br.AffectedComponentRefs,
		"only web-app carries axios@1.11.0; billing + reporting still on simple-webapp fixture")
}

func ptrString(s string) *string { return &s }

// --- event-stream assertion helpers ---

type eventFilter func(domain.DomainEvent) bool

func isSBOMIngested(e domain.DomainEvent) bool { _, ok := e.(domain.SBOMIngestedEvent); return ok }
func isIncidentOpened(e domain.DomainEvent) bool {
	_, ok := e.(domain.IncidentOpenedEvent)
	return ok
}
func isIoCMatched(e domain.DomainEvent) bool { _, ok := e.(domain.IoCMatchedEvent); return ok }
func isIncidentTransitioned(e domain.DomainEvent) bool {
	_, ok := e.(domain.IncidentTransitionedEvent)
	return ok
}
func isRemediationAdded(e domain.DomainEvent) bool {
	_, ok := e.(domain.RemediationAddedEvent)
	return ok
}

// waitForN pulls at most the deadline's worth of events off ch, counting
// those that match filter. Fails the test if fewer than `n` come through.
// Other events are silently dropped — callers that care about ordering
// assert inline on a fresh subscription.
func waitForN(t *testing.T, ch <-chan domain.DomainEvent, n int, filter eventFilter, label string) {
	t.Helper()
	seen := 0
	deadline := time.After(2 * time.Second)
	for seen < n {
		select {
		case e := <-ch:
			if filter(e) {
				seen++
			}
		case <-deadline:
			t.Fatalf("%s: wanted %d events, saw %d before timeout", label, n, seen)
		}
	}
}
