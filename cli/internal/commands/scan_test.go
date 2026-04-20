package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/cli/internal/commands"
)

const axiosFixture = "../../../engine/testdata/lockfiles/axios-compromise.json"

func TestScan_TextOutput(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{
		"--format", "text",
		"--component-ref", "kind:Component/default/web",
		"--commit-sha", "abc123",
		axiosFixture,
	}, &out)
	require.NoError(t, err)

	got := out.String()
	require.Contains(t, got, "Ecosystem:      npm")
	require.Contains(t, got, "Source format:  npm-package-lock-v3")
	require.Contains(t, got, "Component ref:  kind:Component/default/web")
	require.Contains(t, got, "Commit SHA:     abc123")
	require.Contains(t, got, "axios@1.11.0")
	require.Contains(t, got, "plain-crypto-js@4.2.1")
}

func TestScan_JSONOutput(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{"--format", "json", axiosFixture}, &out)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &parsed))
	require.Equal(t, "npm", parsed["Ecosystem"])
	require.Equal(t, "npm-package-lock-v3", parsed["SourceFormat"])
	pkgs, ok := parsed["Packages"].([]any)
	require.True(t, ok)
	require.Len(t, pkgs, 2)
}

func TestScan_SARIFOutput(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{"--format", "sarif", axiosFixture}, &out)
	require.NoError(t, err)

	var sarif map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &sarif))
	require.Equal(t, "2.1.0", sarif["version"])
	runs, ok := sarif["runs"].([]any)
	require.True(t, ok)
	require.Len(t, runs, 1)
	run0 := runs[0].(map[string]any)
	driver := run0["tool"].(map[string]any)["driver"].(map[string]any)
	require.Equal(t, "rampart", driver["name"])
	require.Equal(t, "0.1.0", driver["version"])

	props := run0["properties"].(map[string]any)
	require.Equal(t, "npm", props["scanned_ecosystem"])
	// JSON numbers deserialize as float64
	require.EqualValues(t, 2, props["scanned_package_count"])
}

func TestScan_DefaultFormatIsText(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{axiosFixture}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "Ecosystem:      npm")
}

func TestScan_UnknownFormat(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{"--format", "csv", axiosFixture}, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown format")
}

func TestScan_MissingFile(t *testing.T) {
	var out bytes.Buffer
	err := commands.Scan(context.Background(), []string{"/definitely/does/not/exist.json"}, &out)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "open"))
}

func TestScan_MissingPath(t *testing.T) {
	err := commands.Scan(context.Background(), []string{}, &bytes.Buffer{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing lockfile path")
}
