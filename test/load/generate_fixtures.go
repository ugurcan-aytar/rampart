// generate_fixtures.go writes the v0.2.0 load-test corpus to disk:
//
//   fixtures/components.jsonl       — N components (default 10000)
//   fixtures/sboms/<idx>.json       — one SBOM per component (npm package-lock-v3 shape, base64-payload-ready)
//   fixtures/iocs.jsonl             — M IoCs (default 500: 350 packageVersion + 100 packageRange + 50 anomaly)
//   fixtures/snapshots.jsonl        — K publisher snapshots (default 200, three anomaly types interleaved)
//   fixtures/manifest.json          — counts + the package-name pool the IoCs target
//
// Determinism: a single uint64 seed (default `1`) drives every random
// choice. Re-running with the same seed produces byte-identical output;
// the load orchestrator can keep a fixed seed for repeatable runs.
//
// Run:
//   go run ./test/load/generate_fixtures.go [-out test/load/fixtures] [-components 10000] [-iocs 500] [-snapshots 200] [-seed 1]
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
)

type cfg struct {
	OutDir         string
	NumComponents  int
	NumIoCs        int
	NumSnapshots   int
	Seed           uint64
	PackagesPerSBOM int
}

func main() {
	c := cfg{}
	flag.StringVar(&c.OutDir, "out", "test/load/fixtures", "output directory")
	flag.IntVar(&c.NumComponents, "components", 10_000, "components + SBOMs to generate")
	flag.IntVar(&c.NumIoCs, "iocs", 500, "IoCs to generate")
	flag.IntVar(&c.NumSnapshots, "snapshots", 200, "publisher snapshots to generate")
	flag.IntVar(&c.PackagesPerSBOM, "packages", 25, "packages per SBOM (avg)")
	flag.Uint64Var(&c.Seed, "seed", 1, "PRNG seed for deterministic output")
	flag.Parse()

	if err := run(c); err != nil {
		fmt.Fprintln(os.Stderr, "generate_fixtures:", err)
		os.Exit(1)
	}
}

func run(c cfg) error {
	if err := os.MkdirAll(filepath.Join(c.OutDir, "sboms"), 0o755); err != nil {
		return err
	}

	// Single PRNG threaded everywhere — same seed → same output.
	r := rand.New(rand.NewPCG(c.Seed, c.Seed^0xdeadbeef))

	// 20-team owner pool keeps owner-filter + per-team aggregations
	// realistic without producing 10 000 unique strings.
	owners := make([]string, 20)
	for i := range owners {
		owners[i] = fmt.Sprintf("team-%02d", i)
	}

	// Package pool — 200 fake-but-stable npm names. Some are reused
	// across SBOMs so IoCs have a dependency-tree to match against.
	packagePool := buildPackagePool(200)

	// Pick a subset that IoCs will target. ~25% of the pool — the
	// target set has to be large enough that injecting one target per
	// SBOM at 5% rate produces ~5-30 matches per IoC (real-world CVE
	// blast-radius per GHSA / Snyk advisories). An earlier 10-target
	// pool combined with 30% inject rate produced ~270 matches per
	// IoC and 135k incidents, which is pathological for the matcher
	// fan-out and unrepresentative of real fleets.
	iocTargetCount := 50
	iocTargets := make([]string, iocTargetCount)
	for i := range iocTargets {
		iocTargets[i] = packagePool[r.IntN(len(packagePool))]
	}

	// Manifest first so the orchestrator knows the target pool ahead
	// of querying.
	manifest := map[string]any{
		"components":  c.NumComponents,
		"iocs":        c.NumIoCs,
		"snapshots":   c.NumSnapshots,
		"packages_per_sbom": c.PackagesPerSBOM,
		"seed":        c.Seed,
		"ioc_targets": iocTargets,
		"owners":      owners,
	}
	if err := writeJSON(filepath.Join(c.OutDir, "manifest.json"), manifest); err != nil {
		return err
	}

	// Components — JSONL, one line per record so the orchestrator can
	// stream + parallelise without parsing the whole file.
	compsFile, err := os.Create(filepath.Join(c.OutDir, "components.jsonl"))
	if err != nil {
		return err
	}
	defer compsFile.Close()
	enc := json.NewEncoder(compsFile)

	// SBOMs — one file per component. Embedded in the same loop so
	// the SBOM idx aligns with the component idx.
	for i := 0; i < c.NumComponents; i++ {
		ref := fmt.Sprintf("kind:Component/default/svc-%05d", i)
		owner := owners[r.IntN(len(owners))]
		comp := map[string]any{
			"ref":       ref,
			"kind":      "Component",
			"namespace": "default",
			"name":      fmt.Sprintf("svc-%05d", i),
			"owner":     owner,
		}
		if err := enc.Encode(comp); err != nil {
			return err
		}

		// SBOM: pick ~packages-per-SBOM packages from the pool. ~30%
		// chance to include at least one IoC target so we get
		// matchable incidents at non-trivial rate.
		pkgCount := c.PackagesPerSBOM/2 + r.IntN(c.PackagesPerSBOM)
		picked := make(map[string]string, pkgCount)
		for j := 0; j < pkgCount; j++ {
			name := packagePool[r.IntN(len(packagePool))]
			picked[name] = fmt.Sprintf("%d.%d.%d", r.IntN(20), r.IntN(20), r.IntN(20))
		}
		// Inject an IoC target into ~5% of SBOMs. Combined with the
		// 50-target pool above this settles to ~10 matches per IoC —
		// in line with mid-severity GHSA advisories that hit a handful
		// of services, not every service in the fleet.
		if r.IntN(100) < 5 {
			target := iocTargets[r.IntN(len(iocTargets))]
			picked[target] = "1.11.0" // matches the packageVersion IoCs we'll emit
		}

		lockfile := buildPackageLock(fmt.Sprintf("svc-%05d", i), picked)
		body, err := json.Marshal(lockfile)
		if err != nil {
			return err
		}
		// Pre-encode to base64 — the SBOMSubmission schema wants the
		// content base64-encoded; doing it here keeps the orchestrator
		// hot loop free of the encoding cost.
		submission := map[string]any{
			"ecosystem":    "npm",
			"sourceFormat": "npm-package-lock-v3",
			"content":      base64.StdEncoding.EncodeToString(body),
		}
		if err := writeJSON(filepath.Join(c.OutDir, "sboms", fmt.Sprintf("%05d.json", i)), submission); err != nil {
			return err
		}
	}

	// IoCs — JSONL.
	iocsFile, err := os.Create(filepath.Join(c.OutDir, "iocs.jsonl"))
	if err != nil {
		return err
	}
	defer iocsFile.Close()
	iocEnc := json.NewEncoder(iocsFile)

	severities := []string{"low", "medium", "high", "critical"}
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < c.NumIoCs; i++ {
		id := fmt.Sprintf("01IOC-LOAD-%05d", i)
		sev := severities[r.IntN(len(severities))]
		published := now.Add(-time.Duration(r.IntN(720)) * time.Hour)
		var ioc map[string]any
		switch {
		case i < 350: // 350 packageVersion
			target := iocTargets[r.IntN(len(iocTargets))]
			ioc = map[string]any{
				"id": id, "kind": "packageVersion", "severity": sev,
				"ecosystem": "npm", "source": "load-test",
				"publishedAt": published.Format(time.RFC3339),
				"packageVersion": map[string]any{
					"name":    target,
					"version": "1.11.0",
					"purl":    fmt.Sprintf("pkg:npm/%s@1.11.0", target),
				},
			}
		case i < 450: // 100 packageRange
			target := iocTargets[r.IntN(len(iocTargets))]
			ioc = map[string]any{
				"id": id, "kind": "packageRange", "severity": sev,
				"ecosystem": "npm", "source": "load-test",
				"publishedAt": published.Format(time.RFC3339),
				"packageRange": map[string]any{
					"name":       target,
					"constraint": ">=1.11.0 <1.12.0",
				},
			}
		default: // 50 anomaly (publisherAnomaly + AnomalyBody variant — Theme F3 / ADR-0014)
			target := iocTargets[r.IntN(len(iocTargets))]
			anomalyKinds := []string{"new_maintainer_email", "oidc_regression", "version_jump"}
			confidences := []string{"high", "medium", "low"}
			ioc = map[string]any{
				"id": id, "kind": "publisherAnomaly", "severity": sev,
				"ecosystem": "npm", "source": "load-test",
				"publishedAt": published.Format(time.RFC3339),
				"anomalyBody": map[string]any{
					"kind":        anomalyKinds[r.IntN(len(anomalyKinds))],
					"confidence":  confidences[r.IntN(len(confidences))],
					"explanation": "synthetic load-test anomaly",
					"packageRef":  "npm:" + target,
					"evidence":    map[string]any{"synthetic": true},
				},
			}
		}
		if err := iocEnc.Encode(ioc); err != nil {
			return err
		}
	}

	// Publisher snapshots — JSONL. The anomaly orchestrator reads
	// these via storage; since the orchestrator's storage is the
	// engine's, we'd POST them through an ingestion endpoint... but
	// there's no public endpoint to seed snapshots (Theme F1 ingests
	// from npm/github APIs). For load-test purposes the orchestrator
	// only needs the file present so the perf doc's reproduction
	// section is self-describing; actual snapshot insertion is via
	// the engine running with RAMPART_PUBLISHER_ENABLED=true.
	snapsFile, err := os.Create(filepath.Join(c.OutDir, "snapshots.jsonl"))
	if err != nil {
		return err
	}
	defer snapsFile.Close()
	snapEnc := json.NewEncoder(snapsFile)
	for i := 0; i < c.NumSnapshots; i++ {
		target := iocTargets[i%len(iocTargets)]
		snap := map[string]any{
			"packageRef":    "npm:" + target,
			"snapshotAt":    now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
			"latestVersion": fmt.Sprintf("1.%d.0", i%20),
			"publishMethod": []string{"oidc-trusted-publisher", "token", "unknown"}[r.IntN(3)],
			"maintainers": []map[string]any{
				{"email": fmt.Sprintf("maint%d@example.com", r.IntN(5)), "name": "Mock"},
			},
		}
		if err := snapEnc.Encode(snap); err != nil {
			return err
		}
	}

	fmt.Printf("wrote %d components + SBOMs, %d IoCs, %d snapshots to %s\n",
		c.NumComponents, c.NumIoCs, c.NumSnapshots, c.OutDir)
	return nil
}

// buildPackagePool returns a deterministic list of fake-but-realistic
// npm package names. The pool is generated from a fixed prefix list +
// suffix counter so the same n always yields the same names.
func buildPackagePool(n int) []string {
	prefixes := []string{
		"axios", "lodash", "react", "vue", "express", "typescript", "webpack",
		"babel", "eslint", "jest", "rollup", "vite", "moment", "uuid", "yaml",
		"chalk", "commander", "fs-extra", "glob", "request", "redux", "next",
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("%s-%03d", prefixes[i%len(prefixes)], i)
	}
	return out
}

// buildPackageLock synthesises an npm package-lock-v3 shape with the
// supplied package map. The shape mirrors what `engine/sbom/npm`
// expects: lockfileVersion=3, packages keyed by node_modules path,
// every entry carrying `version` + a fake `integrity` hash.
func buildPackageLock(name string, packages map[string]string) map[string]any {
	pkgEntries := map[string]any{
		"": map[string]any{
			"name":    name,
			"version": "1.0.0",
		},
	}
	for pkg, version := range packages {
		pkgEntries["node_modules/"+pkg] = map[string]any{
			"version":   version,
			"resolved":  fmt.Sprintf("https://registry.npmjs.org/%s/-/%s-%s.tgz", pkg, pkg, version),
			"integrity": "sha512-loadtestfakehash",
		}
	}
	return map[string]any{
		"name":            name,
		"version":         "1.0.0",
		"lockfileVersion": 3,
		"requires":        true,
		"packages":        pkgEntries,
	}
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}
