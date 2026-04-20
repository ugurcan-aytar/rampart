package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/app"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// captureStdout swaps os.Stdout with a pipe, runs fn, and returns whatever fn wrote.
func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()
	orig := os.Stdout
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(copyDone)
	}()

	runErr := fn()
	os.Stdout = orig
	_ = w.Close()
	<-copyDone
	_ = r.Close()
	return buf.Bytes(), runErr
}

func TestMain_ParseSBOMSubcommand_Axios(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return app.Main(context.Background(), []string{"parse-sbom", "../../testdata/lockfiles/axios-compromise.json"})
	})
	require.NoError(t, err)

	var sbom domain.SBOM
	require.NoError(t, json.Unmarshal(out, &sbom))
	require.Equal(t, "npm", sbom.Ecosystem)
	require.Equal(t, "npm-package-lock-v3", sbom.SourceFormat)

	byName := map[string]domain.PackageVersion{}
	for _, p := range sbom.Packages {
		byName[p.Name] = p
	}
	require.Contains(t, byName, "axios")
	require.Equal(t, "1.11.0", byName["axios"].Version)
	require.Contains(t, byName, "plain-crypto-js")
	require.Equal(t, "4.2.1", byName["plain-crypto-js"].Version)
}

func TestMain_ParseSBOM_WithFlags(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return app.Main(context.Background(), []string{
			"parse-sbom",
			"--component-ref", "component:default/web-app",
			"--commit-sha", "abc123deadbeef",
			"../../testdata/lockfiles/axios-compromise.json",
		})
	})
	require.NoError(t, err)

	var sbom domain.SBOM
	require.NoError(t, json.Unmarshal(out, &sbom))
	require.Equal(t, "component:default/web-app", sbom.ComponentRef, "flag value must land on SBOM")
	require.Equal(t, "abc123deadbeef", sbom.CommitSHA)
}

func TestMain_ParseSBOM_UnknownFlag(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom", "--nope", "x", "some.json"})
	require.Error(t, err)
}

func TestMain_ParseSBOM_MissingArg(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom"})
	require.Error(t, err)
}

func TestMain_ParseSBOM_MissingFile(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom", "/definitely/does/not/exist.json"})
	require.Error(t, err)
}
