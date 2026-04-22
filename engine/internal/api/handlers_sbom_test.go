package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

// sbomsPath builds the /v1/components/{ref}/sboms URL with the ref
// path-escaped — ServeMux path params capture a single segment, so
// forward slashes inside the ref must be %2F.
func sbomsPath(componentRef string) string {
	return "/v1/components/" + url.PathEscape(componentRef) + "/sboms"
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "lockfiles", name)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

func postComponent(t *testing.T, h http.Handler, ref, owner string) {
	t.Helper()
	body, _ := json.Marshal(gen.Component{
		Ref:       ref,
		Kind:      "Component",
		Namespace: "default",
		Name:      "placeholder",
		Owner:     &owner,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/components", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Less(t, rr.Code, 400, "pre-seed component failed: %s", rr.Body.String())
}

func postSBOM(t *testing.T, h http.Handler, ref string, fixture []byte) gen.SBOM {
	t.Helper()
	body, _ := json.Marshal(gen.SBOMSubmission{
		Ecosystem:    "npm",
		SourceFormat: gen.SBOMSubmissionSourceFormatNpmPackageLockV3,
		Content:      fixture,
	})
	req := httptest.NewRequest(http.MethodPost, sbomsPath(ref), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "post sbom: %s", rr.Body.String())
	var got gen.SBOM
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	return got
}

func TestSubmitSBOM_CleanLockfile_NoIncidents(t *testing.T) {
	h, store := newTestServer(t)
	postComponent(t, h, "kind:Component/default/clean", "team-a")

	got := postSBOM(t, h, "kind:Component/default/clean", readFixture(t, "simple-webapp.json"))
	require.NotEmpty(t, got.Id)
	require.Equal(t, "kind:Component/default/clean", got.ComponentRef)

	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Empty(t, incs, "no IoCs published → no incidents")
}

func TestSubmitSBOM_MatchesExistingIoC_OpensIncidentAndEmitsSSE(t *testing.T) {
	store := memory.New()
	bus := events.NewBus(16)
	h := api.NewServer(store, bus, time.Minute).Handler()

	// Pre-seed an IoC for axios@1.11.0.
	ioc := domain.IoC{
		ID:          "01IOC-AXIOS-1-11-0",
		Kind:        domain.IoCKindPackageVersion,
		Severity:    domain.SeverityCritical,
		Ecosystem:   "npm",
		PublishedAt: time.Now().UTC(),
		PackageVersion: &domain.IoCPackageVersion{
			Name:    "axios",
			Version: "1.11.0",
			PURL:    "pkg:npm/axios@1.11.0",
		},
	}
	require.NoError(t, store.UpsertIoC(context.Background(), ioc))

	// Subscribe before submitting so we don't miss the event.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	eventCh, unsubscribe := bus.Subscribe(ctx)
	defer unsubscribe()

	postComponent(t, h, "kind:Component/default/web", "team-a")
	sbom := postSBOM(t, h, "kind:Component/default/web", readFixture(t, "axios-compromise.json"))
	require.NotEmpty(t, sbom.Id)

	var sawIngested, sawOpened, sawMatched bool
	for !sawIngested || !sawOpened || !sawMatched {
		select {
		case ev := <-eventCh:
			switch e := ev.(type) {
			case domain.SBOMIngestedEvent:
				sawIngested = true
				require.Equal(t, "kind:Component/default/web", e.ComponentRef)
			case domain.IncidentOpenedEvent:
				sawOpened = true
				require.Equal(t, ioc.ID, e.IoCID)
				require.Equal(t, []string{"kind:Component/default/web"}, e.AffectedComponentsSnapshot)
			case domain.IoCMatchedEvent:
				sawMatched = true
				require.Equal(t, ioc.ID, e.IoCID)
				require.Equal(t, []string{"kind:Component/default/web"}, e.MatchedComponents)
			}
		case <-ctx.Done():
			t.Fatalf("timeout — sawIngested=%v sawOpened=%v sawMatched=%v", sawIngested, sawOpened, sawMatched)
		}
	}

	incs, err := store.ListIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incs, 1, "one open incident per (ioc, component)")
	require.Equal(t, domain.StatePending, incs[0].State)
}

func TestSubmitSBOM_UnregisteredComponent_404(t *testing.T) {
	h, _ := newTestServer(t)
	body, _ := json.Marshal(gen.SBOMSubmission{
		Ecosystem:    "npm",
		SourceFormat: gen.SBOMSubmissionSourceFormatNpmPackageLockV3,
		Content:      []byte(`{"lockfileVersion":3,"packages":{}}`),
	})
	req := httptest.NewRequest(http.MethodPost, sbomsPath("kind:Component/default/phantom"),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "COMPONENT_NOT_FOUND", e.Code)
}

func TestSubmitSBOM_MalformedLockfile_400(t *testing.T) {
	h, _ := newTestServer(t)
	postComponent(t, h, "kind:Component/default/web", "team-a")

	body, _ := json.Marshal(gen.SBOMSubmission{
		Ecosystem:    "npm",
		SourceFormat: gen.SBOMSubmissionSourceFormatNpmPackageLockV3,
		Content:      []byte("not json at all"),
	})
	req := httptest.NewRequest(http.MethodPost, sbomsPath("kind:Component/default/web"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSubmitSBOM_WrongSourceFormat_400(t *testing.T) {
	h, _ := newTestServer(t)
	postComponent(t, h, "kind:Component/default/web", "team-a")
	// Build the JSON manually — gen type constrains the enum.
	body := []byte(`{"ecosystem":"npm","sourceFormat":"npm-v1","content":"` +
		base64.StdEncoding.EncodeToString([]byte(`{"lockfileVersion":3,"packages":{}}`)) + `"}`)
	req := httptest.NewRequest(http.MethodPost, sbomsPath("kind:Component/default/web"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetSBOM_RoundTrip(t *testing.T) {
	h, _ := newTestServer(t)
	postComponent(t, h, "kind:Component/default/web", "team-a")
	sbom := postSBOM(t, h, "kind:Component/default/web", readFixture(t, "simple-webapp.json"))

	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/"+sbom.Id, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var got gen.SBOM
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, sbom.Id, got.Id)
}

func TestGetSBOM_MissingReturns404(t *testing.T) {
	h, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/NOPE", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestListSBOMsByComponent_ReturnsStored(t *testing.T) {
	h, _ := newTestServer(t)
	postComponent(t, h, "kind:Component/default/web", "team-a")
	_ = postSBOM(t, h, "kind:Component/default/web", readFixture(t, "simple-webapp.json"))
	_ = postSBOM(t, h, "kind:Component/default/web", readFixture(t, "axios-compromise.json"))

	req := httptest.NewRequest(http.MethodGet, sbomsPath("kind:Component/default/web"), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var page struct {
		Items []gen.SBOM `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Items, 2)
}
