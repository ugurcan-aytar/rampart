// Package npm ingests publisher metadata from registry.npmjs.org into
// rampart's PublisherSnapshot shape. The package_ref convention is
// `npm:<package-name>` (`npm:axios`, `npm:@scope/pkg`).
package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
)

// DefaultRegistry is the public npm registry. Tests override via
// Config.RegistryBase to point at httptest.Server.
const DefaultRegistry = "https://registry.npmjs.org"

// PackageRefPrefix is the `<ecosystem>:` prefix the scheduler dispatches
// on. Exported so the scheduler can route refs without hard-coding the
// string.
const PackageRefPrefix = "npm:"

// Config tunes the ingestor. Zero values produce sensible production
// defaults (public npm registry, ~100 req/min — well below the
// registry's documented ~5000 req/5min limit per IP, conservative
// enough that a backoff-on-429 path is reachable but rare).
type Config struct {
	RegistryBase string        // defaults to DefaultRegistry
	Timeout      time.Duration // per-request, defaults to 10s
	UserAgent    string        // sent on every request, defaults to "rampart-engine"
}

// Ingestor implements publisher.Ingestor for npm packages.
type Ingestor struct {
	registryBase string
	client       *httpx.Client
}

// New builds an Ingestor with the supplied Config. Pass a zero Config
// for the production defaults.
func New(cfg Config) *Ingestor {
	if cfg.RegistryBase == "" {
		cfg.RegistryBase = DefaultRegistry
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "rampart-engine"
	}
	// 100 req/min ≈ 1.67/s — burst 5 lets the scheduler clear a small
	// queue at once without immediately throttling.
	c := httpx.New(cfg.Timeout, rate.Limit(100.0/60.0), 5)
	c.Header.Set("Accept", "application/vnd.npm.install-v1+json, application/json")
	c.Header.Set("User-Agent", cfg.UserAgent)
	return &Ingestor{
		registryBase: strings.TrimRight(cfg.RegistryBase, "/"),
		client:       c,
	}
}

// registryResponse is the subset of the npm packument we read.
// The full schema is documented at https://github.com/npm/registry/.
type registryResponse struct {
	Name        string                 `json:"name"`
	Maintainers []apiMaintainer        `json:"maintainers"`
	DistTags    map[string]string      `json:"dist-tags"`
	Time        map[string]string      `json:"time"`
	Versions    map[string]versionMeta `json:"versions"`
	Repository  *repositoryField       `json:"repository,omitempty"`
}

type apiMaintainer struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type versionMeta struct {
	// `dist.signatures` is present when a version was published with
	// a registry-validated provenance. Older / token-published
	// versions omit it. Used to derive PublishMethod.
	Dist struct {
		Signatures []json.RawMessage `json:"signatures,omitempty"`
	} `json:"dist"`
}

// repositoryField appears as either a string ("git+https://…") or a
// {type, url} object across the npm corpus. UnmarshalJSON normalises
// both shapes.
type repositoryField struct {
	URL string
}

func (r *repositoryField) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		r.URL = s
		return nil
	}
	var obj struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	r.URL = obj.URL
	return nil
}

// Ingest fetches the npm packument for packageRef and assembles a
// PublisherSnapshot. Errors from httpx (ErrNotFound, ErrRateLimited,
// ErrServerFailure) bubble up untouched so the scheduler can decide
// what to log + whether to retry on the next tick.
func (i *Ingestor) Ingest(ctx context.Context, packageRef string) (*domain.PublisherSnapshot, error) {
	pkgName, ok := strings.CutPrefix(packageRef, PackageRefPrefix)
	if !ok {
		return nil, fmt.Errorf("npm ingestor: package_ref %q is not %s-prefixed", packageRef, PackageRefPrefix)
	}
	if pkgName == "" {
		return nil, fmt.Errorf("npm ingestor: empty package name in %q", packageRef)
	}
	// Scoped names (@scope/pkg) keep the slash unencoded per npm's URL
	// convention; the @ does not need escaping either, but PathEscape
	// handles edge characters safely.
	endpoint := i.registryBase + "/" + url.PathEscape(pkgName)
	// PathEscape encodes "/" as "%2F"; npm wants the slash literal for
	// scoped packages. Restore it.
	endpoint = strings.ReplaceAll(endpoint, "%2F", "/")

	body, err := i.client.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var resp registryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("npm ingestor: decode response for %s: %w", packageRef, err)
	}

	snap := &domain.PublisherSnapshot{
		PackageRef:    packageRef,
		Maintainers:   make([]domain.Maintainer, 0, len(resp.Maintainers)),
		PublishMethod: "unknown",
		RawData:       body,
	}
	for _, m := range resp.Maintainers {
		snap.Maintainers = append(snap.Maintainers, domain.Maintainer{
			Email:    m.Email,
			Name:     m.Name,
			Username: m.Name, // npm packument lacks a separate username field; reuse name.
		})
	}

	// Latest version is dist-tags.latest; the publish timestamp lives
	// in time[<version>].
	if latest, ok := resp.DistTags["latest"]; ok && latest != "" {
		snap.LatestVersion = latest
		if when, ok := resp.Time[latest]; ok {
			if t, err := time.Parse(time.RFC3339, when); err == nil {
				u := t.UTC()
				snap.LatestVersionPublishedAt = &u
			}
		}
		// Publish method: presence of dist.signatures on the latest
		// version implies an OIDC/sigstore-validated trusted publisher.
		// Absence means the publish used a regular auth token (or is
		// older than the trusted-publisher rollout).
		if v, ok := resp.Versions[latest]; ok && len(v.Dist.Signatures) > 0 {
			snap.PublishMethod = "oidc-trusted-publisher"
		} else {
			snap.PublishMethod = "token"
		}
	}

	if resp.Repository != nil && resp.Repository.URL != "" {
		// Strip the "git+" prefix so the URL is dereferenceable in a
		// browser without manual surgery; preserve everything else.
		urlStr := strings.TrimPrefix(resp.Repository.URL, "git+")
		snap.SourceRepoURL = &urlStr
	}

	return snap, nil
}
