package test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/workflow"
)

// SQLRepositoryTest exercises migration reuse, cursor ties and batched event joins
// through the same backend API on each SQL engine.
func SQLRepositoryTest(t *testing.T, factory func(*testing.T) (diag.Backend, *sql.DB), dialect string) {
	t.Helper()
	t.Run("legacy physical rows remain readable", func(t *testing.T) {
		b, pool := factory(t)
		migrator := b.(interface{ Migrate() error })
		require.NoError(t, migrator.Migrate())
		ctx := context.Background()
		var insert string
		switch dialect {
		case "sqlite":
			insert = "INSERT INTO instances (id, execution_id, metadata, state, queue, created_at) VALUES (?, ?, ?, ?, ?, ?)"
		case "mysql":
			insert = "INSERT INTO instances (instance_id, execution_id, metadata, state, queue, created_at) VALUES (?, ?, ?, ?, ?, ?)"
		case "postgres":
			insert = "INSERT INTO instances (instance_id, execution_id, metadata, state, queue, created_at) VALUES ($1, $2, $3, $4, $5, $6)"
		default:
			t.Fatalf("unsupported test dialect %q", dialect)
		}
		instance := core.NewWorkflowInstance("preexisting-'--", "execution-'--")
		_, err := pool.ExecContext(ctx, insert, instance.InstanceID, instance.ExecutionID,
			[]byte("null"), int(core.WorkflowInstanceStateActive), string(workflow.QueueDefault), time.Now().UTC())
		require.NoError(t, err)
		require.NoError(t, migrator.Migrate())
		state, err := b.GetWorkflowInstanceState(ctx, instance)
		require.NoError(t, err)
		require.Equal(t, core.WorkflowInstanceStateActive, state)
		signal := history.NewHistoryEvent(1, time.Now(), history.EventType_SignalReceived, &history.SignalReceivedAttributes{})
		require.NoError(t, b.SignalWorkflow(ctx, instance.InstanceID, signal))
		task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, instance, task.WorkflowInstance)
		require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		events, err := b.GetWorkflowInstanceHistory(ctx, instance, nil)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, signal.ID, events[0].ID)
	})
	t.Run("database handles stay isolated", func(t *testing.T) {
		first, _ := factory(t)
		second, _ := factory(t)
		ctx := context.Background()
		instance := core.NewWorkflowInstance("same-instance", "same-execution")
		for i, b := range []diag.Backend{first, second} {
			require.NoError(t, b.(interface{ Migrate() error }).Migrate())
			queue := workflow.Queue(fmt.Sprintf("database-%d", i))
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted,
				&history.ExecutionStartedAttributes{Queue: queue})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
		}
		for i, b := range []diag.Backend{first, second} {
			queue := workflow.Queue(fmt.Sprintf("database-%d", i))
			task, err := b.GetWorkflowTask(ctx, []workflow.Queue{queue})
			require.NoError(t, err)
			require.NotNil(t, task)
			require.Equal(t, queue, task.Queue)
			require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		}
	})
	t.Run("retention deletes exact execution pairs", func(t *testing.T) {
		b, _ := factory(t)
		require.NoError(t, b.(interface{ Migrate() error }).Migrate())
		ctx := context.Background()
		for _, instance := range []*core.WorkflowInstance{
			core.NewWorkflowInstance("first", "one"),
			core.NewWorkflowInstance("second", "two"),
		} {
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted,
				&history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
			task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
			require.NoError(t, err)
			require.NotNil(t, task)
			require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		}
		active := []*core.WorkflowInstance{
			core.NewWorkflowInstance("first", "two"),
			core.NewWorkflowInstance("second", "one"),
		}
		for _, instance := range active {
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted,
				&history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
		}
		for _, size := range []int{0, -1} {
			require.ErrorIs(t, b.RemoveWorkflowInstances(ctx, backend.RemoveFinishedBatchSize(size)), commerr.ErrInvalidArgument)
		}
		require.NoError(t, b.RemoveWorkflowInstances(ctx, backend.RemoveFinishedBefore(time.Now().Add(time.Hour)), backend.RemoveFinishedBatchSize(1)))
		for _, instance := range active {
			state, err := b.GetWorkflowInstanceState(ctx, instance)
			require.NoError(t, err)
			require.Equal(t, core.WorkflowInstanceStateActive, state)
		}
		for _, instance := range []*core.WorkflowInstance{
			core.NewWorkflowInstance("first", "one"),
			core.NewWorkflowInstance("second", "two"),
		} {
			_, err := b.GetWorkflowInstanceState(ctx, instance)
			require.ErrorIs(t, err, backend.ErrInstanceNotFound)
		}
	})
	t.Run("migration preserves rows and releases connections", func(t *testing.T) {
		b, pool := factory(t)
		migrator := b.(interface{ Migrate() error })
		require.NoError(t, migrator.Migrate())
		require.Zero(t, pool.Stats().InUse)
		pool.SetMaxOpenConns(1)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		instance := core.NewWorkflowInstance("legacy-'--", "execution-'--")
		started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted,
			&history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
		require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
		var before int64
		require.NoError(t, pool.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&before))
		require.NoError(t, migrator.Migrate())
		require.Zero(t, pool.Stats().InUse)
		var after int64
		require.NoError(t, pool.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&after))
		require.Equal(t, before, after)
		task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Equal(t, instance, task.WorkflowInstance)
		require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		var rows int
		require.NoError(t, pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM history").Scan(&rows))
		require.Equal(t, 1, rows)
		require.NoError(t, b.Close())
		require.NoError(t, pool.PingContext(ctx))
	})
	t.Run("cursor pages with tied timestamps", func(t *testing.T) {
		b, pool := factory(t)
		require.NoError(t, b.(interface{ Migrate() error }).Migrate())
		ctx := context.Background()
		for i := range 13 {
			instance := core.NewWorkflowInstance(fmt.Sprintf("instance-%02d", i/2), fmt.Sprintf("execution-%02d", i))
			started := history.NewHistoryEvent(1, time.Now(), history.EventType_WorkflowExecutionStarted,
				&history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
			require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
			task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
			require.NoError(t, err)
			require.NotNil(t, task)
			require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		}
		_, err := pool.ExecContext(ctx, "UPDATE instances SET created_at = '2020-01-01 00:00:00'")
		require.NoError(t, err)
		actual := make([]string, 0, 13)
		var afterID, afterExecution string
		for _, size := range []int{5, 5, 3, 0} {
			page, err := b.GetWorkflowInstances(ctx, afterID, afterExecution, 5)
			require.NoError(t, err)
			require.Len(t, page, size)
			for _, row := range page {
				actual = append(actual, row.Instance.InstanceID+"/"+row.Instance.ExecutionID)
				afterID, afterExecution = row.Instance.InstanceID, row.Instance.ExecutionID
			}
		}
		for i, id := range actual {
			require.Equal(t, fmt.Sprintf("instance-%02d/execution-%02d", (12-i)/2, 12-i), id)
		}
		for _, count := range []int{0, -1} {
			_, err := b.GetWorkflowInstances(ctx, "", "", count)
			require.ErrorIs(t, err, commerr.ErrInvalidArgument)
		}
	})
	t.Run("history order across insert batches", func(t *testing.T) {
		b, _ := factory(t)
		require.NoError(t, b.(interface{ Migrate() error }).Migrate())
		ctx := context.Background()
		instance := core.NewWorkflowInstance("batched", "execution")
		started := history.NewHistoryEvent(51, time.Now(), history.EventType_WorkflowExecutionStarted,
			&history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})
		require.NoError(t, b.CreateWorkflowInstance(ctx, instance, started))
		for sequence := int64(50); sequence > 0; sequence-- {
			event := history.NewHistoryEvent(sequence, time.Now(), history.EventType_SignalReceived, &history.SignalReceivedAttributes{})
			require.NoError(t, b.SignalWorkflow(ctx, instance.InstanceID, event))
		}
		stats, err := b.GetStats(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, stats.PendingWorkflowTasks[workflow.QueueDefault])
		task, err := b.GetWorkflowTask(ctx, []workflow.Queue{workflow.QueueDefault})
		require.NoError(t, err)
		require.NotNil(t, task)
		require.Len(t, task.NewEvents, 51)
		require.NoError(t, b.CompleteWorkflowTask(ctx, task, core.WorkflowInstanceStateFinished, task.NewEvents, nil, nil, nil))
		for _, cursor := range []int64{0, 25, 51, 100} {
			events, err := b.GetWorkflowInstanceHistory(ctx, instance, &cursor)
			require.NoError(t, err)
			if cursor >= 51 {
				require.Empty(t, events)
				continue
			}
			require.Len(t, events, 51-int(cursor))
			for index, event := range events {
				require.Equal(t, cursor+int64(index)+1, event.SequenceID)
			}
		}
		for _, queues := range [][]workflow.Queue{nil, {}} {
			_, err := b.GetWorkflowTask(ctx, queues)
			require.ErrorIs(t, err, commerr.ErrInvalidArgument)
			_, err = b.GetActivityTask(ctx, queues)
			require.ErrorIs(t, err, commerr.ErrInvalidArgument)
		}
	})
}
