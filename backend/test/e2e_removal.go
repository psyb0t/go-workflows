package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

const (
	autoExpirationWaitTimeout  = 10 * time.Second
	autoExpirationPollInterval = 20 * time.Millisecond
)

var e2eRemovalTests = []backendTest{
	{
		name: "RemoveWorkflowInstances/Removes",
		f: func(t *testing.T, ctx context.Context, c *client.Client, w *worker.Worker, b TestBackend) {
			wf := func(ctx workflow.Context) (bool, error) {
				return true, nil
			}

			register(t, ctx, w, []interface{}{wf}, nil)

			workflowA := runWorkflow(t, ctx, c, wf)
			_, err := client.GetWorkflowResult[bool](ctx, c, workflowA, time.Second*10)
			require.NoError(t, err)

			for i := 0; i < 10; i++ {
				time.Sleep(300 * time.Millisecond)

				err = b.RemoveWorkflowInstances(ctx, backend.RemoveFinishedBefore(time.Now()))
				if errors.As(err, &backend.ErrNotSupported{}) {
					t.Skip()
					return
				}

				require.NoError(t, err)

				_, err = c.GetWorkflowInstanceState(ctx, workflowA)
				if errors.Is(err, backend.ErrInstanceNotFound) {
					break
				}
			}

			require.ErrorIs(t, err, backend.ErrInstanceNotFound)
		},
	},
	{
		name: "AutoExpiration/StartsWorkflowAndRemoves",
		f: func(t *testing.T, ctx context.Context, c *client.Client, w *worker.Worker, b TestBackend) {
			wf := func(ctx workflow.Context) (bool, error) {
				return true, nil
			}

			register(t, ctx, w, []interface{}{wf}, nil)

			err := c.StartAutoExpiration(ctx, time.Millisecond)
			if errors.As(err, &backend.ErrNotSupported{}) {
				t.Skip()
				return
			}

			require.NoError(t, err)

			workflowA := runWorkflow(t, ctx, c, wf)
			_, err = client.GetWorkflowResult[bool](ctx, c, workflowA, time.Second*10)
			require.NoError(t, err)

			// Expiration crosses workflow, timer and activity pollers. Await the
			// persisted deletion instead of assuming they finish within one second.
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				_, err := c.GetWorkflowInstanceState(ctx, workflowA)
				assert.ErrorIs(collect, err, backend.ErrInstanceNotFound)
			}, autoExpirationWaitTimeout, autoExpirationPollInterval)
		},
	},
	{
		name: "AutoExpiration/UpdateDelay",
		f: func(t *testing.T, ctx context.Context, c *client.Client, w *worker.Worker, b TestBackend) {
			wf := func(ctx workflow.Context) (bool, error) {
				return true, nil
			}

			register(t, ctx, w, []interface{}{wf}, nil)

			err := c.StartAutoExpiration(ctx, time.Hour)
			if errors.As(err, &backend.ErrNotSupported{}) {
				t.Skip()
				return
			}

			require.NoError(t, err)

			workflowA := runWorkflow(t, ctx, c, wf)
			_, err = client.GetWorkflowResult[bool](ctx, c, workflowA, time.Second*10)
			require.NoError(t, err)

			time.Sleep(1000 * time.Millisecond)
			_, err = c.GetWorkflowInstanceState(ctx, workflowA)
			require.NoError(t, err)

			// update delay
			err = c.StartAutoExpiration(ctx, time.Millisecond)
			require.NoError(t, err)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				_, err := c.GetWorkflowInstanceState(ctx, workflowA)
				assert.ErrorIs(collect, err, backend.ErrInstanceNotFound)
			}, autoExpirationWaitTimeout, autoExpirationPollInterval)
		},
	},
}
