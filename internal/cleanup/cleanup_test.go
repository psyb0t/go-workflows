package cleanup

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingCloser struct{ called bool }

func (c *failingCloser) Close() error {
	c.called = true
	return errors.New("close failed")
}

func TestCloseReportsCleanupFailure(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	closer := &failingCloser{}
	Close(context.Background(), closer)
	require.True(t, closer.called)
	require.Contains(t, output.String(), `"level":"WARN"`)
	require.Contains(t, output.String(), `"err":`)
}
