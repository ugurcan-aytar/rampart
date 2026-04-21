// Command gen-lockfile-fixture generates synthetic npm package-lock.json
// (lockfileVersion 3) fixtures at a caller-requested size.
//
// Intended to produce the large benchmark fixtures that the parity test
// does not ship (they'd balloon the git tree past 100 MiB — see
// .gitignore). Outputs are deterministic for a given -packages / -seed
// combination so benchmark re-runs compare apples to apples.
//
// Usage:
//
//	go run ./engine/cmd/gen-lockfile-fixture \
//	    -packages 10000 \
//	    -output engine/testdata/lockfiles/large-20k-pkgs.json
//
// Realism trade-offs (and why they don't matter for benchmarking):
//   - Names are `pkg-000001` to `pkg-NNNNNN`, flat — no scoped names,
//     no workspaces, no transitive chains. The parser's cost is
//     dominated by JSON decode + per-entry allocation; the shape of
//     the tree doesn't move those numbers meaningfully.
//   - Scope mix follows `~20% dev, ~5% peer, ~3% optional` to exercise
//     `buildScope` in both parsers.
//   - Every entry has an `integrity` string so the `Integrity: ""`
//     warning branch isn't what the benchmark measures.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
)

type lockPackage struct {
	Version   string `json:"version"`
	Integrity string `json:"integrity,omitempty"`
	Dev       bool   `json:"dev,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
	Peer      bool   `json:"peer,omitempty"`
}

type lockfile struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	LockfileVersion int                    `json:"lockfileVersion"`
	Packages        map[string]lockPackage `json:"packages"`
}

func main() {
	packages := flag.Int("packages", 1000, "number of third-party packages to generate")
	output := flag.String("output", "", "output path (required)")
	seed := flag.Int64("seed", 42, "PRNG seed — same seed = same fixture bytes")
	flag.Parse()

	if *output == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "error: -output is required")
		os.Exit(2)
	}
	if *packages < 1 {
		fmt.Fprintln(os.Stderr, "error: -packages must be >= 1")
		os.Exit(2)
	}

	// Deterministic synthetic fixture generator — identical inputs
	// must produce identical bytes so benchmark re-runs compare
	// apples to apples. Crypto randomness would defeat that.
	rng := rand.New(rand.NewSource(*seed)) //nolint:gosec // G404: intentional, fixtures require reproducibility

	lf := lockfile{
		Name:            "synthetic",
		Version:         "1.0.0",
		LockfileVersion: 3,
		Packages:        make(map[string]lockPackage, *packages+1),
	}
	lf.Packages[""] = lockPackage{Version: "1.0.0"}

	for i := 1; i <= *packages; i++ {
		name := fmt.Sprintf("pkg-%06d", i)
		path := "node_modules/" + name
		p := lockPackage{
			Version:   randomVersion(rng),
			Integrity: "sha512-" + randomHex(rng, 86) + "==",
		}
		// Scope mix — roll each bit independently so combined scopes
		// (e.g. dev+peer) occur naturally. Rates: 20 % dev, 5 % peer,
		// 3 % optional. See buildScope in the parser for semantics.
		if rng.Float64() < 0.20 {
			p.Dev = true
		}
		if rng.Float64() < 0.05 {
			p.Peer = true
		}
		if rng.Float64() < 0.03 {
			p.Optional = true
		}
		lf.Packages[path] = p
	}

	f, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: create output:", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(&lf); err != nil {
		fmt.Fprintln(os.Stderr, "error: encode lockfile:", err)
		os.Exit(1)
	}

	info, err := f.Stat()
	if err == nil {
		fmt.Fprintf(os.Stderr, "wrote %d packages to %s (%d bytes)\n",
			*packages, *output, info.Size())
	}
}

func randomVersion(r *rand.Rand) string {
	return fmt.Sprintf("%d.%d.%d", r.Intn(20), r.Intn(50), r.Intn(200))
}

func randomHex(r *rand.Rand, n int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hex[r.Intn(16)]
	}
	return string(b)
}
