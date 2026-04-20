package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheck_ValidV3(t *testing.T) {
	p := writeTemp(t, "package-lock.json", `{"name":"x","version":"1","lockfileVersion":3,"packages":{}}`)
	if err := check(p); err != nil {
		t.Fatalf("valid v3 rejected: %v", err)
	}
}

func TestCheck_WrongVersion(t *testing.T) {
	p := writeTemp(t, "package-lock.json", `{"lockfileVersion":2}`)
	if err := check(p); err == nil {
		t.Fatal("v2 must be rejected")
	}
}

func TestCheck_MalformedJSON(t *testing.T) {
	p := writeTemp(t, "package-lock.json", `{ broken`)
	if err := check(p); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}

func TestCheck_MissingFile(t *testing.T) {
	if err := check("/definitely/does/not/exist.json"); err == nil {
		t.Fatal("missing file must be rejected")
	}
}

func TestCheck_MissingVersionField(t *testing.T) {
	p := writeTemp(t, "package-lock.json", `{"name":"x"}`)
	// No lockfileVersion → defaults to 0 → must fail ("expected 3").
	if err := check(p); err == nil {
		t.Fatal("absent lockfileVersion must be rejected")
	}
}
