package test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/workflow"
)

// SQLFailureTest verifies that unavailable storage is never reported as success.
// The factory supplies a real connection to an isolated database without tables.
func SQLFailureTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB)) {
	t.Helper()
	for _, closed := range []bool{false, true} {
		name := "missing schema"
		if closed {
			name = "closed connection"
		}
		t.Run(name, func(t *testing.T) {
			b, db := factory(t)
			ctx := context.Background()
			require.NoError(t, db.PingContext(ctx))
			if closed {
				require.NoError(t, db.Close())
			}
			instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
			event := history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			task := &backend.WorkflowTask{ID: instance.InstanceID, WorkflowInstance: instance, Queue: workflow.QueueDefault}
			activity := &backend.ActivityTask{ID: uuid.NewString(), ActivityID: uuid.NewString(), WorkflowInstance: instance, Event: event}
			queues := []workflow.Queue{workflow.QueueDefault}
			testCases := []struct {
				name string
				call func() error
			}{
				{"create", func() error { return b.CreateWorkflowInstance(ctx, instance, event) }},
				{"cancel", func() error {
					return b.CancelWorkflowInstance(ctx, instance, history.NewWorkflowCancellationEvent(time.Now()))
				}},
				{"remove", func() error { return b.RemoveWorkflowInstance(ctx, instance) }},
				{"remove expired", func() error { return b.RemoveWorkflowInstances(ctx) }},
				{"state", func() error { _, err := b.GetWorkflowInstanceState(ctx, instance); return err }},
				{"history", func() error { _, err := b.GetWorkflowInstanceHistory(ctx, instance, nil); return err }},
				{"history after cursor", func() error {
					cursor := int64(1)
					_, err := b.GetWorkflowInstanceHistory(ctx, instance, &cursor)
					return err
				}},
				{"signal", func() error { return b.SignalWorkflow(ctx, instance.InstanceID, event) }},
				{"poll workflow", func() error { _, err := b.GetWorkflowTask(ctx, queues); return err }},
				{"extend workflow", func() error { return b.ExtendWorkflowTask(ctx, task) }},
				{"complete workflow", func() error {
					return b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, nil, nil, nil, nil)
				}},
				{"poll activity", func() error { _, err := b.GetActivityTask(ctx, queues); return err }},
				{"extend activity", func() error { return b.ExtendActivityTask(ctx, activity) }},
				{"complete activity", func() error { return b.CompleteActivityTask(ctx, activity, event) }},
				{"stats", func() error { _, err := b.GetStats(ctx); return err }},
				{"diagnostic instance", func() error { _, err := b.GetWorkflowInstance(ctx, instance); return err }},
				{"diagnostic list", func() error { _, err := b.GetWorkflowInstances(ctx, "", "", 10); return err }},
				{"diagnostic page", func() error {
					_, err := b.GetWorkflowInstances(ctx, instance.InstanceID, instance.ExecutionID, 10)
					return err
				}},
				{"diagnostic tree", func() error { _, err := b.GetWorkflowTree(ctx, instance); return err }},
				{"future events", func() error { _, err := b.(TestBackend).GetFutureEvents(ctx); return err }},
			}
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					err := tc.call()
					require.Error(t, err)
					if closed {
						require.ErrorContains(t, err, "database is closed")
					}
				})
			}
		})
	}
}
