package tester

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/workflow"
)

func Test_SubWorkflow(t *testing.T) {
	subWorkflow := func(ctx workflow.Context, input string) (string, error) {
		return input + "sresult", nil
	}

	workflowWithSub := func(ctx workflow.Context, input string) (string, error) {
		sresult, err := workflow.CreateSubWorkflowInstance[string](
			ctx,
			workflow.DefaultSubWorkflowOptions,
			subWorkflow,
			input,
		).Get(ctx)
		if err != nil {
			return "", err
		}

		return sresult, nil
	}

	tester := NewWorkflowTester[string](workflowWithSub)
	if err := tester.Registry().RegisterWorkflow(subWorkflow); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}

	tester.Execute(context.Background(), "hello")

	require.True(t, tester.WorkflowFinished())

	wfR, _ := tester.WorkflowResult()
	require.Equal(t, "hellosresult", wfR)
	tester.AssertExpectations(t)
}

func Test_SubWorkflow_Mocked(t *testing.T) {
	subWorkflow := func(ctx workflow.Context, input string) (string, error) {
		panic("should not call this")
	}

	workflow := func(ctx workflow.Context, input string) (string, error) {
		sresult, err := workflow.CreateSubWorkflowInstance[string](
			ctx,
			workflow.DefaultSubWorkflowOptions,
			subWorkflow,
			input,
		).Get(ctx)
		if err != nil {
			return "", err
		}

		return sresult, nil
	}

	tester := NewWorkflowTester[string](workflow)
	if err := tester.Registry().RegisterWorkflow(subWorkflow); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}
	tester.OnSubWorkflow(subWorkflow, mock.Anything, mock.Anything).Return("sresult2", nil)

	tester.Execute(context.Background(), "hello")

	require.True(t, tester.WorkflowFinished())

	wfR, _ := tester.WorkflowResult()
	require.Equal(t, "sresult2", wfR)
	tester.AssertExpectations(t)
}

func Test_SubWorkflow_Mocked_Failure(t *testing.T) {
	subWorkflow := func(ctx workflow.Context, input string) (string, error) {
		panic("should not call this")
	}

	workflow := func(ctx workflow.Context, input string) (string, error) {
		sresult, err := workflow.CreateSubWorkflowInstance[string](
			ctx,
			workflow.DefaultSubWorkflowOptions,
			subWorkflow,
			input,
		).Get(ctx)
		if err != nil {
			return "", err
		}

		return sresult, nil
	}

	tester := NewWorkflowTester[string](workflow)
	if err := tester.Registry().RegisterWorkflow(subWorkflow); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}
	tester.OnSubWorkflow(subWorkflow, mock.Anything, mock.Anything).Return(nil, errors.New("swf error"))

	tester.Execute(context.Background(), "hello")

	require.True(t, tester.WorkflowFinished())

	wfR, wfE := tester.WorkflowResult()
	require.Empty(t, wfR)
	require.ErrorContains(t, wfE, "swf error")

	tester.AssertExpectations(t)
}

func Test_SubWorkflow_Cancel(t *testing.T) {
	subWorkflow := func(ctx workflow.Context) error {
		_, _ = ctx.Done().Receive(ctx)
		return ctx.Err()
	}

	workflowWithSub := func(ctx workflow.Context) error {
		_, err := workflow.CreateSubWorkflowInstance[any](
			ctx,
			workflow.DefaultSubWorkflowOptions,
			subWorkflow,
		).Get(ctx)
		if err != nil {
			return fmt.Errorf("subworkflow: %w", err)
		}

		return nil
	}

	tester := NewWorkflowTester[string](workflowWithSub)
	if err := tester.Registry().RegisterWorkflow(subWorkflow); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}

	var subWorkflowInstance *core.WorkflowInstance

	tester.ListenSubWorkflow(func(instance *core.WorkflowInstance, _ string) {
		subWorkflowInstance = instance
	})

	tester.ScheduleCallback(time.Millisecond, func() {
		require.NoError(t, tester.CancelWorkflowInstance(subWorkflowInstance))
	})

	tester.Execute(context.Background())

	require.True(t, tester.WorkflowFinished())

	_, err := tester.WorkflowResult()
	require.ErrorContains(t, err, "subworkflow: context canceled")
	tester.AssertExpectations(t)
}

func Test_SubWorkflow_Signals(t *testing.T) {
	subWorkflow := func(ctx workflow.Context, input string) (string, error) {
		c := workflow.NewSignalChannel[string](ctx, "subworkflow-signal")
		r, _ := c.Receive(ctx)

		return input + r, nil
	}

	workflowWithSub := func(ctx workflow.Context, input string) (string, error) {
		sresult, err := workflow.CreateSubWorkflowInstance[string](
			ctx,
			workflow.DefaultSubWorkflowOptions,
			subWorkflow,
			input,
		).Get(ctx)
		if err != nil {
			return "", err
		}

		return sresult, nil
	}

	tester := NewWorkflowTester[string](workflowWithSub)
	if err := tester.Registry().RegisterWorkflow(subWorkflow); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}

	var subWorkflowInstance *core.WorkflowInstance

	tester.ListenSubWorkflow(func(instance *core.WorkflowInstance, name string) {
		subWorkflowInstance = instance
	})

	tester.ScheduleCallback(time.Millisecond, func() {
		require.NoError(t, tester.SignalWorkflowInstance(subWorkflowInstance, "subworkflow-signal", "42"))
	})

	tester.Execute(context.Background(), "hello")

	require.True(t, tester.WorkflowFinished())

	wfR, _ := tester.WorkflowResult()
	require.Equal(t, "hello42", wfR)
	tester.AssertExpectations(t)
}
