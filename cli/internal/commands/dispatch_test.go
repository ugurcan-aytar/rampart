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

func TestDispatch_IngestIsStub(t *testing.T) {
	var out, errOut bytes.Buffer
	err := commands.Dispatch(context.Background(), []string{"ingest"}, &out, &errOut)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not yet implemented")
}
