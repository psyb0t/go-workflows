package db

import (
	"testing"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
)

func TestOpenRejectsMissingPool(t *testing.T) {
	t.Parallel()
	database, err := Open(nil)
	require.Nil(t, database)
	require.ErrorIs(t, err, commerr.ErrRequiredFieldNotSet)
	store, err := New(nil, "worker", backend.ApplyOptions())
	require.Nil(t, store)
	require.ErrorIs(t, err, commerr.ErrRequiredFieldNotSet)
}
