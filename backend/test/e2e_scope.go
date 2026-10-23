package test

import (
	"context"
	"testing"

	"github.com/psyb0t/ctxscope"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

var e2eScopeTests = []backendTest{
	{
		name: "Scope/ChildActivity",
		f: func(t *testing.T, ctx context.Context, c *client.Client, w *worker.Worker, _ TestBackend) {
			readScope := func(ctx context.Context) (map[string]any, error) {
				return ctxscope.Get(ctx), nil
			}
			child := func(ctx workflow.Context) (map[string]any, error) {
				return workflow.ExecuteActivity[map[string]any](ctx, workflow.DefaultActivityOptions, readScope).Get(ctx)
			}
			parent := func(ctx workflow.Context) (map[string]any, error) {
				return workflow.CreateSubWorkflowInstance[map[string]any](ctx, workflow.DefaultSubWorkflowOptions, child).Get(ctx)
			}
			register(t, ctx, w, []any{parent, child}, []any{readScope})
			ctx = ctxscope.Set(ctx, ctxscope.Attr("request_id", "scope-integration"))
			result, err := runWorkflowWithResult[map[string]any](t, ctx, c, parent)
			require.NoError(t, err)
			require.Equal(t, "scope-integration", result["request_id"])
			for _, key := range []string{"workflow_id", "workflow_run_id", "activity_id", "activity_type", "task_queue"} {
				require.NotEmpty(t, result[key], key)
			}
			require.Contains(t, result, "attempt")
		},
	},
}
