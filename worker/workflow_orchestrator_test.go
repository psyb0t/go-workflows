package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestWorkflowOrchestrator(t *testing.T) {
	t.Parallel()
	for _, custom := range []bool{false, true} {
		name := "defaults"
		if custom {
			name = "explicit pollers"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := sqlite.NewInMemoryBackend(sqlite.WithBackendOptions(backend.WithStickyTimeout(0)))
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			var options *worker.Options
			if custom {
				o := worker.DefaultOptions
				o.WorkflowPollers, o.ActivityPollers = 3, 3
				o.WorkflowPollingInterval, o.ActivityPollingInterval = time.Millisecond, time.Millisecond
				options = &o
			}
			o := worker.NewWorkflowOrchestrator(b, options)
			a := func(ctx context.Context, value int) (int, error) { return value * 2, nil }
			wf := func(ctx workflow.Context) (int, error) {
				value, ok := workflow.NewSignalChannel[int](ctx, "value").Receive(ctx)
				if !ok {
					return 0, workflow.Canceled
				}
				return workflow.ExecuteActivity[int](ctx, workflow.ActivityOptions{}, a, value).Get(ctx)
			}
			require.NoError(t, o.RegisterWorkflow(wf))
			require.NoError(t, o.RegisterActivity(a))
			require.Error(t, o.RegisterWorkflow(wf))
			require.Error(t, o.RegisterActivity(a))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			require.NoError(t, o.Start(ctx))
			t.Cleanup(func() { cancel(); require.NoError(t, o.WaitForCompletion()) })
			instance, err := o.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{InstanceID: uuid.NewString()}, wf)
			require.NoError(t, err)
			require.NoError(t, o.SignalWorkflow(ctx, instance.InstanceID, "value", 21))
			require.NoError(t, o.WaitForWorkflowInstance(ctx, instance, 5*time.Second))
			result, err := client.GetWorkflowResult[int](ctx, o.Client, instance, 5*time.Second)
			require.NoError(t, err)
			require.Equal(t, 42, result)
			require.NoError(t, o.RemoveWorkflowInstance(ctx, instance))
			_, err = b.GetWorkflowInstanceState(ctx, instance)
			require.ErrorIs(t, err, backend.ErrInstanceNotFound)
		})
	}
}
