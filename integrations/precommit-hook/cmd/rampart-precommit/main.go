// Copyright (c) 2026 Uğurcan Aytar. MIT License.
//
// rampart-precommit runs over staged package-lock.json files before a
// commit lands. Exits non-zero on malformed JSON or on lockfileVersion
// other than 3 — so a developer doesn't commit a lockfile the engine's
// parser will refuse.
//
// This binary is deliberately tiny (stdlib only, zero external deps).
// Full IoC-matching scans belong in the `rampart scan` CLI or CI —
// pre-commit is the wrong layer for network calls.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type minimalLockfile struct {
	LockfileVersion int `json:"lockfileVersion"`
}

func main() {
	if len(os.Args) < 2 {
		// Nothing staged — succeed silently.
		return
	}
	fail := 0
	for _, path := range os.Args[1:] {
		if err := check(path); err != nil {
			fmt.Fprintf(os.Stderr, "rampart-precommit: %s: %v\n", path, err)
			fail++
		}
	}
	if fail > 0 {
		os.Exit(1)
	}
}

func check(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lf minimalLockfile
	if err := json.Unmarshal(body, &lf); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	if lf.LockfileVersion != 3 {
		return fmt.Errorf("lockfileVersion is %d, expected 3", lf.LockfileVersion)
	}
	return nil
}
