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

// SQLTableFaultTest verifies errors and rollback after dependent queries fail.
func SQLTableFaultTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB)) {
	t.Helper()
	testCases := []struct{ operation, table string }{
		{"create", "attributes"},
		{"create", "pending_events"},
		{"poll", "attributes"},
		{"poll", "history"},
		{"retain", "history"},
		{"retain", "attributes"},
		{"stats", "pending_events"},
		{"stats", "activities"},
	}
	for _, tc := range testCases {
		t.Run(tc.operation+" without "+tc.table, func(t *testing.T) {
			b, db := factory(t)
			require.NoError(t, b.(interface{ Migrate() error }).Migrate())
			ctx := context.Background()
			instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
			event := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			queues := []workflow.Queue{workflow.QueueDefault}
			if tc.operation != "create" {
				require.NoError(t, b.CreateWorkflowInstance(ctx, instance, event))
			}
			if tc.operation == "retain" {
				task, err := b.GetWorkflowTask(ctx, queues)
				require.NoError(t, err)
				require.NotNil(t, task)
				require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
			}
			_, err := db.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable") // #nosec G202 -- Fixed table names from the cases above in a private database.
			require.NoError(t, err)
			switch tc.operation {
			case "create":
				err = b.CreateWorkflowInstance(ctx, instance, event)
			case "poll":
				_, err = b.GetWorkflowTask(ctx, queues)
			case "retain":
				err = b.RemoveWorkflowInstances(ctx, backend.RemoveFinishedBefore(time.Now().Add(time.Hour)))
			case "stats":
				_, err = b.GetStats(ctx)
			}
			require.Error(t, err)
			_, err = db.ExecContext(ctx, "ALTER TABLE unavailable RENAME TO "+tc.table) // #nosec G202 -- Restores the same fixed test table.
			require.NoError(t, err)
			switch tc.operation {
			case "create":
				_, err = b.GetWorkflowInstanceState(ctx, instance)
				require.ErrorIs(t, err, backend.ErrInstanceNotFound)
				require.NoError(t, b.CreateWorkflowInstance(ctx, instance, event))
			case "poll":
				task, err := b.GetWorkflowTask(ctx, queues)
				require.NoError(t, err)
				require.NotNil(t, task)
				require.Equal(t, instance.InstanceID, task.WorkflowInstance.InstanceID)
			case "retain":
				state, err := b.GetWorkflowInstanceState(ctx, instance)
				require.NoError(t, err)
				require.Equal(t, core.WorkflowInstanceStateFinished, state)
				events, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
				require.NoError(t, err)
				require.Len(t, events, 1)
				require.Equal(t, event.ID, events[0].ID)
			case "stats":
				stats, err := b.GetStats(ctx)
				require.NoError(t, err)
				require.EqualValues(t, 1, stats.ActiveWorkflowInstances)
			}
		})
	}
}
