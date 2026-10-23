package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/tester"
)

func TestRegisteredActivityReceiver(t *testing.T) {
	t.Parallel()
	runner := tester.NewWorkflowTester[any](Workflow1)
	// Use real receiver methods so argument conversion and method lookup are tested.
	require.NoError(t, runner.Registry().RegisterActivity(&activities{SomeParam: "example state"}))
	runner.Execute(context.Background(), "example input")
	require.True(t, runner.WorkflowFinished())
	_, err := runner.WorkflowResult()
	require.NoError(t, err)
}
