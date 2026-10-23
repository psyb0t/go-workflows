package workflowerrors

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/psyb0t/ctxerrors"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/internal/sync"
)

func TestPersistedCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "direct", err: sync.Canceled},
		{name: "wrapped", err: ctxerrors.Wrap(sync.Canceled, "operation")},
		{name: "joined", err: errors.Join(errors.New("first"), ctxerrors.Wrap(sync.Canceled, "operation"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, FromError(tc.err), sync.Canceled)
			data, err := json.Marshal(FromError(tc.err))
			require.NoError(t, err)
			var restored Error
			require.NoError(t, json.Unmarshal(data, &restored))
			decoded := ToError(&restored)
			require.ErrorIs(t, decoded, sync.Canceled)
			require.False(t, CanRetry(decoded))
		})
	}
}

func TestPersistedPermanentPanic(t *testing.T) {
	t.Parallel()
	input := NewPermanentError(NewPanicError("failed"))
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var restored Error
	require.NoError(t, json.Unmarshal(data, &restored))
	decoded := ToError(&restored)
	var panicError *PanicError
	require.ErrorAs(t, decoded, &panicError)
	require.False(t, CanRetry(decoded))
	require.NotEmpty(t, panicError.Stack())
}
