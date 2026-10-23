package main

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_WORKFLOWS_SCALE_CLI_PROBE") == "1" {
		os.Args = []string{os.Args[0], "-backend", "memory"}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestScaleWorkerBackendFlag(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, executable) // #nosec G204 -- Reexecutes this test binary with fixed inputs.
	command.Env = append(os.Environ(), "GO_WORKFLOWS_SCALE_CLI_PROBE=1")
	output, err := command.CombinedOutput()
	require.Error(t, err, "worker should remain alive until interrupted")
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded, "%s", output)
	require.NotContains(t, string(output), "flag provided but not defined")
}
