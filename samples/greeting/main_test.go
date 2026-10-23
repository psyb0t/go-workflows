package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGreeting(t *testing.T) {
	t.Parallel()
	result, err := run(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Hello, reader", result)
}
