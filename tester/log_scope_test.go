package tester

import (
	"context"
	"testing"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxscope"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/workflow"
)

func TestLogScopeReachesSubworkflowActivity(t *testing.T) {
	t.Parallel()
	readScope := func(ctx context.Context) (string, error) {
		encoded, err := ctxscope.ToJSON(ctx)
		if err != nil {
			return "", ctxerrors.Wrap(err, "read activity scope")
		}
		return string(encoded), nil
	}
	child := func(ctx workflow.Context) (string, error) {
		return workflow.ExecuteActivity[string](ctx, workflow.DefaultActivityOptions, readScope).Get(ctx)
	}
	parent := func(ctx workflow.Context) (string, error) {
		return workflow.CreateSubWorkflowInstance[string](ctx, workflow.DefaultSubWorkflowOptions, child).Get(ctx)
	}
	engine := NewWorkflowTester[string](parent)
	require.NoError(t, engine.Registry().RegisterWorkflow(child))
	require.NoError(t, engine.Registry().RegisterActivity(readScope))
	ctx := ctxscope.Set(context.Background(), ctxscope.Attr("request_id", "scope-test"))
	engine.Execute(ctx)
	require.True(t, engine.WorkflowFinished())
	result, err := engine.WorkflowResult()
	require.NoError(t, err)
	require.Contains(t, result, `"request_id":"scope-test"`)
	require.Contains(t, result, `"workflow_id"`)
	require.Contains(t, result, `"activity_id"`)
}
