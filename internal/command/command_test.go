package command

import (
	"testing"

	"github.com/psyb0t/ctxerrors/commerr"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/history"
)

func assertInvalidStatePanic(t *testing.T, message string, operation func()) {
	t.Helper()
	defer func() {
		value := recover()
		err, ok := value.(error)
		require.True(t, ok, "expected an error panic, got %v", value)
		require.ErrorIs(t, err, commerr.ErrInvalidState)
		require.ErrorContains(t, err, message)
	}()
	operation()
}

func assertExecuteNoEvent(t *testing.T, c Command, expectedState CommandState) {
	r := c.Execute(clock.New())

	require.Nil(t, r)
	require.Equal(t, expectedState, c.State())
}

func assertExecuteWithEvent(t *testing.T, c Command, expectedState CommandState, expectedEventType history.EventType) *CommandResult {
	r := c.Execute(clock.New())

	require.NotNil(t, r)
	require.Equal(t, expectedState, c.State())
	require.Len(t, r.Events, 1)
	require.Equal(t, expectedEventType, r.Events[0].Type)

	return r
}
