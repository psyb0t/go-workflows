package workflow_test

import (
	"context"
	"testing"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/tester"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestWorkflowSynchronization(t *testing.T) {
	t.Parallel()
	definition := func(ctx workflow.Context) (int, error) {
		wg := workflow.NewWaitGroup()
		total := 0
		for i := range 4 {
			wg.Add(1)
			workflow.Go(ctx, func(workflow.Context) { total += i; wg.Done() })
		}
		wg.Wait(ctx)
		groupCtx, group := workflow.WithErrGroup(ctx)
		group.Go(func(workflow.Context) error { total *= 7; return nil })
		if err := group.Wait(groupCtx); err != nil {
			return 0, err
		}
		return total, nil
	}
	w := tester.NewWorkflowTester[int](definition)
	w.Execute(context.Background())
	result, err := w.WorkflowResult()
	require.NoError(t, err)
	require.Equal(t, 42, result)

	failure := func(ctx workflow.Context) error {
		_, group := workflow.WithErrGroup(ctx)
		group.Go(func(workflow.Context) error { return commerr.ErrInvalidState })
		return group.Wait(ctx)
	}
	f := tester.NewWorkflowTester[any](failure)
	f.Execute(context.Background())
	_, err = f.WorkflowResult()
	require.Error(t, err)
	require.ErrorContains(t, err, commerr.ErrInvalidState.Error())
}
