package test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/workflow"
)

// SQLCorruptionTest checks persisted data boundaries and rollback of failed polls.
func SQLCorruptionTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB), activityReadsInstanceMetadata bool) {
	t.Helper()
	phases := []string{"workflow metadata", "workflow payload", "activity payload", "history payload", "future payload"}
	if activityReadsInstanceMetadata {
		phases = append(phases, "activity metadata")
	}
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			b, db := factory(t)
			require.NoError(t, b.(interface{ Migrate() error }).Migrate())
			ctx := context.Background()
			instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
			queues := []workflow.Queue{workflow.QueueDefault}
			if phase == "activity metadata" || phase == "activity payload" || phase == "history payload" || phase == "future payload" {
				task, err := b.GetWorkflowTask(ctx, queues)
				require.NoError(t, err)
				require.NotNil(t, task)
				var activities, timers []*history.Event
				if phase == "activity metadata" || phase == "activity payload" {
					activities = []*history.Event{history.NewHistoryEvent(2, time.Now(), history.EventType_ActivityScheduled, &history.ActivityScheduledAttributes{}, history.ScheduleEventID(1))}
					task.NewEvents = append(task.NewEvents, activities...)
				}
				if phase == "future payload" {
					timers = []*history.Event{history.NewPendingEvent(time.Now(), history.EventType_TimerFired, &history.TimerFiredAttributes{}, history.VisibleAt(time.Now().Add(time.Hour)))}
				}
				require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateActive, task.NewEvents, activities, timers, nil))
			}
			metadata := phase == "workflow metadata" || phase == "activity metadata"
			corrupt, restore := "UPDATE attributes SET data = '{'", "UPDATE attributes SET data = '{}'"
			if metadata {
				corrupt, restore = "UPDATE instances SET metadata = '{'", "UPDATE instances SET metadata = '{}'"
			}
			_, err := db.ExecContext(ctx, corrupt)
			require.NoError(t, err)
			switch phase {
			case "workflow metadata", "workflow payload":
				_, err = b.GetWorkflowTask(ctx, queues)
			case "activity metadata", "activity payload":
				_, err = b.GetActivityTask(ctx, queues)
			case "history payload":
				_, err = b.GetWorkflowInstanceHistory(ctx, instance, nil)
				require.Error(t, err)
				cursor := int64(0)
				_, err = b.GetWorkflowInstanceHistory(ctx, instance, &cursor)
			case "future payload":
				_, err = b.(TestBackend).GetFutureEvents(ctx)
			}
			require.Error(t, err)
			_, err = db.ExecContext(ctx, restore)
			require.NoError(t, err)
			switch phase {
			case "workflow metadata", "workflow payload":
				task, err := b.GetWorkflowTask(ctx, queues)
				require.NoError(t, err)
				require.NotNil(t, task)
				require.Equal(t, instance.InstanceID, task.WorkflowInstance.InstanceID)
			case "activity metadata", "activity payload":
				task, err := b.GetActivityTask(ctx, queues)
				require.NoError(t, err)
				require.NotNil(t, task)
				require.Equal(t, instance.InstanceID, task.WorkflowInstance.InstanceID)
			case "history payload":
				events, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
				require.NoError(t, err)
				require.Len(t, events, 1)
			case "future payload":
				events, err := b.(TestBackend).GetFutureEvents(ctx)
				require.NoError(t, err)
				require.Len(t, events, 1)
			}
		})
	}
}
