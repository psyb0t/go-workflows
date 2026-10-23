package sync

import (
	"fmt"
	"testing"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"
)

func TestContextBoundaries(t *testing.T) {
	t.Parallel()
	ctx := Background()
	require.Nil(t, ctx.Done())
	require.NoError(t, ctx.Err())
	require.Nil(t, ctx.Value(ctxKey(1)))
	require.NoError(t, Cause(ctx))
	require.Equal(t, "context.Background", fmt.Sprint(ctx))
	require.Panics(t, func() { WithValue(nil, ctxKey(1), 1) })
	require.Panics(t, func() { WithValue(ctx, nil, 1) })
	require.Panics(t, func() { WithValue(ctx, []int{1}, 1) })
	require.Panics(t, func() { WithCancel(nil) })
	ctx = WithValue(ctx, ctxKey(1), "parent")
	ctx = WithValue(ctx, ctxKey(2), "child")
	require.Equal(t, "parent", ctx.Value(ctxKey(1)))
	require.Equal(t, "child", ctx.Value(ctxKey(2)))

	coroutine := NewCoroutine(ctx, func(ctx Context) error {
		parent, cancelParent := WithCancelCause(ctx)
		child, cancelChild := WithCancelCause(parent)
		cancelChild(commerr.ErrInvalidState)
		cancelParent(commerr.ErrNotFound)
		require.ErrorIs(t, Cause(child), commerr.ErrInvalidState)
		require.ErrorIs(t, Cause(parent), commerr.ErrNotFound)
		require.ErrorIs(t, child.Err(), Canceled)
		cancelChild(nil)
		require.ErrorIs(t, Cause(child), commerr.ErrInvalidState)
		late, cancelLate := WithCancel(parent)
		cancelLate()
		require.ErrorIs(t, late.Err(), Canceled)
		require.ErrorIs(t, Cause(late), commerr.ErrNotFound)
		disconnected := NewDisconnectedContext(parent)
		require.NoError(t, disconnected.Err())
		require.Equal(t, "parent", disconnected.Value(ctxKey(1)))
		_, received := disconnected.Done().ReceiveNonBlocking()
		require.False(t, received)
		return nil
	})
	coroutine.Execute()
	require.True(t, coroutine.Finished())
	require.NoError(t, coroutine.Error())
}
