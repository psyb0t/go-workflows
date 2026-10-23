package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/psyb0t/go-workflows/internal/cleanup"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/core"
)

func (sb *sqliteBackend) GetStats(ctx context.Context) (*backend.Stats, error) {
	s := &backend.Stats{}

	tx, err := sb.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to start transaction")
	}
	defer cleanup.Rollback(ctx, tx)

	row := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM instances i WHERE i.completed_at IS NULL",
	)
	if err := row.Err(); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active instances")
	}

	var activeInstances int64
	if err := row.Scan(&activeInstances); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to scan active instances")
	}

	s.ActiveWorkflowInstances = activeInstances

	// Get workflow instances ready to be picked up
	now := time.Now()
	workflowRows, err := tx.QueryContext(
		ctx,
		`SELECT i.queue, COUNT(*) FROM instances i
			WHERE
				(i.locked_until IS NULL OR i.locked_until < ?)
				AND i.state = ? AND i.completed_at IS NULL
				AND EXISTS (
					SELECT 1
						FROM pending_events
						WHERE instance_id = i.id AND execution_id = i.execution_id AND (visible_at IS NULL OR visible_at <= ?)
				)
			GROUP BY i.queue`,
		now,                              // locked_until
		core.WorkflowInstanceStateActive, // state
		now,                              // pending_event.visible_at
	)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active instances")
	}

	s.PendingWorkflowTasks = make(map[core.Queue]int64)

	for workflowRows.Next() {
		var queue string
		var pendingInstances int64
		if err := workflowRows.Scan(&queue, &pendingInstances); err != nil {
			return nil, ctxerrors.Wrap(err, "failed to scan active instances")
		}

		s.PendingWorkflowTasks[core.Queue(queue)] = pendingInstances
	}

	if workflowRows.Err() != nil {
		return nil, workflowRows.Err()
	}

	// Get pending activities
	activityRows, err := tx.QueryContext(
		ctx,
		"SELECT queue, COUNT(*) FROM activities GROUP BY queue")
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active activities")
	}

	s.PendingActivityTasks = make(map[core.Queue]int64)

	for activityRows.Next() {
		var queue string
		var pendingActivities int64
		if err := activityRows.Scan(&queue, &pendingActivities); err != nil {
			return nil, ctxerrors.Wrap(err, "failed to scan active activities")
		}

		s.PendingActivityTasks[core.Queue(queue)] = pendingActivities
	}

	if activityRows.Err() != nil {
		return nil, activityRows.Err()
	}

	return s, nil
}
