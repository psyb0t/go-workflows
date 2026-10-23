package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/payload"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/workflowerrors"
	"github.com/psyb0t/go-workflows/registry"
	wf "github.com/psyb0t/go-workflows/workflow"
)

func TestExecutorRejectsInconsistentEvents(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		kind       history.EventType
		attributes any
		scheduleID int64
	}{
		{"orphan activity scheduled", history.EventType_ActivityScheduled, &history.ActivityScheduledAttributes{}, 999},
		{"orphan activity completed", history.EventType_ActivityCompleted, &history.ActivityCompletedAttributes{}, 999},
		{"orphan activity failed", history.EventType_ActivityFailed, &history.ActivityFailedAttributes{}, 999},
		{"orphan timer scheduled", history.EventType_TimerScheduled, &history.TimerScheduledAttributes{}, 999},
		{"orphan timer canceled", history.EventType_TimerCanceled, &history.TimerCanceledAttributes{}, 999},
		{"orphan subworkflow scheduled", history.EventType_SubWorkflowScheduled, &history.SubWorkflowScheduledAttributes{}, 999},
		{"orphan subworkflow cancellation", history.EventType_SubWorkflowCancellationRequested, &history.SubWorkflowCancellationRequestedAttributes{}, 999},
		{"orphan subworkflow completed", history.EventType_SubWorkflowCompleted, &history.SubWorkflowCompletedAttributes{}, 999},
		{"orphan subworkflow failed", history.EventType_SubWorkflowFailed, &history.SubWorkflowFailedAttributes{}, 999},
		{"orphan side effect", history.EventType_SideEffectResult, &history.SideEffectResultAttributes{}, 999},
		{"orphan trace", history.EventType_TraceStarted, &history.TraceStartedAttributes{}, 999},
		{"different activity name", history.EventType_ActivityScheduled, &history.ActivityScheduledAttributes{Name: "different-activity"}, 1},
		{"timer command mismatch", history.EventType_TimerScheduled, &history.TimerScheduledAttributes{}, 1},
		{"timer cancellation mismatch", history.EventType_TimerCanceled, &history.TimerCanceledAttributes{}, 1},
		{"subworkflow command mismatch", history.EventType_SubWorkflowScheduled, &history.SubWorkflowScheduledAttributes{}, 1},
		{"subworkflow cancel mismatch", history.EventType_SubWorkflowCancellationRequested, &history.SubWorkflowCancellationRequestedAttributes{}, 1},
		{"side effect command mismatch", history.EventType_SideEffectResult, &history.SideEffectResultAttributes{}, 1},
		{"trace command mismatch", history.EventType_TraceStarted, &history.TraceStartedAttributes{}, 1},
		{"invalid activity result", history.EventType_ActivityCompleted, &history.ActivityCompletedAttributes{Result: payload.Payload("not-json")}, 1},
		{"unknown event", history.EventType(999), nil, 999},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := registry.New()
			definition := func(ctx wf.Context) (int, error) {
				return wf.ExecuteActivity[int](ctx, wf.DefaultActivityOptions, activity1, 42).Get(ctx)
			}
			require.NoError(t, r.RegisterWorkflow(definition))
			instance := core.NewWorkflowInstance(uuid.NewString(), "executionID")
			e, err := newExecutor(r, instance, &testHistoryProvider{})
			require.NoError(t, err)
			t.Cleanup(e.Close)
			first, err := e.ExecuteTask(context.Background(), startWorkflowTask(instance.InstanceID, definition))
			require.NoError(t, err)
			require.Equal(t, core.WorkflowInstanceStateActive, first.State)
			bad := history.NewPendingEvent(time.Now(), tc.kind, tc.attributes, history.ScheduleEventID(tc.scheduleID))
			result, err := e.ExecuteTask(context.Background(), continueTask(instance.InstanceID, []*history.Event{bad}, first.Executed[len(first.Executed)-1].SequenceID))
			require.NoError(t, err)
			require.Equal(t, core.WorkflowInstanceStateFinished, result.State)
			var completion *history.ExecutionCompletedAttributes
			for _, event := range result.Executed {
				if event.Type == history.EventType_WorkflowExecutionFinished {
					completion = event.Attributes.(*history.ExecutionCompletedAttributes)
				}
			}
			require.NotNil(t, completion)
			require.NotNil(t, completion.Error)
		})
	}
}

func TestExecutorHistoryBoundaries(t *testing.T) {
	t.Parallel()
	definition := func(ctx wf.Context) error {
		wf.NewSignalChannel[bool](ctx, "resume").Receive(ctx)
		return nil
	}
	for _, limit := range []int64{1, 10_000} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			r := registry.New()
			require.NoError(t, r.RegisterWorkflow(definition))
			instance := core.NewWorkflowInstance(uuid.NewString(), "executionID")
			e, err := newExecutor(r, instance, &testHistoryProvider{}, limit)
			require.NoError(t, err)
			t.Cleanup(e.Close)
			result, err := e.ExecuteTask(context.Background(), startWorkflowTask(instance.InstanceID, definition))
			require.NoError(t, err)
			if limit == 1 {
				require.Equal(t, core.WorkflowInstanceStateFinished, result.State)
				last := result.Executed[len(result.Executed)-1]
				require.Equal(t, history.EventType_WorkflowExecutionFinished, last.Type)
				require.ErrorContains(t, workflowerrors.ToError(last.Attributes.(*history.ExecutionCompletedAttributes).Error), "workflow history size exceeded 1 events")
				return
			}
			require.Equal(t, core.WorkflowInstanceStateActive, result.State)
			_, err = e.ExecuteTask(context.Background(), continueTask(instance.InstanceID, nil, 0))
			require.ErrorIs(t, err, commerr.ErrInvalidState)
			_, err = e.ExecuteTask(context.Background(), continueTask(instance.InstanceID, nil, 100))
			require.Error(t, err)
		})
	}
}
