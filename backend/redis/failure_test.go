package redis

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestClosedClientFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("external-backend integration requires the complete suite")
	}
	ctx := context.Background()
	rdb := getClient()
	require.NoError(t, rdb.Ping(ctx).Err())
	b, err := NewRedisBackend(rdb, WithKeyPrefix(uuid.NewString()))
	require.NoError(t, err)
	require.NoError(t, b.Close())
	instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
	event := history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
	task := &backend.WorkflowTask{ID: "1-0", Queue: workflow.QueueDefault, WorkflowInstance: instance, CustomData: "1-0"}
	activity := &backend.ActivityTask{ID: "1-0", ActivityID: uuid.NewString(), Queue: workflow.QueueDefault, WorkflowInstance: instance}
	queues := []workflow.Queue{workflow.QueueDefault}
	testCases := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return b.CreateWorkflowInstance(ctx, instance, event) }},
		{"cancel", func() error { return b.CancelWorkflowInstance(ctx, instance, event) }},
		{"remove", func() error { return b.RemoveWorkflowInstance(ctx, instance) }},
		{"state", func() error { _, err := b.GetWorkflowInstanceState(ctx, instance); return err }},
		{"history", func() error { _, err := b.GetWorkflowInstanceHistory(ctx, instance, nil); return err }},
		{"signal", func() error { return b.SignalWorkflow(ctx, instance.InstanceID, event) }},
		{"prepare workflow queues", func() error { return b.PrepareWorkflowQueues(ctx, queues) }},
		{"prepare activity queues", func() error { return b.PrepareActivityQueues(ctx, queues) }},
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
		{"future events", func() error { _, err := b.GetFutureEvents(ctx); return err }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { require.ErrorIs(t, tc.call(), redisclient.ErrClosed) })
	}
	_, err = NewRedisBackend(rdb)
	require.ErrorIs(t, err, redisclient.ErrClosed)
}

func TestSerializationFailurePreservesWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("external-backend integration requires the complete suite")
	}
	ctx := context.Background()
	b, err := NewRedisBackend(getClient(), WithKeyPrefix(uuid.NewString()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
	started := history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault, Metadata: &workflow.Metadata{}})
	require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
	require.NoError(t, b.PrepareWorkflowQueues(ctx, []workflow.Queue{workflow.QueueDefault}))
	task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
	require.NoError(t, err)
	require.NotNil(t, task)
	for i, event := range task.NewEvents {
		event.SequenceID = int64(i + 1)
	}
	invalid := history.NewPendingEvent(time.Now(), history.EventType_SignalReceived, make(chan int), history.VisibleAt(time.Now().Add(time.Hour)))
	for _, timers := range []bool{false, true} {
		var executed, scheduled []*history.Event
		if timers {
			scheduled = []*history.Event{invalid}
		} else {
			executed = []*history.Event{invalid}
		}
		require.Error(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, executed, nil, scheduled, nil))
	}
	require.Error(t, b.SignalWorkflow(ctx, instance.InstanceID, invalid))
	require.Error(t, b.CancelWorkflowInstance(ctx, instance, invalid))
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
}
