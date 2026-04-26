// Package github augments the npm-derived publisher snapshot with
// upstream-source-repo metadata: latest release tag, release author,
// publish timestamp. The GitHub ingestor is reachable for any package
// whose snapshot already carries a `SourceRepoURL` pointing at
// github.com — Theme F1.2 wires this on top of the npm ingestor.
//
// Authentication: when GITHUB_TOKEN is set, the client sends it as a
// Bearer token (5000 req/hr quota). Without a token, GitHub's
// unauthenticated 60 req/hr limit applies — the rate-limit-aware
// httpx layer surfaces 429s back to the scheduler so a misconfigured
// deployment doesn't cascade-fail the rest of the engine.
package github

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

// DefaultAPIBase is the public github.com REST endpoint. Tests
// override via Config.APIBase to point at httptest.Server.
const DefaultAPIBase = "https://api.github.com"

// Config tunes the ingestor. Zero values produce sensible production
// defaults. Token, when non-empty, is sent as `Authorization: Bearer
// <token>`.
type Config struct {
	APIBase   string        // defaults to DefaultAPIBase
	Token     string        // optional; from GITHUB_TOKEN env if set
	Timeout   time.Duration // per-request, defaults to 10s
	UserAgent string        // sent on every request, defaults to "rampart-engine"
}

// Ingestor produces a release-info supplement for a snapshot whose
// SourceRepoURL points at github.com.
type Ingestor struct {
	apiBase string
	client  *httpx.Client
}

// New builds an Ingestor with the supplied Config.
func New(cfg Config) *Ingestor {
	if cfg.APIBase == "" {
		cfg.APIBase = DefaultAPIBase
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "rampart-engine"
	}
	// Authenticated quota is 5000/hr ≈ 1.4/s. Unauthenticated is
	// 60/hr ≈ 0.017/s — too slow to be useful at any reasonable
	// batch size. We rate ourselves at the unauth ceiling regardless;
	// the 429 backoff path is what stops a misconfigured deployment
	// from spamming.
	limit := rate.Limit(0.5) // ≤ 30 req/min, comfortable inside 60/hr unauth
	if cfg.Token != "" {
		limit = rate.Limit(1.0) // ≤ 60 req/min, well below 5000/hr authed
	}
	c := httpx.New(cfg.Timeout, limit, 5)
	c.Header.Set("Accept", "application/vnd.github+json")
	c.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	c.Header.Set("User-Agent", cfg.UserAgent)
	if cfg.Token != "" {
		c.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	return &Ingestor{
		apiBase: strings.TrimRight(cfg.APIBase, "/"),
		client:  c,
	}
}

type releaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

// LatestRelease fetches /repos/{owner}/{repo}/releases/latest and
// returns the parsed payload alongside the raw bytes (useful to
// callers that want to merge into PublisherSnapshot.RawData).
//
// Errors from httpx (ErrNotFound for repos without releases,
// ErrRateLimited, ErrServerFailure) bubble up untouched.
func (i *Ingestor) LatestRelease(ctx context.Context, owner, repo string) (*Release, []byte, error) {
	if owner == "" || repo == "" {
		return nil, nil, fmt.Errorf("github ingestor: owner+repo required (got %q / %q)", owner, repo)
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/latest", i.apiBase, url.PathEscape(owner), url.PathEscape(repo))
	body, err := i.client.Get(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	var resp releaseResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("github ingestor: decode release for %s/%s: %w", owner, repo, err)
	}
	return &Release{
		TagName:     resp.TagName,
		Name:        resp.Name,
		PublishedAt: resp.PublishedAt.UTC(),
		HTMLURL:     resp.HTMLURL,
		AuthorLogin: resp.Author.Login,
	}, body, nil
}

// Release is the parsed shape callers consume.
type Release struct {
	TagName     string
	Name        string
	PublishedAt time.Time
	HTMLURL     string
	AuthorLogin string
}

// Ingest implements publisher.Ingestor for github-source packages.
// packageRef must follow `gomod:github.com/<owner>/<repo>` or carry a
// `github.com/<owner>/<repo>` suffix the parser can extract.
//
// Returned snapshots set:
//   - LatestVersion = release tag (`v1.8.0` etc.)
//   - LatestVersionPublishedAt = release publish time
//   - PublishMethod = "unknown" (sigstore detection is a Theme F2 follow-up)
//   - SourceRepoURL = canonical https://github.com/<owner>/<repo>
//   - RawData = raw GitHub release JSON
//
// Maintainers is left empty — GitHub releases identify a single
// publisher (release author), not the maintainer roster; that lives
// on the npm side.
func (i *Ingestor) Ingest(ctx context.Context, packageRef string) (*domain.PublisherSnapshot, error) {
	owner, repo, err := ParseGithubRef(packageRef)
	if err != nil {
		return nil, err
	}
	rel, raw, err := i.LatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	repoURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	publishedAt := rel.PublishedAt
	snap := &domain.PublisherSnapshot{
		PackageRef:               packageRef,
		LatestVersion:            rel.TagName,
		LatestVersionPublishedAt: &publishedAt,
		PublishMethod:            "unknown",
		SourceRepoURL:            &repoURL,
		RawData:                  raw,
	}
	if rel.AuthorLogin != "" {
		snap.Maintainers = []domain.Maintainer{{Username: rel.AuthorLogin}}
	}
	return snap, nil
}

// ParseGithubRef extracts (owner, repo) from a package_ref. Accepts:
//
//   - `gomod:github.com/<owner>/<repo>`  (Go modules canonical form)
//   - `gomod:github.com/<owner>/<repo>/<sub-path>` (drops sub-path)
//   - `github:<owner>/<repo>`             (explicit)
//
// Other forms are rejected so callers don't accidentally hit the
// wrong endpoint with a parsed-from-ambiguous-string owner.
func ParseGithubRef(packageRef string) (string, string, error) {
	if rest, ok := strings.CutPrefix(packageRef, "github:"); ok {
		owner, repo, ok := strings.Cut(rest, "/")
		if !ok || owner == "" || repo == "" {
			return "", "", fmt.Errorf("github ingestor: malformed github: ref %q", packageRef)
		}
		return owner, repo, nil
	}
	if rest, ok := strings.CutPrefix(packageRef, "gomod:github.com/"); ok {
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("github ingestor: malformed gomod github.com ref %q", packageRef)
		}
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("github ingestor: package_ref %q does not point at github.com", packageRef)
}
