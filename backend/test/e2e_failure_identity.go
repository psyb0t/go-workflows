package test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/psyb0t/ctxerrors"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/internal/workflowerrors"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func failureIdentityTests() []backendTest {
	tests := make([]backendTest, 0, 2)
	for _, permanentPanic := range []bool{false, true} {
		name := "Failure/CancellationDoesNotRetry"
		if permanentPanic {
			name = "Failure/PermanentPanicDoesNotRetry"
		}
		tests = append(tests, backendTest{
			name: name,
			f: func(t *testing.T, ctx context.Context, c *client.Client, w *worker.Worker, _ TestBackend) {
				var attempts atomic.Int32
				activity := func(context.Context) error {
					attempts.Add(1)
					if permanentPanic {
						return workflow.NewPermanentError(workflowerrors.NewPanicError("permanent failure"))
					}
					return ctxerrors.Wrap(context.Canceled, "activity canceled")
				}
				definition := func(ctx workflow.Context) (bool, error) {
					_, err := workflow.ExecuteActivity[any](ctx, workflow.DefaultActivityOptions, activity).Get(ctx)
					if permanentPanic {
						var panicError *workflow.PanicError
						return errors.As(err, &panicError) && !workflow.CanRetry(err), nil
					}
					return errors.Is(err, workflow.Canceled) && !workflow.CanRetry(err), nil
				}
				register(t, ctx, w, []any{definition}, []any{activity})
				result, err := runWorkflowWithResult[bool](t, ctx, c, definition)
				require.NoError(t, err)
				require.True(t, result)
				require.Equal(t, int32(1), attempts.Load())
			},
		})
	}
	return tests
}
