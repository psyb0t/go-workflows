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

// SQLMutationTest checks lease identity and rollback across dependent writes.
func SQLMutationTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB)) {
	t.Helper()
	t.Run("lease identity and serialization", func(t *testing.T) {
		b, _ := factory(t)
		require.NoError(t, b.(interface{ Migrate() error }).Migrate())
		ctx := context.Background()
		instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
		started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
		require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
		queues := []workflow.Queue{workflow.QueueDefault}
		task, err := b.GetWorkflowTask(ctx, queues)
		require.NoError(t, err)
		require.NotNil(t, task)
		forged := *task
		forged.WorkflowInstance = core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
		require.Error(t, b.ExtendWorkflowTask(ctx, &forged))
		require.Error(t, b.CompleteWorkflowTask(ctx, &forged, core.WorkflowInstanceStateFinished, nil, nil, nil, nil))
		invalid := history.NewPendingEvent(time.Now(), history.EventType_SignalReceived, make(chan int))
		require.Error(t, b.CancelWorkflowInstance(ctx, instance, invalid))
		require.Error(t, b.SignalWorkflow(ctx, instance.InstanceID, invalid))
		scheduled := history.NewHistoryEvent(2, time.Now(), history.EventType_ActivityScheduled, &history.ActivityScheduledAttributes{}, history.ScheduleEventID(1))
		executed := append(task.NewEvents, scheduled)
		require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateActive, executed, []*history.Event{scheduled}, nil, nil))
		activity, err := b.GetActivityTask(ctx, queues)
		require.NoError(t, err)
		require.NotNil(t, activity)
		forgedActivity := *activity
		forgedActivity.ActivityID = uuid.NewString()
		require.Error(t, b.ExtendActivityTask(ctx, &forgedActivity))
		require.Error(t, b.CompleteActivityTask(ctx, &forgedActivity, invalid))
		require.Error(t, b.CompleteActivityTask(ctx, activity, invalid))
		completed := history.NewHistoryEvent(3, time.Now(), history.EventType_ActivityCompleted, &history.ActivityCompletedAttributes{}, history.ScheduleEventID(1))
		require.NoError(t, b.CompleteActivityTask(ctx, activity, completed))
		require.Error(t, b.CompleteActivityTask(ctx, activity, completed))
		next, err := b.GetWorkflowTask(ctx, queues)
		require.NoError(t, err)
		require.NotNil(t, next)
		require.Len(t, next.NewEvents, 1)
		require.Equal(t, completed.ID, next.NewEvents[0].ID)
		require.NoError(t, b.CompleteWorkflowTask(ctx, next, core.WorkflowInstanceStateFinished, next.NewEvents, nil, nil, nil))
		events, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
		require.NoError(t, err)
		require.Len(t, events, 3)
	})
	for _, unavailable := range []string{"history", "attributes"} {
		t.Run("removal rollback "+unavailable, func(t *testing.T) {
			b, db := factory(t)
			require.NoError(t, b.(interface{ Migrate() error }).Migrate())
			ctx := context.Background()
			instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
			task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
			require.NoError(t, err)
			require.NotNil(t, task)
			require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
			_, err = db.ExecContext(ctx, "ALTER TABLE "+unavailable+" RENAME TO unavailable") // #nosec G202 -- Only the two fixed table names above in a private test database.
			require.NoError(t, err)
			require.Error(t, b.RemoveWorkflowInstance(ctx, instance))
			_, err = db.ExecContext(ctx, "ALTER TABLE unavailable RENAME TO "+unavailable) // #nosec G202 -- Restores the same fixed test table.
			require.NoError(t, err)
			state, err := b.GetWorkflowInstanceState(ctx, instance)
			require.NoError(t, err)
			require.Equal(t, core.WorkflowInstanceStateFinished, state)
			events, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, started.ID, events[0].ID)
			require.NoError(t, b.RemoveWorkflowInstance(ctx, instance))
			_, err = b.GetWorkflowInstanceState(ctx, instance)
			require.ErrorIs(t, err, backend.ErrInstanceNotFound)
		})
	}
}
