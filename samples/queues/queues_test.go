package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
)

func TestQueueSampleRouting(t *testing.T) {
	t.Parallel()
	b := sqlite.NewInMemoryBackend()
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	w, activityWorker, workflowWorker := startWorkers(ctx, b)
	t.Cleanup(func() {
		cancel()
		require.NoError(t, w.WaitForCompletion())
		require.NoError(t, activityWorker.WaitForCompletion())
		require.NoError(t, workflowWorker.WaitForCompletion())
	})
	c := client.New(b)
	instance, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "example input", 42, Inputs{})
	require.NoError(t, err)
	result, err := client.GetWorkflowResult[int](ctx, c, instance, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, 59, result)
}
