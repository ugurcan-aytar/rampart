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

func postIoC(t *testing.T, h http.Handler, ioc gen.IoC) gen.IoC {
	t.Helper()
	body, _ := json.Marshal(ioc)
	req := httptest.NewRequest(http.MethodPost, "/v1/iocs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "post ioc: %s", rr.Body.String())
	var got gen.IoC
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	return got
}

// TestSubmitIoC_ForwardMatch_OpensIncidentPerComponent is the flagship
// demo scenario in miniature: seed three components with SBOMs that
// carry axios@1.11.0, publish one IoC, expect three incidents opened.
func TestSubmitIoC_ForwardMatch_OpensIncidentPerComponent(t *testing.T) {
	store := memory.New()
	bus := events.NewBus(64)
	h := api.NewServer(store, bus, time.Minute).Handler()

	components := []string{
		"kind:Component/default/web-app",
		"kind:Component/default/billing",
		"kind:Component/default/reporting",
	}
	for _, ref := range components {
		postComponent(t, h, ref, "team-platform")
		postSBOM(t, h, ref, readFixture(t, "axios-compromise.json"))
	}

	// Subscribe BEFORE publishing the IoC to capture the incident
	// events without a race.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, unsubscribe := bus.Subscribe(ctx)
	defer unsubscribe()

	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-AXIOS-1-11-0",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.Critical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name:    "axios",
			Version: "1.11.0",
			Purl:    "pkg:npm/axios@1.11.0",
		},
	})

	openedIDs := map[string]string{}
	var sawMatched bool
	deadline := time.After(2 * time.Second)
	for len(openedIDs) < 3 || !sawMatched {
		select {
		case ev := <-eventCh:
			switch e := ev.(type) {
			case domain.IncidentOpenedEvent:
				require.Equal(t, "01IOC-AXIOS-1-11-0", e.IoCID)
				require.Len(t, e.AffectedComponentsSnapshot, 1)
				openedIDs[e.AffectedComponentsSnapshot[0]] = e.IncidentID
			case domain.IoCMatchedEvent:
				sawMatched = true
				require.ElementsMatch(t, components, e.MatchedComponents)
			}
		case <-deadline:
			t.Fatalf("timeout — opened=%d matched=%v", len(openedIDs), sawMatched)
		}
	}

	require.Len(t, openedIDs, 3, "one incident per affected component")
	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incs, 3)
	for _, inc := range incs {
		require.Equal(t, domain.StatePending, inc.State)
		require.Equal(t, "01IOC-AXIOS-1-11-0", inc.IoCID)
	}
}

// Idempotency: republishing the same IoC while incidents are open must
// not create duplicates.
func TestSubmitIoC_Idempotent(t *testing.T) {
	store := memory.New()
	bus := events.NewBus(32)
	h := api.NewServer(store, bus, time.Minute).Handler()

	postComponent(t, h, "kind:Component/default/web", "team")
	postSBOM(t, h, "kind:Component/default/web", readFixture(t, "axios-compromise.json"))

	body := gen.IoC{
		Id:          "01IOC-DUP",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.Critical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
		},
	}
	_ = postIoC(t, h, body)
	_ = postIoC(t, h, body) // second publish, same id

	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incs, 1, "idempotency key (ioc, ref) must not produce duplicates")
}

func TestSubmitIoC_InvalidKind_400(t *testing.T) {
	h, _ := newTestServer(t)
	body := gen.IoC{
		Id:          "01IOC-BAD",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.High,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		// no body set → Validate() fails
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/iocs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "INVALID_IOC", e.Code)
}

func TestSubmitIoC_InvalidSemverConstraint_400(t *testing.T) {
	h, _ := newTestServer(t)
	body := gen.IoC{
		Id:          "01IOC-BADRANGE",
		Kind:        gen.IoCKindPackageRange,
		Severity:    gen.High,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageRange: &gen.IoCPackageRange{
			Name:       "axios",
			Constraint: "not a semver",
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/iocs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "INVALID_CONSTRAINT", e.Code)
}

func TestListIoCs_ReturnsStored(t *testing.T) {
	h, _ := newTestServer(t)
	_ = postIoC(t, h, gen.IoC{
		Id:          "01IOC-A",
		Kind:        gen.IoCKindPackageVersion,
		Severity:    gen.Critical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &gen.IoCPackageVersion{
			Name: "axios", Version: "1.11.0", Purl: "pkg:npm/axios@1.11.0",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/iocs", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var page gen.IoCPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, "01IOC-A", page.Items[0].Id)
}
