package workflow

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/internal/sync"
)

func TestCause(t *testing.T) {
	myErr := errors.New("my cause")

	ctx, cancel := WithCancelCause(sync.Background())
	require.NoError(t, Cause(ctx))

	cancel(myErr)

	require.ErrorIs(t, ctx.Err(), Canceled)
	require.Equal(t, myErr, Cause(ctx))
}
