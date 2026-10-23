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

// SQLAtomicityTest makes individual tables unavailable after a task is leased.
// Every failed completion must leave the task and its original history intact.
func SQLAtomicityTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB)) {
	t.Helper()
	testCases := []struct {
		name, unavailable           string
		activities, timers, invalid bool
	}{
		{name: "history write", unavailable: "history"},
		{name: "attribute write", unavailable: "attributes"},
		{name: "pending event deletion", unavailable: "pending_events"},
		{name: "activity scheduling", unavailable: "activities", activities: true},
		{name: "timer scheduling", unavailable: "pending_events", timers: true},
		{name: "unserializable history", invalid: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b, db := factory(t)
			require.NoError(t, b.(interface{ Migrate() error }).Migrate())
			ctx := context.Background()
			instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
			started := history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
			task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
			require.NoError(t, err)
			require.NotNil(t, task)
			executed := task.NewEvents
			var activities, timers []*history.Event
			if tc.activities {
				activities = []*history.Event{history.NewPendingEvent(time.Now(), history.EventType_ActivityScheduled, &history.ActivityScheduledAttributes{})}
			}
			if tc.timers {
				executed = nil
				timers = []*history.Event{history.NewPendingEvent(time.Now(), history.EventType_TimerFired, &history.TimerFiredAttributes{}, history.VisibleAt(time.Now().Add(time.Hour)))}
			}
			if tc.invalid {
				executed = []*history.Event{history.NewPendingEvent(time.Now(), history.EventType_SignalReceived, make(chan int))}
			}
			if tc.unavailable != "" {
				switch tc.unavailable {
				case "history", "attributes", "pending_events", "activities":
					_, err = db.ExecContext(ctx, "ALTER TABLE "+tc.unavailable+" RENAME TO unavailable") // #nosec G202 -- Closed table-name allowlist in a test-owned database.
					require.NoError(t, err)
				default:
					t.Fatal("unsupported test table")
				}
			}
			err = b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, executed, activities, timers, nil)
			require.Error(t, err)
			if tc.unavailable != "" {
				_, err = db.ExecContext(ctx, "ALTER TABLE unavailable RENAME TO "+tc.unavailable) // #nosec G202 -- Same allowlisted test table restored above.
				require.NoError(t, err)
			}
			state, err := b.GetWorkflowInstanceState(ctx, instance)
			require.NoError(t, err)
			require.Equal(t, core.WorkflowInstanceStateActive, state)
			h, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
			require.NoError(t, err)
			require.Empty(t, h)
			require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
			h, err = b.GetWorkflowInstanceHistory(ctx, instance, nil)
			require.NoError(t, err)
			require.Len(t, h, 1)
			require.Equal(t, started.ID, h[0].ID)
		})
	}
}
