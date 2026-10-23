package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuickstart(t *testing.T) {
	t.Parallel()
	result, err := run(context.Background(), filepath.Join(t.TempDir(), "workflow.sqlite"))
	require.NoError(t, err)
	require.Equal(t, 42, result)
}
