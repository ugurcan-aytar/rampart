package commands_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/cli/internal/commands"
)

func TestDispatch_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	require.NoError(t, commands.Dispatch(context.Background(), []string{"help"}, &out, &errOut))
	require.Contains(t, out.String(), "usage: rampart")
	require.Contains(t, out.String(), "scan")
}

func TestDispatch_Unknown(t *testing.T) {
	var out, errOut bytes.Buffer
	err := commands.Dispatch(context.Background(), []string{"foo"}, &out, &errOut)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown subcommand")
}

func TestDispatch_Missing(t *testing.T) {
	var out, errOut bytes.Buffer
	err := commands.Dispatch(context.Background(), []string{}, &out, &errOut)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing subcommand")
}

func TestDispatch_RemovedStubsReturnUnknown(t *testing.T) {
	// ingest / status / serve were stub commands until the v0.2.x park-prep
	// cleanup removed them. They now hit the default branch and surface as
	// 'unknown subcommand', same as any unrecognised input. Pinning this
	// behaviour so a future re-introduction of any name is a deliberate
	// choice, not a regression.
	for _, name := range []string{"ingest", "status", "serve"} {
		var out, errOut bytes.Buffer
		err := commands.Dispatch(context.Background(), []string{name}, &out, &errOut)
		require.Error(t, err, "dispatch on removed stub %q must error", name)
		require.Contains(t, err.Error(), "unknown subcommand", "dispatch on %q", name)
	}
}
